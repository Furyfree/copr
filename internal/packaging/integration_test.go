//go:build integration

package packaging

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func native(t *testing.T, dir, name string, args ...string) []byte {
	t.Helper()
	data, err := Run(t.Context(), name, args, dir, CleanEnvironment())
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func selected(t *testing.T, name string) {
	t.Helper()
	if selection := os.Getenv("COPR_TEST_PACKAGE"); selection != "" {
		c, err := Load(repository(t))
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := c.Projects[selection]; !ok {
			t.Fatalf("unknown package %q", selection)
		}
		if selection != name {
			t.Skip("another package selected")
		}
	}
}

func TestNativeHelperRPMs(t *testing.T) {
	for _, name := range []string{"github-copilot-installer", "wowup-cf-installer"} {
		t.Run(name, func(t *testing.T) {
			selected(t, name)
			root := t.TempDir()
			b := New(repository(t))
			srpm, err := b.Prepare(t.Context(), name, filepath.Join(root, "sources"))
			if err != nil {
				t.Fatal(err)
			}
			identity := native(t, "", "rpm", "-qp", "--qf", "%{NAME}\n%{SOURCEPACKAGE}\n", srpm)
			if string(identity) != name+"\n1\n" {
				t.Fatalf("source identity: %s", identity)
			}
			files := strings.Fields(string(native(t, "", "rpm", "-qpl", srpm)))
			if len(files) != 2 || !slices.Contains(files, name+".spec") {
				t.Fatalf("unexpected SRPM payload: %v", files)
			}
			native(t, "", "rpmbuild", "--rebuild", srpm, "--define", "_topdir "+filepath.Join(root, "build"))
			packages, err := filepath.Glob(filepath.Join(root, "build/RPMS/x86_64/*.rpm"))
			if err != nil || len(packages) != 1 {
				t.Fatalf("expected one binary RPM, got %v", packages)
			}
			rpm := packages[0]
			for _, option := range []string{"--scripts", "--triggers"} {
				if data := native(t, "", "rpm", "-qp", option, rpm); len(data) != 0 {
					t.Fatalf("unexpected %s: %s", option, data)
				}
			}
			requires := strings.Fields(string(native(t, "", "rpm", "-qp", "--requires", rpm)))
			for _, dependency := range requires {
				if strings.HasPrefix(dependency, "python") || dependency == "jq" || dependency == "curl" {
					t.Fatalf("obsolete interpreter dependency: %s", dependency)
				}
			}
			paths := strings.Fields(string(native(t, "", "rpm", "-qpl", rpm)))
			for _, path := range paths {
				if path != "/usr/bin/"+name && !strings.HasPrefix(path, "/usr/share/doc/"+name) && !strings.HasPrefix(path, "/usr/share/licenses/"+name) && !strings.HasPrefix(path, "/usr/share/man/man1/"+name+".1") {
					t.Fatalf("unexpected installed path %q", path)
				}
			}
			payload := native(t, "", "rpm2cpio", rpm)
			extracted := filepath.Join(root, "extracted")
			if err := os.Mkdir(extracted, 0o755); err != nil {
				t.Fatal(err)
			}
			cmd := exec.CommandContext(t.Context(), "cpio", "-idm", "--quiet")
			cmd.Dir = extracted
			cmd.Stdin = bytes.NewReader(payload)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("extract: %v %s", err, out)
			}
			version := string(native(t, "", "rpm", "-qp", "--qf", "%{VERSION}", rpm))
			arg := "version"
			if name == "wowup-cf-installer" {
				arg = "--version"
			}
			if actual := strings.TrimSpace(string(native(t, "", filepath.Join(extracted, "usr/bin", name), arg))); actual != name+" "+version {
				t.Fatalf("helper version: %q", actual)
			}
			license, err := os.ReadFile(filepath.Join(extracted, "usr/share/licenses", name, "LICENSE"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(license, []byte("Permission is hereby granted")) {
				t.Fatal("MIT notice missing")
			}
			if name == "github-copilot-installer" && !bytes.Contains(license, []byte("Chris Titus Tech")) {
				t.Fatal("upstream copyright missing")
			}
		})
	}
}

func TestNativeMakeContract(t *testing.T) {
	root := t.TempDir()
	pkg := filepath.Join(root, "package")
	source := filepath.Join(root, "source")
	write(t, filepath.Join(source, "README"), "fixture\n")
	if err := os.Mkdir(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := bundle(source, filepath.Join(pkg, "hello.tar.gz"), "hello-1.0"); err != nil {
		t.Fatal(err)
	}
	spec := "Name: hello\nVersion: 1.0\nRelease: 1\nSummary: Packaging fixture\nLicense: MIT\nSource0: hello.tar.gz\nBuildArch: noarch\n%description\nFixture.\n%prep\n%setup -q\n%files\n"
	write(t, filepath.Join(pkg, "hello.spec"), spec)
	native(t, pkg, "make", "-f", filepath.Join(repository(t), ".copr/Makefile"), "srpm", "spec=hello.spec", "outdir="+filepath.Join(root, "out"))
	archives, _ := filepath.Glob(filepath.Join(root, "out/*.src.rpm"))
	if len(archives) != 1 {
		t.Fatalf("SRPMs: %v", archives)
	}
	if string(native(t, "", "rpm", "-qp", "--qf", "%{NAME}", archives[0])) != "hello" {
		t.Fatal("wrong SRPM")
	}
	entries, _ := os.ReadDir(pkg)
	if len(entries) != 2 {
		t.Fatal("source directory mutated")
	}
	after, _ := os.ReadFile(filepath.Join(pkg, "hello.spec"))
	if string(after) != spec {
		t.Fatal("source spec changed")
	}
}

func TestNimbusChecksumBeforeExtraction(t *testing.T) {
	selected(t, "nimbus")
	root := t.TempDir()
	recipe, err := os.ReadFile(filepath.Join(repository(t), "packages/nimbus/nimbus.spec"))
	if err != nil {
		t.Fatal(err)
	}
	version, err := specVersion(recipe)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source")
	write(t, filepath.Join(source, "extraction-marker"), "approved source\n")
	archive := filepath.Join(root, "nimbus-"+version+"-vendor.tar.gz")
	if err := bundle(source, archive, "nimbus-"+version); err != nil {
		t.Fatal(err)
	}
	digest, err := fileDigest(archive)
	if err != nil {
		t.Fatal(err)
	}
	spec := filepath.Join(root, "nimbus.spec")
	pattern := regexp.MustCompile(`(?m)^%global source_sha256 [a-f0-9]+$`)
	write(t, spec, pattern.ReplaceAllString(string(recipe), "%global source_sha256 "+digest))
	for _, changed := range []bool{false, true} {
		if changed {
			write(t, filepath.Join(source, "extraction-marker"), "changed")
			os.Remove(archive)
			if err := bundle(source, archive, "nimbus-"+version); err != nil {
				t.Fatal(err)
			}
		}
		build := filepath.Join(root, "approved")
		if changed {
			build = filepath.Join(root, "changed")
		}
		_, err := Run(t.Context(), "rpmbuild", []string{"-bp", "--nodeps", spec, "--define", "_topdir " + build, "--define", "_sourcedir " + root}, "", CleanEnvironment())
		found := false
		filepath.WalkDir(build, func(path string, d os.DirEntry, err error) error {
			if err == nil && d.Name() == "extraction-marker" {
				found = true
			}
			return nil
		})
		if changed && (err == nil || found) {
			t.Fatal("changed source was extracted")
		}
		if !changed && (err != nil || !found) {
			t.Fatalf("approved source: %v, extracted=%v", err, found)
		}
	}
}

func TestVoxtypeNativeLifecycle(t *testing.T) {
	selected(t, "voxtype")
	expanded := native(t, "", "rpmspec", "-P", filepath.Join(repository(t), "packages/voxtype/voxtype.spec"))
	scripts := map[string]string{}
	for _, phase := range []string{"post", "preun", "postun"} {
		start := strings.Index(string(expanded), "%"+phase+"\n")
		if start < 0 {
			t.Fatalf("missing %s", phase)
		}
		body := string(expanded)[start+len(phase)+2:]
		if end := strings.Index(body, "\n%"); end >= 0 {
			body = body[:end]
		}
		scripts[phase] = body
	}
	root := t.TempDir()
	helper := filepath.Join(root, "systemd-update-helper")
	log := filepath.Join(root, "calls")
	cases := [][3]string{{"post", "1", "install-user-units voxtype.service\n"}, {"post", "2", ""}, {"preun", "0", "remove-user-units voxtype.service\n"}, {"preun", "1", ""}, {"postun", "1", "mark-restart-user-units voxtype.service\n"}, {"postun", "0", ""}}
	for _, outcome := range []string{"0", "1", "missing"} {
		write(t, helper, "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$CALL_LOG\"\nexit \"$HELPER_RESULT\"\n")
		os.Chmod(helper, 0o755)
		if outcome == "missing" {
			os.Remove(helper)
		}
		for _, c := range cases {
			write(t, log, "")
			script := strings.ReplaceAll(scripts[c[0]], "/usr/lib/systemd/systemd-update-helper", helper)
			if strings.Contains(script, "%systemd_") {
				t.Fatal("systemd macros were not expanded")
			}
			cmd := exec.CommandContext(t.Context(), "sh", "-c", script, "scriptlet", c[1])
			cmd.Env = append(CleanEnvironment(), "CALL_LOG="+log, "HELPER_RESULT="+outcome)
			if data, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("scriptlet: %v %s", err, data)
			}
			want := c[2]
			if outcome == "missing" {
				want = ""
			}
			data, _ := os.ReadFile(log)
			if string(data) != want {
				t.Fatalf("%v: %q", c, data)
			}
		}
	}
}

func TestVoxtypeFedoraPreset(t *testing.T) {
	selected(t, "voxtype")
	root := t.TempDir()
	write(t, filepath.Join(root, "usr/lib/systemd/user/voxtype.service"), "[Service]\nExecStart=/usr/bin/true\n[Install]\nWantedBy=default.target\n")
	presets, err := filepath.Glob("/usr/lib/systemd/user-preset/*.preset")
	if err != nil || len(presets) == 0 {
		t.Fatal("Fedora user presets required")
	}
	for _, preset := range presets {
		data, err := os.ReadFile(preset)
		if err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(root, "usr/lib/systemd/user-preset", filepath.Base(preset)), string(data))
	}
	native(t, "", "systemctl", "--root", root, "--global", "preset", "voxtype.service")
	cmd := exec.CommandContext(context.Background(), "systemctl", "--root", root, "--global", "is-enabled", "voxtype.service")
	data, err := cmd.Output()
	if err == nil || strings.TrimSpace(string(data)) != "disabled" {
		t.Fatalf("preset state %q: %v", data, err)
	}
}

func TestNativeCheckPackageScope(t *testing.T) {
	for _, tc := range []struct{ name, targets string }{
		{"nimbus", "./cmd/coprctl ./internal/packaging"},
		{"voxtype", "./cmd/coprctl ./internal/packaging"},
		{"github-copilot-installer", "./cmd/coprctl ./internal/packaging ./cmd/github-copilot-installer ./internal/copilot"},
		{"wowup-cf-installer", "./cmd/coprctl ./internal/packaging ./cmd/wowup-cf-installer ./internal/wowup"},
		{"", "./..."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			bin := filepath.Join(root, "bin")
			log := filepath.Join(root, "commands")
			write(t, filepath.Join(bin, "go"), "#!/bin/sh\nprintf '%s|%s\\n' \"${COPR_TEST_PACKAGE-}\" \"$*\" >> \"$COMMAND_LOG\"\n")
			if err := os.Chmod(filepath.Join(bin, "go"), 0o755); err != nil {
				t.Fatal(err)
			}
			args := []string{"check"}
			want := []string{"|vet ./...", "|test -tags=integration ./...", "|run ./cmd/coprctl verify-specs"}
			if tc.name != "" {
				args = []string{"check-package", tc.name}
				want = []string{"|run ./cmd/coprctl verify-specs " + tc.name, "|vet " + tc.targets, tc.name + "|test -tags=integration " + tc.targets + " -count=1"}
			}
			cmd := exec.CommandContext(t.Context(), "just", args...)
			cmd.Dir = repository(t)
			cmd.Env = append(CleanEnvironment(), "PATH="+bin+":"+os.Getenv("PATH"), "COMMAND_LOG="+log, "COPR_TEST_PACKAGE=")
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("just %v: %s %v", args, output, err)
			}
			data, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != strings.Join(want, "\n")+"\n" {
				t.Fatalf("wrong scope:\n%s\nwant: %v", data, want)
			}
		})
	}
}
