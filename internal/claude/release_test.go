package claude

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixtureIndex(version, digest string, size int) []byte {
	return []byte(fmt.Sprintf("Package: claude-desktop\nVersion: %s\nArchitecture: amd64\nFilename: pool/main/c/claude-desktop/claude-desktop_%s_amd64.deb\nSize: %d\nSHA256: %s\nDescription: Claude\n A continued description.\n\n", version, version, size, digest))
}
func fixtureRelease(index []byte) []byte {
	hash := sha256.Sum256(index)
	return []byte(fmt.Sprintf("Origin: Anthropic\nSuite: stable\nCodename: stable\nArchitectures: amd64 arm64\nComponents: main\nDate: %s\nValid-Until: %s\nSHA256:\n %x %d main/binary-amd64/Packages\n", time.Now().UTC().Add(-time.Hour).Format(time.RFC1123Z), time.Now().UTC().Add(time.Hour).Format(time.RFC1123), hash, len(index)))
}
func serveRelease(f fixture, version string, index, payload, release []byte) *int {
	calls := new(int)
	f.e.verify = func(_ context.Context, data []byte) ([]byte, error) {
		if string(data) != "signed fixture" {
			return nil, errors.New("signature rejected")
		}
		return release, nil
	}
	f.e.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		*calls++
		var data []byte
		switch r.URL.String() {
		case releaseURL:
			data = []byte("signed fixture")
		case indexURL:
			data = index
		case sourceURL(version):
			data = payload
		default:
			return nil, fmt.Errorf("unexpected URL: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), Header: http.Header{}}, nil
	})
	return calls
}
func selectRelease(f fixture, version string) {
	f.e.release = func() (Artifact, error) {
		return Artifact{1, version, "claude-desktop", "x86_64", f.digest, "", sourceURL(version)}, nil
	}
}

func TestPrepareRejectsUntrustedOrDamagedDownloads(t *testing.T) {
	for _, failure := range []string{"valid", "signature", "expired", "origin", "index-drift", "package-digest", "foreign-source", "duplicate-version", "short", "excess", "not-debian"} {
		t.Run(failure, func(t *testing.T) {
			f := setup(t)
			payload := artifactBytes()
			index := fixtureIndex("2.10.0", f.digest, len(payload))
			switch failure {
			case "foreign-source":
				index = bytes.ReplaceAll(index, []byte("pool/main/c/"), []byte("../foreign/"))
			case "package-digest":
				index = bytes.ReplaceAll(index, []byte(f.digest), []byte(strings.Repeat("0", 64)))
			case "duplicate-version":
				index = append(index, index...)
			case "short":
				payload = payload[:len(payload)-1]
			case "excess":
				payload = append(payload, 0)
			case "not-debian":
				payload[0] = 0
				hash := sha256.Sum256(payload)
				index = fixtureIndex("2.10.0", fmt.Sprintf("%x", hash), len(payload))
			}
			release := fixtureRelease(index)
			switch failure {
			case "expired":
				metadata, err := fields(string(release))
				if err != nil {
					t.Fatal(err)
				}
				release = []byte(strings.Replace(string(release), metadata["Valid-Until"], time.Now().UTC().Add(-time.Minute).Format(time.RFC1123), 1))
			case "origin":
				release = bytes.ReplaceAll(release, []byte("Origin: Anthropic"), []byte("Origin: Impostor"))
			case "index-drift":
				index = bytes.Replace(index, []byte("Description: Claude"), []byte("Description: Edited"), 1)
			}
			calls := serveRelease(f, "2.10.0", index, payload, release)
			if failure == "signature" {
				f.e.verify = func(context.Context, []byte) ([]byte, error) { return nil, errors.New("bad signature") }
			}
			a, err := f.e.Prepare(t.Context(), f.root, "2.10.0")
			if failure == "valid" {
				if err != nil || a.Version != "2.10.0" || a.SHA256 != f.digest || !exists(a.Path) {
					t.Fatalf("prepare: %+v %v", a, err)
				}
			} else {
				if err == nil {
					t.Fatal("invalid preparation accepted")
				}
				stages, _ := filepath.Glob(filepath.Join(f.root, "claude-desktop-*"))
				if len(stages) != 0 {
					t.Fatalf("failed preparation retained downloads: %v", stages)
				}
				if (failure == "signature" || failure == "expired" || failure == "origin") && *calls != 1 {
					t.Fatal("untrusted release fetched the package index")
				}
				if (failure == "index-drift" || failure == "foreign-source" || failure == "duplicate-version") && *calls != 2 {
					t.Fatal("invalid index fetched the application")
				}
			}
			if exists(f.e.app) {
				t.Fatal("prepare installed application")
			}
		})
	}
}

func TestSignedIndexSelectsVersionWithoutDependingOnOrder(t *testing.T) {
	f := setup(t)
	for _, version := range []string{"", "2.9.0", "2.10.0", "2.11.0"} {
		index := append(fixtureIndex("2.10.0", f.digest, 256), fixtureIndex("2.9.0", f.digest, 256)...)
		serveRelease(f, "2.10.0", index, nil, fixtureRelease(index))
		a, _, err := f.e.resolve(t.Context(), version)
		if version == "2.11.0" {
			if err == nil {
				t.Fatal("absent release accepted")
			}
			continue
		}
		want := version
		if want == "" {
			want = "2.10.0"
		}
		if err != nil || a.Version != want {
			t.Fatalf("selected %q: %+v %v", version, a, err)
		}
	}
}

func TestAutomaticInstallIsPinnedAndRepairsOffline(t *testing.T) {
	f := setup(t)
	selectRelease(f, "2.10.0")
	index := fixtureIndex("2.10.0", f.digest, 256)
	calls := serveRelease(f, "2.10.0", index, artifactBytes(), fixtureRelease(index))
	s, err := f.e.Install(t.Context())
	if err != nil || !s.Integrated || !s.Verified || s.Version != "2.10.0" || *calls != 3 {
		t.Fatalf("install: %+v %v (%d requests)", s, err, *calls)
	}
	info, err := os.Stat(filepath.Join(f.e.app, "receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	f.e.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
		t.Fatal("current installation used the network")
		return nil, errors.New("offline")
	})
	s, err = f.e.Install(t.Context())
	after, statErr := os.Stat(filepath.Join(f.e.app, "receipt.json"))
	if err != nil || statErr != nil || !s.Integrated || !os.SameFile(info, after) {
		t.Fatalf("rerun mutated installed state: %+v %v %v", s, err, statErr)
	}
	if err := os.Remove(f.e.links[0].path); err != nil {
		t.Fatal(err)
	}
	if s, err := f.e.Install(t.Context()); err != nil || !s.Integrated {
		t.Fatalf("offline repair: %+v %v", s, err)
	}
}

func TestFailedAutomaticUpdatePreservesPreviousRelease(t *testing.T) {
	for _, failure := range []string{"drift", "network", "extract", "cancel"} {
		t.Run(failure, func(t *testing.T) {
			f := setup(t)
			f.install(t, "2.9.0")
			selectRelease(f, "2.10.0")
			before, _ := os.ReadFile(filepath.Join(f.e.app, "receipt.json"))
			digest := f.digest
			if failure == "drift" {
				digest = strings.Repeat("b", 64)
			}
			index := fixtureIndex("2.10.0", digest, 256)
			calls := serveRelease(f, "2.10.0", index, artifactBytes(), fixtureRelease(index))
			extract := f.e.extract
			ctx := t.Context()
			switch failure {
			case "network":
				f.e.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })
			case "extract":
				f.e.extract = func(context.Context, string, string, string) error { return errors.New("extraction failed") }
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
			if failure == "drift" && *calls != 2 {
				t.Fatal("metadata drift downloaded the application")
			}
			stages, _ := filepath.Glob(filepath.Join(filepath.Dir(f.e.app), ".claude-desktop-download-*"))
			if len(stages) != 0 {
				t.Fatalf("failed update left downloads: %v", stages)
			}
			f.e.extract = extract
			index = fixtureIndex("2.10.0", f.digest, 256)
			serveRelease(f, "2.10.0", index, artifactBytes(), fixtureRelease(index))
			if s, err := f.e.Install(t.Context()); err != nil || s.Version != "2.10.0" || !s.Integrated {
				t.Fatalf("retry: %+v %v", s, err)
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
		strings.Replace(string(data), "https://downloads.claude.ai/", "https://example.invalid/", 1),
		strings.Replace(string(data), `"arch":"x86_64"`, `"arch":"aarch64"`, 1),
		strings.Replace(string(data), `"name":"claude-desktop"`, `"name":"other"`, 1),
	} {
		if _, err := ParseRelease([]byte(bad)); err == nil {
			t.Errorf("accepted invalid pin: %s", bad)
		}
	}
}
