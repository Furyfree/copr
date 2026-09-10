package copilot

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestPackageSelectsAppVersionAndDigest(t *testing.T) {
	for _, changed := range []bool{false, true} {
		f := setup(t)
		if changed {
			f.metadata["assets"].([]any)[0].(map[string]any)["digest"] = "sha256:" + strings.Repeat("b", 64)
		}
		underlying := f.e.client.Transport
		f.e.client.Transport = transport(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host == "api.github.com" && r.URL.Path != "/repos/github/app/releases/tags/v1.1.17" {
				t.Fatalf("package installation followed an unpinned release: %s", r.URL)
			}
			return underlying.RoundTrip(r)
		})
		err := f.e.Execute(t.Context(), Options{Command: "install", Yes: true})
		if changed {
			if err == nil || !strings.Contains(err.Error(), "selected by this package") || len(f.dnfCalls) != 0 {
				t.Fatalf("changed release reached DNF: %v, %v", err, f.dnfCalls)
			}
		} else if err != nil || !f.installed {
			t.Fatalf("pinned installation failed: %v", err)
		}
	}
}

func TestPackagedReleaseValidation(t *testing.T) {
	a, err := packageRelease()
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
		strings.Replace(string(data), `"path":""`, `"path":"/tmp/app.rpm"`, 1),
		strings.Replace(string(data), "https://github.com/", "https://example.invalid/", 1),
		strings.Replace(string(data), `"arch":"x86_64"`, `"arch":"aarch64"`, 1),
		strings.Replace(string(data), `"name":"github"`, `"name":"other"`, 1),
	} {
		if _, err := ParseRelease([]byte(bad)); err == nil {
			t.Errorf("accepted invalid release: %s", bad)
		}
	}
}
