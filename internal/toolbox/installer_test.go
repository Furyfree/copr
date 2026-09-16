package toolbox

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

const current, newer, previous = "3.8.0.87909", "3.8.1.90000", "3.7.2.87231"

type member struct {
	name, body, link string
	typ              byte
	mode             int64
}

func archive(t *testing.T, members []member) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, m := range members {
		h := &tar.Header{Name: m.name, Typeflag: m.typ, Mode: m.mode, Linkname: m.link, Size: int64(len(m.body))}
		if m.typ == 0 {
			h.Typeflag = tar.TypeReg
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(m.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := errors.Join(tw.Close(), gz.Close()); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// officialMembers mirrors the published distribution layout: one top-level
// directory holding bin/ with the launcher, assets and an internal symlink.
func officialMembers() []member {
	top := "jetbrains-toolbox-" + current + "/"
	return []member{
		{name: top, typ: tar.TypeDir, mode: 0o755},
		{name: top + "bin/", typ: tar.TypeDir, mode: 0o755},
		{name: top + "bin/jetbrains-toolbox", body: "#!/bin/sh\nexit 0\n", mode: 0o755},
		{name: top + "bin/toolbox.svg", body: "<svg/>", mode: 0o644},
		{name: top + "bin/payload", body: "application payload", mode: 0o644},
		{name: top + "bin/lib/", typ: tar.TypeDir, mode: 0o755},
		{name: top + "bin/lib/link", typ: tar.TypeSymlink, link: "../jetbrains-toolbox"},
	}
}

type fixture struct {
	e                      *Installer
	root, artifact, digest string
}

func setup(t *testing.T) fixture {
	t.Helper()
	root := t.TempDir()
	e := New()
	e.fixtureRoot = root
	e.uid, e.gid = uint32(os.Getuid()), uint32(os.Getgid())
	e.app = filepath.Join(root, "opt/jetbrains-toolbox")
	e.removing = filepath.Join(root, "opt/.jetbrains-toolbox-removing")
	e.journal = filepath.Join(root, "opt/.jetbrains-toolbox-removal.json")
	e.links = []link{{filepath.Join(root, "usr/local/bin/jetbrains-toolbox"), filepath.Join(e.app, "launcher")}, {filepath.Join(root, "usr/local/share/applications/jetbrains-toolbox.desktop"), filepath.Join(e.app, "jetbrains-toolbox.desktop")}}
	if err := os.MkdirAll(filepath.Dir(e.app), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "official.tar.gz")
	data := archive(t, officialMembers())
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return fixture{e, root, path, hex.EncodeToString(sum[:])}
}
func (f fixture) install(t *testing.T, version string) Status {
	t.Helper()
	s, err := f.e.Apply(t.Context(), f.artifact, f.digest, version)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestLifecycle(t *testing.T) {
	f := setup(t)
	s, err := f.e.Status()
	if err != nil || s.Installed {
		t.Fatalf("absent: %+v %v", s, err)
	}
	if !f.install(t, current).Integrated {
		t.Fatal("not integrated")
	}
	for _, name := range []string{"app/bin/jetbrains-toolbox", "app/bin/lib/link", "launcher", "jetbrains-toolbox.desktop", archiveName, "receipt.json"} {
		if _, err := os.Lstat(filepath.Join(f.e.app, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(f.e.links[0].path); err != nil {
		t.Fatal(err)
	}
	s, err = f.e.Status()
	if err != nil || s.Integrated || !s.Verified {
		t.Fatalf("missing link: %+v %v", s, err)
	}
	if !f.install(t, current).Integrated {
		t.Fatal("repair failed")
	}
	if f.install(t, newer).Version != newer {
		t.Fatal("update failed")
	}
	before, _ := os.ReadFile(filepath.Join(f.e.app, "receipt.json"))
	if _, err := f.e.Apply(t.Context(), f.artifact, f.digest, current); err == nil {
		t.Fatal("downgrade accepted")
	}
	after, _ := os.ReadFile(filepath.Join(f.e.app, "receipt.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("downgrade changed receipt")
	}
	personal := filepath.Join(f.root, "user-data")
	if err := os.WriteFile(personal, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err = f.e.Uninstall(t.Context())
	if err != nil || s.Installed {
		t.Fatalf("remove: %+v %v", s, err)
	}
	if _, err := os.Stat(personal); err != nil {
		t.Fatal("user data removed")
	}
	for _, link := range f.e.links {
		if exists(link.path) {
			t.Fatal("integration remains")
		}
		if _, err := os.Stat(filepath.Dir(link.path)); err != nil {
			t.Fatal(err)
		}
	}
}
func TestForeignAndModifiedStateBlocksMutation(t *testing.T) {
	for _, kind := range []string{"unknown", "changed", "privileged-receipt", "external-link", "foreign-launcher", "receipt-traversal"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			f.install(t, current)
			switch kind {
			case "unknown":
				os.WriteFile(filepath.Join(f.e.app, "personal"), []byte("keep"), 0o644)
			case "changed":
				os.WriteFile(filepath.Join(f.e.app, "app/bin/payload"), []byte("changed"), 0o644)
			case "privileged-receipt":
				os.Chmod(filepath.Join(f.e.app, "receipt.json"), os.ModeSetuid|0o644)
			case "external-link":
				os.Symlink(f.artifact, filepath.Join(f.e.app, "outside"))
			case "foreign-launcher":
				os.Remove(f.e.links[0].path)
				os.Symlink("/unrelated", f.e.links[0].path)
			case "receipt-traversal":
				data, _ := os.ReadFile(filepath.Join(f.e.app, "receipt.json"))
				var r Receipt
				json.Unmarshal(data, &r)
				r.Entries["../victim"] = Entry{Type: "file", Mode: new(uint32(0o644)), SHA256: f.digest}
				data, _ = json.Marshal(r)
				writeFile(f.e.journal, data, 0o644)
			}
			if _, err := f.e.Uninstall(t.Context()); err == nil {
				t.Fatal("unsafe removal accepted")
			}
			if !exists(f.e.app) {
				t.Fatal("bundle deleted")
			}
		})
	}
}
func TestBadArtifactAndExtractionPreserveInstall(t *testing.T) {
	for _, kind := range []string{"digest", "symlink", "not-gzip", "extract", "external-symlink", "parent-symlink"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			path, digest := f.artifact, f.digest
			switch kind {
			case "digest":
				digest = strings.Repeat("0", 64)
			case "symlink":
				path = filepath.Join(f.root, "link")
				os.Symlink(f.artifact, path)
			case "not-gzip":
				data := []byte("plain text archive")
				path = filepath.Join(f.root, "plain.tar.gz")
				os.WriteFile(path, data, 0o644)
				sum := sha256.Sum256(data)
				digest = hex.EncodeToString(sum[:])
			case "extract":
				f.install(t, current)
				f.e.extract = func(context.Context, string, string) error { return errors.New("injected extraction failure") }
			case "external-symlink":
				native := f.e.extract
				f.e.extract = func(ctx context.Context, a, d string) error {
					if err := native(ctx, a, d); err != nil {
						return err
					}
					return os.Symlink(f.artifact, filepath.Join(d, "external"))
				}
			case "parent-symlink":
				os.Mkdir(filepath.Join(f.root, "elsewhere"), 0o755)
				os.Symlink(filepath.Join(f.root, "elsewhere"), filepath.Join(f.root, "usr"))
			}
			if _, err := f.e.Apply(t.Context(), path, digest, newer); err == nil {
				t.Fatal("invalid apply accepted")
			}
			if kind == "extract" {
				s, err := f.e.Status()
				if err != nil || s.Version != current || !s.Integrated {
					t.Fatalf("previous state: %+v %v", s, err)
				}
			} else if exists(f.e.app) {
				t.Fatal("invalid bundle published")
			}
			stages, _ := filepath.Glob(filepath.Join(filepath.Dir(f.e.app), ".jetbrains-toolbox-*"))
			if len(stages) != 0 {
				t.Fatalf("failed apply left staging: %v", stages)
			}
		})
	}
}
func TestRestrictiveUmask(t *testing.T) {
	f := setup(t)
	old := unix.Umask(0o077)
	defer unix.Umask(old)
	f.install(t, current)
	for _, link := range f.e.links {
		for parent := filepath.Dir(link.path); parent != f.root; parent = filepath.Dir(parent) {
			info, err := os.Stat(parent)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o755 {
				t.Fatalf("%s: %o", parent, info.Mode().Perm())
			}
		}
	}
	for name, want := range map[string]os.FileMode{"app/bin": 0o755, "app/bin/jetbrains-toolbox": 0o755, "app/bin/payload": 0o644, "launcher": 0o755} {
		info, err := os.Stat(filepath.Join(f.e.app, name))
		if err != nil || info.Mode().Perm() != want {
			t.Fatalf("%s: %v %v", name, info, err)
		}
	}
}
func TestIncompleteIntegrationRemoval(t *testing.T) {
	for index, name := range []string{"launcher", "desktop"} {
		t.Run(name, func(t *testing.T) {
			f := setup(t)
			failed := filepath.Dir(f.e.links[index].path)
			f.e.mkdir = func(path string, mode os.FileMode) error {
				if path == failed {
					return errors.New("injected mkdir failure")
				}
				return os.Mkdir(path, mode)
			}
			if _, err := f.e.Apply(t.Context(), f.artifact, f.digest, current); err == nil {
				t.Fatal("failure lost")
			}
			s, err := f.e.Status()
			if err != nil || !s.Verified || s.Integrated {
				t.Fatalf("partial state: %+v %v", s, err)
			}
			f.e.mkdir = os.Mkdir
			s, err = f.e.Uninstall(t.Context())
			if err != nil || s.Installed {
				t.Fatalf("remove partial: %+v %v", s, err)
			}
			if exists(failed) {
				t.Fatal("uninstall created directory")
			}
		})
	}
}
func TestRemovalRecovery(t *testing.T) {
	for _, kind := range []string{"integration", "sync", "payload", "final-directory"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			f.install(t, current)
			failed := f.e.links[1].path
			if kind == "payload" {
				failed = filepath.Join(f.e.removing, "app/bin/payload")
			}
			if kind == "final-directory" {
				failed = f.e.removing
			}
			if kind == "sync" {
				f.e.syncDir = func(string) error { return errors.New("sync failed") }
			} else {
				f.e.remove = func(path string) error {
					if path == failed {
						return errors.New("remove failed")
					}
					return os.Remove(path)
				}
			}
			if _, err := f.e.Uninstall(t.Context()); err == nil {
				t.Fatal("failure lost")
			}
			f.e.syncDir = syncDirectory
			f.e.remove = os.Remove
			s, err := f.e.Status()
			if err != nil || !s.Verified {
				t.Fatalf("retry state: %+v %v", s, err)
			}
			if kind == "payload" || kind == "final-directory" {
				if !s.CleanupPending || !exists(f.e.journal) {
					t.Fatal("journal lost")
				}
				if _, err := f.e.Apply(t.Context(), f.artifact, f.digest, current); err == nil {
					t.Fatal("apply during removal accepted")
				}
			}
			s, err = f.e.Uninstall(t.Context())
			if err != nil || s.Installed || exists(f.e.journal) || exists(f.e.removing) {
				t.Fatalf("retry: %+v %v", s, err)
			}
		})
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func respond(data []byte) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), Header: http.Header{}}
}
func metadata(version string, size int, checksumLink string) map[string]any {
	linux := map[string]any{"link": sourceURL(version), "size": size, "checksumLink": checksumLink}
	release := map[string]any{"build": version, "version": "3.8", "type": "release", "date": "2026-09-14", "downloads": map[string]any{"linux": linux, "windows": map[string]any{"link": "https://download.jetbrains.com/toolbox/jetbrains-toolbox-" + version + ".exe"}}}
	return map[string]any{"TBA": []any{release}}
}
func TestPrepareAndMetadataBoundaries(t *testing.T) {
	for _, kind := range []string{"valid", "eap", "foreign-url", "digest", "checksum-name", "multiple", "short", "excess", "bad-size"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			payload, _ := os.ReadFile(f.artifact)
			meta := metadata(current, len(payload), sourceURL(current)+".sha256")
			release := meta["TBA"].([]any)[0].(map[string]any)
			linux := release["downloads"].(map[string]any)["linux"].(map[string]any)
			checksum := f.digest + " *jetbrains-toolbox-" + current + ".tar.gz\n"
			switch kind {
			case "eap":
				release["type"] = "eap"
			case "foreign-url":
				linux["link"] = "https://example.invalid/app.tar.gz"
			case "digest":
				checksum = strings.Repeat("0", 64) + " *jetbrains-toolbox-" + current + ".tar.gz\n"
			case "checksum-name":
				checksum = f.digest + " *jetbrains-toolbox-" + previous + ".tar.gz\n"
			case "multiple":
				meta["TBA"] = []any{release, release}
			case "short":
				payload = payload[:len(payload)-1]
			case "excess":
				payload = append(payload, 0)
			case "bad-size":
				linux["size"] = true
			}
			f.e.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
				switch {
				case r.URL.Host == "data.services.jetbrains.com":
					if r.URL.Query().Get("latest") != "true" || r.URL.Query().Get("code") != "TBA" {
						t.Fatalf("unexpected metadata query: %s", r.URL)
					}
					data, _ := json.Marshal(meta)
					return respond(data), nil
				case strings.HasSuffix(r.URL.Path, ".sha256"):
					return respond([]byte(checksum)), nil
				}
				return respond(payload), nil
			})
			a, err := f.e.Prepare(t.Context(), f.root, "")
			if kind == "valid" {
				if err != nil || a.SHA256 != f.digest || a.SchemaVersion != 1 || a.Version != current || !exists(a.Path) {
					t.Fatalf("prepare: %+v %v", a, err)
				}
			} else {
				if err == nil {
					t.Fatal("invalid preparation accepted")
				}
				stages, _ := filepath.Glob(filepath.Join(f.root, "jetbrains-toolbox-*"))
				if len(stages) != 0 {
					t.Fatal("failed preparation left staging")
				}
			}
			if exists(f.e.app) {
				t.Fatal("prepare installed application")
			}
		})
	}
}
func TestOfficialURLsAndChecksums(t *testing.T) {
	for _, raw := range []string{"http://download.jetbrains.com/toolbox/a.tar.gz", "https://download.jetbrains.com.evil.invalid/toolbox/a.tar.gz", "https://download.jetbrains.com/idea/a.tar.gz", "https://user@download.jetbrains.com/toolbox/a.tar.gz", "https://data.services.jetbrains.com/products/code"} {
		if officialURL(raw) {
			t.Fatalf("unsafe URL accepted: %s", raw)
		}
	}
	for _, raw := range []string{sourceURL(current), "https://download-cdn.jetbrains.com/toolbox/jetbrains-toolbox-" + current + ".tar.gz", releasesURL(""), releasesURL(current)} {
		if !officialURL(raw) {
			t.Fatalf("official URL rejected: %s", raw)
		}
	}
	digest := strings.Repeat("a", 64)
	for _, line := range []string{digest + " *jetbrains-toolbox-" + current + ".tar.gz\n", strings.ToUpper(digest) + "  jetbrains-toolbox-" + current + ".tar.gz"} {
		if got, err := parseChecksum([]byte(line), current); err != nil || got != digest {
			t.Fatalf("checksum %q: %q %v", line, got, err)
		}
	}
	for _, line := range []string{"", digest, digest + " *other.tar.gz", "xyz *jetbrains-toolbox-" + current + ".tar.gz", digest + " *jetbrains-toolbox-" + current + ".tar.gz extra"} {
		if _, err := parseChecksum([]byte(line), current); err == nil {
			t.Fatalf("checksum accepted: %q", line)
		}
	}
	if !older(previous, current) || older(current, previous) || older(current, current) || !older("3.8.0.9999", current) {
		t.Fatal("build comparison")
	}
}

func TestCancellationWhileWaitingForLock(t *testing.T) {
	f := setup(t)
	held, err := f.e.lock(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	lock, err := f.e.lock(ctx)
	if lock != nil {
		lock.Close()
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("blocked lock ignored cancellation: %v", err)
	}
}

// The launcher records the autostart opt-out only before the first launch and
// otherwise hands the user's settings and arguments to Toolbox unchanged.
func TestLauncherSeedsAutostartOptOut(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "app/bin/jetbrains-toolbox")
	if err := os.MkdirAll(filepath.Dir(app), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(app, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$LAUNCH_LOG\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "launcher")
	if err := writeFile(script, []byte(strings.ReplaceAll(launcher, "/opt/jetbrains-toolbox/app/bin/jetbrains-toolbox", app)), 0o755); err != nil {
		t.Fatal(err)
	}
	if path, err := exec.LookPath("shellcheck"); err == nil {
		if out, err := exec.CommandContext(t.Context(), path, "--shell=sh", script).CombinedOutput(); err != nil {
			t.Fatalf("shellcheck: %s %v", out, err)
		}
	}
	launch := func(home, data string, args ...string) string {
		t.Helper()
		log := filepath.Join(root, "launch.log")
		os.Remove(log)
		cmd := exec.CommandContext(t.Context(), script, args...)
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "LAUNCH_LOG=" + log}
		if data != "" {
			cmd.Env = append(cmd.Env, "XDG_DATA_HOME="+data)
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("launcher: %s %v", out, err)
		}
		logged, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		return string(logged)
	}
	home := filepath.Join(root, "home")
	if got := launch(home, "", "--minimize", "jetbrains://open"); got != "--minimize\njetbrains://open\n" {
		t.Fatalf("arguments changed: %q", got)
	}
	settings := filepath.Join(home, ".local/share/JetBrains/Toolbox/.settings.json")
	data, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	var seeded struct {
		Autostart *bool `json:"autostart"`
	}
	if err := json.Unmarshal(data, &seeded); err != nil || seeded.Autostart == nil || *seeded.Autostart {
		t.Fatalf("seeded settings %q: %v", data, err)
	}
	if err := os.WriteFile(settings, []byte(`{"autostart": true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	launch(home, "")
	if after, _ := os.ReadFile(settings); string(after) != `{"autostart": true}` {
		t.Fatalf("existing settings changed: %q", after)
	}
	xdg := filepath.Join(root, "xdg")
	launch(home, xdg)
	if _, err := os.Stat(filepath.Join(xdg, "JetBrains/Toolbox/.settings.json")); err != nil {
		t.Fatal("XDG_DATA_HOME ignored:", err)
	}
}
