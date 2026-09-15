package wowup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func selectFixtureRelease(f fixture, version string) {
	f.e.release = func() (Artifact, error) {
		return Artifact{1, version, "wowup-cf", "x86_64", f.digest, "", sourceURL(version)}, nil
	}
}

func serveFixtureRelease(t *testing.T, f fixture, version string, changed bool) *int {
	t.Helper()
	calls := new(int)
	f.e.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		*calls++
		data := artifactBytes()
		if r.URL.Host == "api.github.com" {
			if r.URL.Path != "/repos/WowUp/WowUp.CF/releases/tags/v"+version {
				t.Fatalf("requested unpinned release: %s", r.URL)
			}
			digest := f.digest
			if changed {
				digest = strings.Repeat("b", 64)
			}
			data, _ = json.Marshal(map[string]any{"tag_name": "v" + version, "draft": false, "prerelease": false, "assets": []any{map[string]any{"name": "WowUp-CF-" + version + ".AppImage", "browser_download_url": sourceURL(version), "digest": "sha256:" + digest, "size": len(data)}}})
		} else if r.URL.String() != sourceURL(version) {
			t.Fatalf("unexpected artifact URL: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), Header: http.Header{}}, nil
	})
	return calls
}

func TestAutomaticInstallAndOfflineRerun(t *testing.T) {
	f := setup(t)
	selectFixtureRelease(f, "2.23.1")
	calls := serveFixtureRelease(t, f, "2.23.1", false)
	s, err := f.e.Install(t.Context())
	if err != nil || !s.Verified || !s.Integrated || s.Version != "2.23.1" || *calls != 2 {
		t.Fatalf("install: %+v %v (%d requests)", s, err, *calls)
	}
	info, err := os.Stat(filepath.Join(f.e.app, "receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	f.e.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
		t.Fatal("current installation used network")
		return nil, errors.New("offline")
	})
	s, err = f.e.Install(t.Context())
	after, statErr := os.Stat(filepath.Join(f.e.app, "receipt.json"))
	if err != nil || statErr != nil || !s.Integrated || !os.SameFile(info, after) {
		t.Fatalf("current installation mutated: %+v %v %v", s, err, statErr)
	}
	if err := os.Remove(f.e.links[0].path); err != nil {
		t.Fatal(err)
	}
	s, err = f.e.Install(t.Context())
	if err != nil || !s.Integrated {
		t.Fatalf("offline link repair: %+v %v", s, err)
	}
	stages, _ := filepath.Glob(filepath.Join(filepath.Dir(f.e.app), ".wowup-cf-download-*"))
	if len(stages) != 0 {
		t.Fatalf("download stages left: %v", stages)
	}
}

func TestAutomaticUpdateFailureAndRetry(t *testing.T) {
	for _, failure := range []string{"metadata", "network", "extract", "cancel"} {
		t.Run(failure, func(t *testing.T) {
			f := setup(t)
			f.install(t, "2.23.0")
			selectFixtureRelease(f, "2.23.1")
			before, _ := os.ReadFile(filepath.Join(f.e.app, "receipt.json"))
			calls := serveFixtureRelease(t, f, "2.23.1", failure == "metadata")
			extract := f.e.extract
			ctx := t.Context()
			switch failure {
			case "network":
				f.e.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })
			case "extract":
				f.e.extract = func(context.Context, string, string) error { return errors.New("extraction failed") }
			case "cancel":
				canceled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = canceled
			}
			if _, err := f.e.Install(ctx); err == nil {
				t.Fatal("failed update reported success")
			}
			after, _ := os.ReadFile(filepath.Join(f.e.app, "receipt.json"))
			if !bytes.Equal(before, after) {
				t.Fatal("failure changed installed release")
			}
			if failure == "metadata" && *calls != 1 {
				t.Fatal("changed metadata downloaded application")
			}
			stages, _ := filepath.Glob(filepath.Join(filepath.Dir(f.e.app), ".wowup-cf-download-*"))
			if len(stages) != 0 {
				t.Fatalf("failure left downloads: %v", stages)
			}
			f.e.extract = extract
			serveFixtureRelease(t, f, "2.23.1", false)
			s, err := f.e.Install(t.Context())
			if err != nil || s.Version != "2.23.1" || !s.Integrated {
				t.Fatalf("retry: %+v %v", s, err)
			}
		})
	}
}

func TestAutomaticInstallRefusesConflictsBeforeNetwork(t *testing.T) {
	for _, conflict := range []string{"downgrade", "digest", "modified", "foreign", "removal"} {
		t.Run(conflict, func(t *testing.T) {
			f := setup(t)
			selectFixtureRelease(f, "2.23.1")
			switch conflict {
			case "downgrade":
				f.install(t, "2.23.2")
			case "digest":
				f.install(t, "2.23.1")
				f.digest = strings.Repeat("b", 64)
				selectFixtureRelease(f, "2.23.1")
			case "modified":
				f.install(t, "2.23.1")
				if err := os.WriteFile(filepath.Join(f.e.app, "app/payload"), []byte("foreign"), 0o644); err != nil {
					t.Fatal(err)
				}
			case "foreign":
				if err := os.Mkdir(f.e.app, 0o755); err != nil {
					t.Fatal(err)
				}
			case "removal":
				f.install(t, "2.23.1")
				// Fail after journaling, rather than while removing the integration links.
				f.e.remove = func(path string) error {
					if strings.HasPrefix(path, f.e.removing) {
						return errors.New("interrupted removal")
					}
					return os.Remove(path)
				}
				if _, err := f.e.Uninstall(t.Context()); err == nil {
					t.Fatal("removal failure missing")
				}
			}
			f.e.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
				t.Fatal("conflicting installation used network")
				return nil, errors.New("offline")
			})
			if _, err := f.e.Install(t.Context()); err == nil {
				t.Fatal("conflict accepted")
			}
		})
	}
}

func TestPackagedReleaseValidation(t *testing.T) {
	a, err := PackagedRelease()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		string(data) + `{}`,
		strings.Replace(string(data), `"schema_version":1`, `"schema_version":2`, 1),
		strings.Replace(string(data), a.SHA256, "missing", 1),
		strings.Replace(string(data), `"path":""`, `"path":"/tmp/app"`, 1),
		strings.Replace(string(data), "https://github.com/", "https://example.invalid/", 1),
		strings.Replace(string(data), `"arch":"x86_64"`, `"arch":"aarch64"`, 1),
		strings.Replace(string(data), `"name":"wowup-cf"`, `"name":"other"`, 1),
	} {
		if _, err := ParseRelease([]byte(bad)); err == nil {
			t.Errorf("accepted invalid pin: %s", bad)
		}
	}
}
