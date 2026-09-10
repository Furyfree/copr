// Package wowup delivers the official CurseForge AppImage with explicit ownership.
package wowup

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
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
	"time"
)

const Version = "0.1.0"
const maxArtifact = 1024 * 1024 * 1024

var stableVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Artifact struct {
	SchemaVersion int    `json:"schema_version"`
	Version       string `json:"version"`
	Name          string `json:"name"`
	Arch          string `json:"arch"`
	SHA256        string `json:"sha256"`
	Path          string `json:"path"`
	Source        string `json:"source"`
}

func sourceURL(version string) string {
	return "https://github.com/WowUp/WowUp.CF/releases/download/v" + version + "/WowUp-CF-" + version + ".AppImage"
}
func officialURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return false
	}
	return u.Host == "api.github.com" && strings.HasPrefix(u.Path, "/repos/WowUp/WowUp.CF/releases/") || u.Host == "github.com" && strings.HasPrefix(u.Path, "/WowUp/WowUp.CF/releases/download/") || u.Host == "release-assets.githubusercontent.com"
}
func httpClient() *http.Client {
	return &http.Client{Timeout: 120 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 10 || !officialURL(r.URL.String()) {
			return errors.New("download redirected outside official GitHub source")
		}
		return nil
	}}
}
func (e *Installer) fetch(ctx context.Context, raw string, destination string, size int64) ([]byte, error) {
	if !officialURL(raw) {
		return nil, errors.New("download is outside official GitHub source")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "wowup-cf-installer")
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub returned HTTP %d", resp.StatusCode)
	}
	if destination == "" {
		data, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024+1))
		if len(data) > 2*1024*1024 {
			return nil, errors.New("release metadata is too large")
		}
		return data, err
	}
	if size < 64 || size > maxArtifact {
		return nil, errors.New("invalid AppImage download size")
	}
	f, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	n, copyErr := io.Copy(f, io.LimitReader(resp.Body, size))
	closeErr := f.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return nil, err
	}
	if n != size {
		return nil, errors.New("AppImage is shorter than release size")
	}
	var extra [1]byte
	nExtra, readErr := io.ReadFull(resp.Body, extra[:])
	if nExtra != 0 {
		return nil, errors.New("AppImage exceeds release size")
	}
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return nil, readErr
	}
	return nil, nil
}
func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func regular(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("AppImage must be a regular file, not a symlink")
	}
	return nil
}
func checkArtifact(path, digest string) error {
	if err := regular(path); err != nil {
		return err
	}
	actual, err := fileHash(path)
	if err != nil {
		return err
	}
	if !digestPattern.MatchString(digest) || actual != digest {
		return errors.New("AppImage does not match approved SHA-256")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var header [64]byte
	if _, err := io.ReadFull(f, header[:]); err != nil {
		return err
	}
	if string(header[:11]) != "\x7fELF\x02\x01\x01\x00AI\x02" || binary.LittleEndian.Uint16(header[18:]) != 62 {
		return errors.New("artifact is not an x86_64 type-2 AppImage")
	}
	return nil
}

func (e *Installer) Prepare(ctx context.Context, directory, version string) (Artifact, error) {
	var result Artifact
	if version != "" && !stableVersion.MatchString(version) {
		return result, errors.New("invalid stable application version")
	}
	endpoint := "latest"
	if version != "" {
		endpoint = "tags/v" + version
	}
	data, err := e.fetch(ctx, "https://api.github.com/repos/WowUp/WowUp.CF/releases/"+endpoint, "", 0)
	if err != nil {
		return result, err
	}
	var release struct {
		Tag        string `json:"tag_name"`
		Draft      *bool  `json:"draft"`
		Prerelease *bool  `json:"prerelease"`
		Assets     []struct {
			Name   string `json:"name"`
			URL    string `json:"browser_download_url"`
			Digest string `json:"digest"`
			Size   int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(data, &release); err != nil {
		return result, err
	}
	selected := strings.TrimPrefix(release.Tag, "v")
	if release.Draft == nil || *release.Draft || release.Prerelease == nil || *release.Prerelease || release.Tag != "v"+selected || !stableVersion.MatchString(selected) || version != "" && selected != version {
		return result, errors.New("metadata must describe selected published stable release")
	}
	count := 0
	var digest string
	var size int64
	for _, asset := range release.Assets {
		if asset.Name == "WowUp-CF-"+selected+".AppImage" {
			count++
			digest = strings.TrimPrefix(asset.Digest, "sha256:")
			size = asset.Size
			if asset.URL != sourceURL(selected) || asset.Digest != "sha256:"+digest || !digestPattern.MatchString(digest) {
				return result, errors.New("release asset has invalid source or SHA-256")
			}
		}
	}
	if count != 1 || size < 64 || size > maxArtifact {
		return result, errors.New("release requires exactly one valid-sized official CF AppImage")
	}
	directory, err = filepath.Abs(directory)
	if err != nil {
		return result, err
	}
	directory, err = filepath.EvalSymlinks(directory)
	if err != nil {
		return result, err
	}
	stage, err := os.MkdirTemp(directory, "wowup-cf-")
	if err != nil {
		return result, err
	}
	keep := false
	defer func() {
		if !keep {
			os.RemoveAll(stage)
		}
	}()
	artifact := filepath.Join(stage, "WowUp-CF-"+selected+".AppImage")
	if _, err := e.fetch(ctx, sourceURL(selected), artifact, size); err != nil {
		return result, err
	}
	if err := checkArtifact(artifact, digest); err != nil {
		return result, err
	}
	keep = true
	return Artifact{1, selected, "wowup-cf", "x86_64", digest, artifact, sourceURL(selected)}, nil
}

func older(candidate, installed string) bool {
	a, b := strings.Split(candidate, "."), strings.Split(installed, ".")
	for i := range a {
		if len(a[i]) != len(b[i]) {
			return len(a[i]) < len(b[i])
		}
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}
