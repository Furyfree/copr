package toolbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func selectFixtureRelease(f fixture, version string) {
	f.e.release = func() (Artifact, error) {
		return Artifact{1, version, "jetbrains-toolbox", "x86_64", f.digest, "", sourceURL(version)}, nil
	}
}

func serveFixtureRelease(t *testing.T, f fixture, version string, changed bool) *int {
	t.Helper()
	calls := new(int)
	payload, err := os.ReadFile(f.artifact)
	if err != nil {
		t.Fatal(err)
	}
	f.e.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		*calls++
		switch {
		case r.URL.Host == "data.services.jetbrains.com":
			if r.URL.Query().Get("build") != version || r.URL.Query().Has("latest") {
				t.Fatalf("requested unpinned release: %s", r.URL)
			}
			data, _ := json.Marshal(metadata(version, len(payload), sourceURL(version)+".sha256"))
			return respond(data), nil
		case r.URL.String() == sourceURL(version)+".sha256":
			digest := f.digest
			if changed {
				digest = strings.Repeat("b", 64)
			}
			return respond([]byte(digest + " *jetbrains-toolbox-" + version + ".tar.gz\n")), nil
		case r.URL.String() == sourceURL(version):
			return respond(payload), nil
		}
		t.Fatalf("unexpected request: %s", r.URL)
		return nil, nil
	})
	return calls
}

func TestAutomaticInstallAndOfflineRerun(t *testing.T) {
	f := setup(t)
	selectFixtureRelease(f, current)
	calls := serveFixtureRelease(t, f, current, false)
	s, err := f.e.Install(t.Context())
	if err != nil || !s.Verified || !s.Integrated || s.Version != current || *calls != 3 {
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
	stages, _ := filepath.Glob(filepath.Join(filepath.Dir(f.e.app), ".jetbrains-toolbox-download-*"))
	if len(stages) != 0 {
		t.Fatalf("download stages left: %v", stages)
	}
}

func TestAutomaticUpdateFailureAndRetry(t *testing.T) {
	for _, failure := range []string{"metadata", "network", "extract", "cancel"} {
		t.Run(failure, func(t *testing.T) {
			f := setup(t)
			f.install(t, previous)
			selectFixtureRelease(f, current)
			before, _ := os.ReadFile(filepath.Join(f.e.app, "receipt.json"))
			calls := serveFixtureRelease(t, f, current, failure == "metadata")
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
			if failure == "metadata" && *calls != 2 {
				t.Fatalf("changed metadata downloaded application (%d requests)", *calls)
			}
			stages, _ := filepath.Glob(filepath.Join(filepath.Dir(f.e.app), ".jetbrains-toolbox-*"))
			if len(stages) != 0 {
				t.Fatalf("failure left downloads: %v", stages)
			}
			f.e.extract = extract
			serveFixtureRelease(t, f, current, false)
			s, err := f.e.Install(t.Context())
			if err != nil || s.Version != current || !s.Integrated {
				t.Fatalf("retry: %+v %v", s, err)
			}
		})
	}
}

func TestAutomaticInstallRefusesConflictsBeforeNetwork(t *testing.T) {
	for _, conflict := range []string{"downgrade", "digest", "modified", "foreign", "removal"} {
		t.Run(conflict, func(t *testing.T) {
			f := setup(t)
			selectFixtureRelease(f, current)
			switch conflict {
			case "downgrade":
				f.install(t, newer)
			case "digest":
				f.install(t, current)
				f.digest = strings.Repeat("b", 64)
				selectFixtureRelease(f, current)
			case "modified":
				f.install(t, current)
				if err := os.WriteFile(filepath.Join(f.e.app, "app/bin/payload"), []byte("foreign"), 0o644); err != nil {
					t.Fatal(err)
				}
			case "foreign":
				if err := os.Mkdir(f.e.app, 0o755); err != nil {
					t.Fatal(err)
				}
			case "removal":
				f.install(t, current)
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
		strings.Replace(string(data), "https://download.jetbrains.com/", "https://example.invalid/", 1),
		strings.Replace(string(data), `"arch":"x86_64"`, `"arch":"aarch64"`, 1),
		strings.Replace(string(data), `"name":"jetbrains-toolbox"`, `"name":"other"`, 1),
		strings.Replace(string(data), a.Version, "3.8", 1),
	} {
		if _, err := ParseRelease([]byte(bad)); err == nil {
			t.Errorf("accepted invalid pin: %s", bad)
		}
	}
}
