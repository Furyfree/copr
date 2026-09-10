package copilot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type fixture struct {
	e                                    *Installer
	out                                  bytes.Buffer
	data                                 []byte
	digest                               string
	metadata                             map[string]any
	installed                            bool
	version, release, header, comparison string
	dnfCalls                             [][]string
	failDNF                              bool
	skipDNF                              bool
	artifactIdentity                     string
}

func setup(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{e: New(), data: []byte("official RPM fixture"), version: "1.1.17", release: "1", header: strings.Repeat("a", 64), comparison: "-1"}
	sum := sha256.Sum256(f.data)
	f.digest = hex.EncodeToString(sum[:])
	f.e.release = func() (Artifact, error) {
		return Artifact{1, "1.1.17", "github", "x86_64", f.digest, "", sourceURL("1.1.17")}, nil
	}
	f.e.temp = t.TempDir()
	f.e.out = &f.out
	f.e.root = func() bool { return true }
	f.e.platform = func() error { return nil }
	f.metadata = map[string]any{"draft": false, "prerelease": false, "tag_name": "v1.1.17", "assets": []any{map[string]any{"name": assetName, "browser_download_url": sourceURL("1.1.17"), "digest": "sha256:" + f.digest}}}
	f.e.client.Transport = transport(func(r *http.Request) (*http.Response, error) {
		data := f.data
		if r.URL.Host == "api.github.com" {
			data, _ = json.Marshal(f.metadata)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), Header: http.Header{}}, nil
	})
	absent := exec.Command("sh", "-c", "exit 1").Run()
	f.e.run = func(_ context.Context, name string, args []string, _ bool) ([]byte, error) {
		if name == "dnf" {
			f.dnfCalls = append(f.dnfCalls, slices.Clone(args))
			if f.failDNF {
				return nil, errors.New("DNF failed")
			}
			if !f.skipDNF {
				f.installed = !slices.Contains(args, "remove")
				f.version = "1.1.17"
				f.release = "1"
			}
			return nil, nil
		}
		if name != "rpm" {
			return nil, fmt.Errorf("unexpected native tool %s", name)
		}
		if args[0] == "--checksig" {
			return nil, nil
		}
		if args[0] == "--eval" {
			return []byte(f.comparison + "\n"), nil
		}
		if args[0] == "-q" && !f.installed {
			return nil, absent
		}
		if args[2] == "%{SHA256HEADER}\n" {
			return []byte(f.header + "\n"), nil
		}
		if args[0] == "-qp" {
			if f.artifactIdentity != "" {
				return []byte(f.artifactIdentity), nil
			}
			return []byte("github\t1.1.17\t1\tx86_64\tProprietary\tTauri Copilot Application\t0\n"), nil
		}
		return []byte(fmt.Sprintf("github\t%s\t%s\tx86_64\tProprietary\tTauri Copilot Application\t0\n", f.version, f.release)), nil
	}
	return f
}
func TestParse(t *testing.T) {
	for _, args := range [][]string{{"apply"}, {"prepare"}, {"status", "--assumeyes"}, {"status", "--app-version", "1.0.0"}, {"install", "update"}, {"prepare", "--directory", "/tmp", "-y"}, {"install", "--rpm", "a"}, {"apply", "--rpm", "a", "--app-version", "1.0.0", "--sha256", "bad"}, {"install", "--app-version", "1.0.0';evil"}, {"--", "bad"}} {
		if _, err := Parse(args); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
	for _, args := range [][]string{nil, {"help"}, {"version"}, {"status"}, {"--app-version", "1.1.17", "install", "-y"}, {"prepare", "--directory", "/tmp"}, {"apply", "--rpm", "a", "--app-version", "1.1.17", "--sha256", strings.Repeat("a", 64)}} {
		if _, err := Parse(args); err != nil {
			t.Errorf("rejected %v: %v", args, err)
		}
	}
}
func TestReadOnlyCommands(t *testing.T) {
	f := setup(t)
	f.e.client.Transport = transport(func(*http.Request) (*http.Response, error) {
		t.Fatal("read-only command used network")
		return nil, nil
	})
	f.e.temp = filepath.Join(f.e.temp, "nonexistent")
	for _, command := range []string{"help", "version", "status"} {
		if err := f.e.Execute(t.Context(), Options{Command: command}); err != nil {
			t.Fatal(err)
		}
	}
	if !strings.Contains(f.out.String(), "Installed GitHub Copilot: not installed") {
		t.Fatal(f.out.String())
	}
	if len(f.dnfCalls) != 0 {
		t.Fatal("status mutated packages")
	}
}
func TestPrepareAndApply(t *testing.T) {
	f := setup(t)
	if err := f.e.Execute(t.Context(), Options{Command: "prepare", Directory: f.e.temp}); err != nil {
		t.Fatal(err)
	}
	var a Artifact
	if err := json.Unmarshal(f.out.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	if a.SchemaVersion != 1 || a.Name != "github" || a.SHA256 != f.digest || a.Source != sourceURL("1.1.17") {
		t.Fatalf("artifact %+v", a)
	}
	if len(f.dnfCalls) != 0 {
		t.Fatal("prepare ran DNF")
	}
	f.e.client.Transport = transport(func(*http.Request) (*http.Response, error) { t.Fatal("apply used network"); return nil, nil })
	if err := f.e.Execute(t.Context(), Options{Command: "apply", RPM: a.Path, SHA256: a.SHA256, Version: a.Version, Yes: true}); err != nil {
		t.Fatal(err)
	}
	if len(f.dnfCalls) != 1 {
		t.Fatal(f.dnfCalls)
	}
	args := f.dnfCalls[0]
	if !slices.Contains(args, "--setopt=localpkg_gpgcheck=0") || args[0] != "-y" || args[1] != "install" || args[len(args)-1] == a.Path {
		t.Fatalf("unverified DNF arguments: %v", args)
	}
	if _, err := os.Stat(args[len(args)-1]); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("privileged staging remains")
	}
	if _, err := os.Stat(a.Path); err != nil {
		t.Fatal("caller artifact removed")
	}
}
func TestMetadataFailures(t *testing.T) {
	for _, kind := range []string{"draft", "prerelease", "missing-draft", "tag", "version", "url", "digest", "duplicate"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			asset := f.metadata["assets"].([]any)[0].(map[string]any)
			version := ""
			switch kind {
			case "draft":
				f.metadata["draft"] = true
			case "prerelease":
				f.metadata["prerelease"] = true
			case "missing-draft":
				delete(f.metadata, "draft")
			case "tag":
				f.metadata["tag_name"] = "malformed"
			case "version":
				version = "1.0.0"
			case "url":
				asset["browser_download_url"] = "https://example.invalid/pkg.rpm"
			case "digest":
				asset["digest"] = "missing"
			case "duplicate":
				f.metadata["assets"] = []any{asset, asset}
			}
			if err := f.e.Execute(t.Context(), Options{Command: "prepare", Directory: f.e.temp, Version: version}); err == nil {
				t.Fatal("invalid metadata accepted")
			}
			entries, _ := os.ReadDir(f.e.temp)
			if len(entries) != 0 {
				t.Fatal("failure left preparation directory")
			}
			if len(f.dnfCalls) != 0 {
				t.Fatal("failure called DNF")
			}
		})
	}
}
func TestArtifactIdentityFailures(t *testing.T) {
	for index := range 7 {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			f := setup(t)
			fields := []string{"github", "1.1.17", "1", "x86_64", "Proprietary", "Tauri Copilot Application", "0"}
			fields[index] = "invalid!"
			f.artifactIdentity = strings.Join(fields, "\t") + "\n"
			if err := f.e.Execute(t.Context(), Options{Command: "install"}); err == nil {
				t.Fatal("invalid RPM accepted")
			}
			if len(f.dnfCalls) != 0 {
				t.Fatal("invalid RPM reached DNF")
			}
		})
	}
}
func TestDowngradeAndSameVersionHeader(t *testing.T) {
	for _, kind := range []string{"downgrade", "same-header-mismatch", "DNF-failure", "postcheck-version"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			f.installed = true
			f.version = "1.0.0"
			switch kind {
			case "downgrade":
				f.comparison = "1"
			case "same-header-mismatch":
				f.version = "1.1.17"
				f.comparison = "0"
				native := f.e.run
				f.e.run = func(ctx context.Context, name string, args []string, interactive bool) ([]byte, error) {
					if name == "rpm" && args[0] == "-q" && args[2] == "%{SHA256HEADER}\n" {
						return []byte(strings.Repeat("b", 64) + "\n"), nil
					}
					return native(ctx, name, args, interactive)
				}
			case "DNF-failure":
				f.failDNF = true
			case "postcheck-version":
				f.skipDNF = true
			}
			if err := f.e.Execute(t.Context(), Options{Command: "install"}); err == nil {
				t.Fatal("invalid transaction accepted")
			}
			if (kind == "downgrade" || kind == "same-header-mismatch") && len(f.dnfCalls) != 0 {
				t.Fatal("unsafe transaction reached DNF")
			}
		})
	}
}
func TestMutationGuardsAndRemoval(t *testing.T) {
	f := setup(t)
	if err := f.e.Execute(t.Context(), Options{Command: "update"}); err == nil {
		t.Fatal("update of absent package accepted")
	}
	f.e.root = func() bool { return false }
	for _, command := range []string{"install", "update", "apply", "uninstall"} {
		if err := f.e.Execute(t.Context(), Options{Command: command}); err == nil {
			t.Fatal("unprivileged mutation accepted")
		}
	}
	f.e.root = func() bool { return true }
	f.installed = true
	if err := f.e.Execute(t.Context(), Options{Command: "uninstall", Yes: true}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.dnfCalls[0], []string{"-y", "remove", "github"}) {
		t.Fatal(f.dnfCalls)
	}
	f.installed = true
	f.skipDNF = true
	if err := f.e.Execute(t.Context(), Options{Command: "uninstall"}); err == nil {
		t.Fatal("removal falsely succeeded")
	}
}
func TestInputAndRedirectTrust(t *testing.T) {
	f := setup(t)
	input := filepath.Join(f.e.temp, "prepared.rpm")
	if err := os.WriteFile(input, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := f.e.Execute(t.Context(), Options{Command: "apply", RPM: input, SHA256: f.digest, Version: "1.1.17"}); err == nil {
		t.Fatal("changed input accepted")
	}
	linked := input + ".link"
	os.Symlink(input, linked)
	if err := f.e.Execute(t.Context(), Options{Command: "apply", RPM: linked, SHA256: f.digest, Version: "1.1.17"}); err == nil {
		t.Fatal("symlink input accepted")
	}
	for _, raw := range []string{"http://github.com/github/app/releases/download/a", "https://github.com.evil.invalid/github/app/releases/download/a", "https://github.com/other/repo/releases/download/a"} {
		if officialURL(raw) {
			t.Fatal("unsafe URL accepted")
		}
	}
	if len(f.dnfCalls) != 0 {
		t.Fatal("bad input mutated packages")
	}
}
