package packaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// The develop channel is built from the rolling GitHub release the nimbus
// develop workflow republishes on every push. The release metadata only names
// the archive and its digest; the archive itself is downloaded by the shared
// SRPM builder through the spec's Source0 URL.
const developReleaseURL = "https://api.github.com/repos/Furyfree/nimbus/releases/tags/develop"

var (
	developVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+~dev\.[0-9]{14}$`)
	developArchive = regexp.MustCompile(`^nimbus-(.+)-vendor\.tar\.gz$`)
)

// fetchMetadata is the metadata fetch used by the resolver; tests replace it.
var fetchMetadata = httpGet

// developSource is the resolved rolling asset for the develop channel.
// GitHub replaces the tilde in asset names with a dot, so Asset keeps the
// published spelling while Version uses RPM's tilde form.
type developSource struct {
	Version string
	Asset   string
	Digest  string
}

// resolveDevelopSource reads the rolling release metadata and the published
// checksum file. It never downloads the archive.
func resolveDevelopSource(ctx context.Context, client *http.Client, releaseURL string) (developSource, error) {
	type asset struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	}
	var release struct {
		Assets []asset `json:"assets"`
	}
	body, err := fetchMetadata(ctx, client, releaseURL)
	if err != nil {
		return developSource{}, err
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return developSource{}, fmt.Errorf("parse develop release: %w", err)
	}
	archive, checksums := "", ""
	for _, a := range release.Assets {
		if !officialURL(a.URL) || !strings.HasPrefix(a.URL, "https://github.com/Furyfree/nimbus/releases/download/develop/") {
			return developSource{}, fmt.Errorf("develop asset %q is not an official nimbus release URL", a.Name)
		}
		switch {
		case a.Name == "SHA256SUMS":
			checksums = a.URL
		case strings.HasSuffix(a.Name, "-vendor.tar.gz"):
			if archive != "" {
				return developSource{}, errors.New("the develop release carries more than one vendored archive")
			}
			archive = a.Name
		}
	}
	if archive == "" || checksums == "" {
		return developSource{}, errors.New("the develop release lacks a vendored archive or SHA256SUMS")
	}
	match := developArchive.FindStringSubmatch(archive)
	if match == nil {
		return developSource{}, fmt.Errorf("unexpected develop archive name %q", archive)
	}
	version := strings.Replace(match[1], ".dev.", "~dev.", 1)
	if !developVersion.MatchString(version) {
		return developSource{}, fmt.Errorf("unexpected develop version %q", version)
	}
	sums, err := fetchMetadata(ctx, client, checksums)
	if err != nil {
		return developSource{}, err
	}
	digest, err := digestFromSums(sums, archive)
	if err != nil {
		return developSource{}, err
	}
	return developSource{Version: version, Asset: archive, Digest: digest}, nil
}

// digestFromSums returns the lowercase digest recorded for name.
func digestFromSums(sums []byte, name string) (string, error) {
	for line := range strings.SplitSeq(string(sums), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) != 2 {
			continue
		}
		digest := strings.ToLower(strings.TrimPrefix(fields[0], "\\"))
		if l := len(digest); l != 64 {
			continue
		}
		if regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(digest) && fields[1] == name {
			return digest, nil
		}
	}
	return "", fmt.Errorf("SHA256SUMS has no digest for %s", name)
}

// developSpec substitutes the resolved version and digest into the committed
// develop recipe so the SRPM is pinned to one rolling build.
func developSpec(recipe []byte, source developSource) ([]byte, error) {
	text := string(recipe)
	versionRE := regexp.MustCompile(`(?m)^Version:\s+\S+\s*$`)
	assetRE := regexp.MustCompile(`(?m)^%global develop_asset \S+\s*$`)
	digestRE := regexp.MustCompile(`(?m)^%global source_sha256 \S+\s*$`)
	if len(versionRE.FindAllString(text, -1)) != 1 || len(assetRE.FindAllString(text, -1)) != 1 ||
		len(digestRE.FindAllString(text, -1)) != 1 {
		return nil, errors.New("the develop recipe lacks a unique Version, develop_asset or source_sha256 line")
	}
	text = versionRE.ReplaceAllString(text, "Version:        "+source.Version)
	text = assetRE.ReplaceAllString(text, "%global develop_asset "+source.Asset)
	text = digestRE.ReplaceAllString(text, "%global source_sha256 "+source.Digest)
	return []byte(text), nil
}

// prepareNimbusDevelop builds the develop channel's SRPM from the rolling
// release resolved right now.
func (b *Builder) prepareNimbusDevelop(ctx context.Context, out string) (string, error) {
	source, err := resolveDevelopSource(ctx, b.httpClient(), developReleaseURL)
	if err != nil {
		return "", err
	}
	recipe := filepath.Join(b.Root, "packages", "nimbus-develop", "nimbus-develop.spec")
	committed, err := os.ReadFile(recipe)
	if err != nil {
		return "", err
	}
	pinned, err := developSpec(committed, source)
	if err != nil {
		return "", err
	}
	root, err := os.MkdirTemp("", "copr-nimbus-develop-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(root)
	temporary := filepath.Join(root, "nimbus-develop.spec")
	if err := os.WriteFile(temporary, pinned, 0o644); err != nil {
		return "", err
	}
	return b.SRPM(ctx, temporary, out)
}

// officialURL restricts metadata fetches to GitHub over HTTPS; tests replace it.
var officialURL = func(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" &&
		(u.Hostname() == "github.com" || u.Hostname() == "api.github.com") && u.User == nil
}

// httpGet fetches one small metadata document from an official URL.
func httpGet(ctx context.Context, client *http.Client, raw string) ([]byte, error) {
	if !officialURL(raw) {
		return nil, fmt.Errorf("refusing non-official URL %s", raw)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Furyfree-copr")
	response, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", raw, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: unexpected status %s", raw, response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", raw, err)
	}
	return body, nil
}
