package packaging

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

const developDigest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func developFixture(t *testing.T, release, sums string) func(context.Context, *http.Client, string) ([]byte, error) {
	t.Helper()
	previous := fetchMetadata
	fetchMetadata = func(_ context.Context, _ *http.Client, raw string) ([]byte, error) {
		switch {
		case strings.HasSuffix(raw, "/releases/tags/develop"):
			return []byte(release), nil
		case strings.HasSuffix(raw, "/SHA256SUMS"):
			return []byte(sums), nil
		default:
			t.Fatalf("unexpected metadata fetch %s", raw)
			return nil, errors.New("unexpected")
		}
	}
	t.Cleanup(func() { fetchMetadata = previous })
	return fetchMetadata
}

func TestResolveDevelopSource(t *testing.T) {
	const archive = "nimbus-0.6.0~dev.20260918git0123456789ab-vendor.tar.gz"
	release := `{"assets":[
		{"name":"` + archive + `","browser_download_url":"https://github.com/Furyfree/nimbus/releases/download/develop/` + archive + `"},
		{"name":"SHA256SUMS","browser_download_url":"https://github.com/Furyfree/nimbus/releases/download/develop/SHA256SUMS"}]}`
	sums := developDigest + "  " + archive + "\n"
	developFixture(t, release, sums)
	source, err := resolveDevelopSource(t.Context(), &http.Client{}, developReleaseURL)
	if err != nil || source.Version != "0.6.0~dev.20260918git0123456789ab" ||
		source.Asset != archive || source.Digest != developDigest {
		t.Fatalf("source = %+v, %v", source, err)
	}
}

func TestResolveDevelopSourceRefusals(t *testing.T) {
	const archive = "nimbus-0.6.0~dev.20260918git0123456789ab-vendor.tar.gz"
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
		{"bad version", strings.ReplaceAll(base, "0.6.0~dev.20260918git0123456789ab", "0.6.0"), developDigest + "  " + strings.ReplaceAll(archive, "0.6.0~dev.20260918git0123456789ab", "0.6.0"), "unexpected develop version"},
		{"digest missing", base, "deadbeef  other.tar.gz\n", "no digest"},
		{"short digest", base, "abc  " + archive + "\n", "no digest"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			developFixture(t, tc.release, tc.sums)
			_, err := resolveDevelopSource(t.Context(), &http.Client{}, developReleaseURL)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestDevelopSpec(t *testing.T) {
	recipe := "Name: nimbus\nVersion:        0.6.0~dev.00000000git000000000000\n" +
		"%global source_sha256 " + strings.Repeat("0", 64) + "\n" +
		"%global develop_asset nimbus-0.6.0.dev.00000000git000000000000-vendor.tar.gz\n"
	pinned, err := developSpec([]byte(recipe), developSource{
		Version: "0.6.0~dev.20260918git0123456789ab",
		Asset:   "nimbus-0.6.0.dev.20260918git0123456789ab-vendor.tar.gz",
		Digest:  developDigest,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pinned), "Version:        0.6.0~dev.20260918git0123456789ab") ||
		!strings.Contains(string(pinned), "%global source_sha256 "+developDigest) ||
		!strings.Contains(string(pinned), "%global develop_asset nimbus-0.6.0.dev.20260918git0123456789ab-vendor.tar.gz") {
		t.Fatalf("spec not pinned: %s", pinned)
	}
	if _, err := developSpec([]byte("Name: nimbus\n"), developSource{}); err == nil {
		t.Fatal("spec without lines was accepted")
	}
	if _, err := developSpec([]byte(recipe+recipe), developSource{}); err == nil {
		t.Fatal("spec with duplicate lines was accepted")
	}
}
