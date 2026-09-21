package packaging

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

const developDigest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func developFixture(t *testing.T, release, sums string) *http.Client {
	t.Helper()
	return &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		var body string
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/tags/develop"):
			body = release
		case strings.HasSuffix(r.URL.Path, "/SHA256SUMS"):
			body = sums
		default:
			t.Fatalf("unexpected metadata fetch %s", r.URL)
			return nil, errors.New("unexpected")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})}
}

func TestResolveDevelopSource(t *testing.T) {
	const archive = "nimbus-0.6.1~dev.20260919020436-vendor.tar.gz"
	release := `{"assets":[
		{"name":"` + archive + `","browser_download_url":"https://github.com/Furyfree/nimbus/releases/download/develop/` + archive + `"},
		{"name":"SHA256SUMS","browser_download_url":"https://github.com/Furyfree/nimbus/releases/download/develop/SHA256SUMS"}]}`
	sums := developDigest + "  " + archive + "\n"
	source, err := resolveDevelopSource(t.Context(), developFixture(t, release, sums), developReleaseURL)
	if err != nil || source.Version != "0.6.1~dev.20260919020436" ||
		source.Asset != archive || source.Digest != developDigest {
		t.Fatalf("source = %+v, %v", source, err)
	}
}

func TestResolveDevelopSourceRefusals(t *testing.T) {
	const archive = "nimbus-0.6.1~dev.20260919020436-vendor.tar.gz"
	base := `{"assets":[
		{"name":"` + archive + `","browser_download_url":"https://github.com/Furyfree/nimbus/releases/download/develop/` + archive + `"},
		{"name":"SHA256SUMS","browser_download_url":"https://github.com/Furyfree/nimbus/releases/download/develop/SHA256SUMS"}]}`
	for _, tc := range []struct {
		name          string
		release, sums string
		want          string
	}{
		{"missing checksums", strings.Replace(base, "SHA256SUMS", "SHA256SUM", 1), developDigest + "  " + archive, "lacks"},
		{"foreign asset URL", strings.Replace(base, "https://github.com/Furyfree/nimbus", "https://example.test/nimbus", 1), developDigest + "  " + archive, "official"},
		{"bad version", strings.ReplaceAll(base, "0.6.1~dev.20260919020436", "0.6.0"), developDigest + "  " + strings.ReplaceAll(archive, "0.6.1~dev.20260919020436", "0.6.0"), "unexpected develop version"},
		{"digest missing", base, "deadbeef  other.tar.gz\n", "no digest"},
		{"short digest", base, "abc  " + archive + "\n", "no digest"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolveDevelopSource(t.Context(), developFixture(t, tc.release, tc.sums), developReleaseURL)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestMetadataRedirects(t *testing.T) {
	for _, tc := range []struct {
		name, target string
		allowed      bool
	}{
		{"release asset", "https://release-assets.githubusercontent.com/fixture/SHA256SUMS", true},
		{"foreign host", "https://example.invalid/SHA256SUMS", false},
		{"HTTP downgrade", "http://release-assets.githubusercontent.com/fixture/SHA256SUMS", false},
		{"credentials", "https://user:password@github.com/Furyfree/nimbus/releases/download/develop/SHA256SUMS", false},
		{"other repository", "https://github.com/other/project/releases/download/develop/SHA256SUMS", false},
		{"unexpected port", "https://github.com:8443/Furyfree/nimbus/releases/download/develop/SHA256SUMS", false},
		{"redirect loop", developReleaseURL, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() == developReleaseURL {
					return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": {tc.target}}, Body: io.NopCloser(strings.NewReader(""))}, nil
				}
				if !tc.allowed {
					t.Fatal("request reached a rejected redirect target")
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("checksum")), Header: http.Header{}}, nil
			})}
			body, err := httpGet(t.Context(), client, developReleaseURL)
			if tc.allowed {
				if err != nil || string(body) != "checksum" {
					t.Fatalf("permitted redirect failed: %q %v", body, err)
				}
			} else if err == nil {
				t.Fatal("unsafe redirect accepted")
			}
		})
	}
}

func TestMetadataSizeLimit(t *testing.T) {
	for _, size := range []int{1 << 20, (1 << 20) + 1} {
		payload := strings.Repeat("x", size)
		client := &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(payload)), Header: http.Header{}}, nil
		})}
		body, err := httpGet(t.Context(), client, developReleaseURL)
		if size == 1<<20 {
			if err != nil || string(body) != payload {
				t.Fatalf("bounded response changed: %v", err)
			}
		} else if err == nil {
			t.Fatal("oversized response silently truncated")
		}
	}
}

func TestDevelopSpec(t *testing.T) {
	recipe := "Name: nimbus\nVersion:        0.6.1~dev.00000000000000\n" +
		"%global source_sha256 " + strings.Repeat("0", 64) + "\n" +
		"%global develop_asset nimbus-0.6.1.dev.00000000000000-vendor.tar.gz\n"
	pinned, err := developSpec([]byte(recipe), developSource{
		Version: "0.6.1~dev.20260919020436",
		Asset:   "nimbus-0.6.1.dev.20260919020436-vendor.tar.gz",
		Digest:  developDigest,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pinned), "Version:        0.6.1~dev.20260919020436") ||
		!strings.Contains(string(pinned), "%global source_sha256 "+developDigest) ||
		!strings.Contains(string(pinned), "%global develop_asset nimbus-0.6.1.dev.20260919020436-vendor.tar.gz") {
		t.Fatalf("spec not pinned: %s", pinned)
	}
	if _, err := developSpec([]byte("Name: nimbus\n"), developSource{}); err == nil {
		t.Fatal("spec without lines was accepted")
	}
	if _, err := developSpec([]byte(recipe+recipe), developSource{}); err == nil {
		t.Fatal("spec with duplicate lines was accepted")
	}
}
