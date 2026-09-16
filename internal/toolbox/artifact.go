// Package toolbox delivers the official JetBrains Toolbox App with explicit ownership.
package toolbox

import (
	"context"
	"crypto/sha256"
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
const archiveName = "jetbrains-toolbox.tar.gz"

// JetBrains identifies Toolbox releases by a four-component build number.
var buildPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.(0|[1-9][0-9]*)){3}$`)
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

func sourceURL(build string) string {
	return "https://download.jetbrains.com/toolbox/jetbrains-toolbox-" + build + ".tar.gz"
}
func releasesURL(build string) string {
	query := "code=TBA&type=release&latest=true"
	if build != "" {
		query = "code=TBA&type=release&build=" + build
	}
	return "https://data.services.jetbrains.com/products/releases?" + query
}
func officialURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return false
	}
	return u.Host == "data.services.jetbrains.com" && u.Path == "/products/releases" || (u.Host == "download.jetbrains.com" || u.Host == "download-cdn.jetbrains.com") && strings.HasPrefix(u.Path, "/toolbox/")
}
func httpClient() *http.Client {
	return &http.Client{Timeout: 120 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 10 || !officialURL(r.URL.String()) {
			return errors.New("download redirected outside official JetBrains source")
		}
		return nil
	}}
}
func (e *Installer) fetch(ctx context.Context, raw string, destination string, size int64) ([]byte, error) {
	if !officialURL(raw) {
		return nil, errors.New("download is outside official JetBrains source")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "jetbrains-toolbox-installer")
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("JetBrains returned HTTP %d", resp.StatusCode)
	}
	if destination == "" {
		data, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024+1))
		if len(data) > 2*1024*1024 {
			return nil, errors.New("release metadata is too large")
		}
		return data, err
	}
	if size < 64 || size > maxArtifact {
		return nil, errors.New("invalid archive download size")
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
		return nil, errors.New("archive is shorter than release size")
	}
	var extra [1]byte
	nExtra, readErr := io.ReadFull(resp.Body, extra[:])
	if nExtra != 0 {
		return nil, errors.New("archive exceeds release size")
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
		return errors.New("archive must be a regular file, not a symlink")
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
		return errors.New("archive does not match approved SHA-256")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var header [3]byte
	if _, err := io.ReadFull(f, header[:]); err != nil {
		return err
	}
	if header != [3]byte{0x1f, 0x8b, 0x08} {
		return errors.New("artifact is not a gzip-compressed archive")
	}
	return nil
}

// parseChecksum reads the published "<sha256> *<file>" line for the archive.
func parseChecksum(data []byte, build string) (string, error) {
	fields := strings.Fields(string(data))
	if len(fields) != 2 {
		return "", errors.New("checksum file must contain one digest and file name")
	}
	digest := strings.ToLower(fields[0])
	if !digestPattern.MatchString(digest) || strings.TrimPrefix(fields[1], "*") != "jetbrains-toolbox-"+build+".tar.gz" {
		return "", errors.New("checksum file does not describe the selected archive")
	}
	return digest, nil
}

func (e *Installer) resolve(ctx context.Context, build string) (Artifact, int64, error) {
	var result Artifact
	if build != "" && !buildPattern.MatchString(build) {
		return result, 0, errors.New("invalid release build number")
	}
	data, err := e.fetch(ctx, releasesURL(build), "", 0)
	if err != nil {
		return result, 0, err
	}
	var releases struct {
		TBA []struct {
			Build     string `json:"build"`
			Type      string `json:"type"`
			Downloads struct {
				Linux struct {
					Link         string `json:"link"`
					Size         int64  `json:"size"`
					ChecksumLink string `json:"checksumLink"`
				} `json:"linux"`
			} `json:"downloads"`
		} `json:"TBA"`
	}
	if err := json.Unmarshal(data, &releases); err != nil {
		return result, 0, err
	}
	if len(releases.TBA) != 1 {
		return result, 0, errors.New("metadata must describe exactly one selected release")
	}
	release := releases.TBA[0]
	selected := release.Build
	if release.Type != "release" || !buildPattern.MatchString(selected) || build != "" && selected != build {
		return result, 0, errors.New("metadata must describe selected published stable release")
	}
	linux := release.Downloads.Linux
	if linux.Link != sourceURL(selected) || linux.ChecksumLink != sourceURL(selected)+".sha256" {
		return result, 0, errors.New("release download has invalid source or checksum location")
	}
	if linux.Size < 64 || linux.Size > maxArtifact {
		return result, 0, errors.New("release requires a valid-sized official Linux archive")
	}
	checksum, err := e.fetch(ctx, linux.ChecksumLink, "", 0)
	if err != nil {
		return result, 0, err
	}
	digest, err := parseChecksum(checksum, selected)
	if err != nil {
		return result, 0, err
	}
	return Artifact{1, selected, "jetbrains-toolbox", "x86_64", digest, "", sourceURL(selected)}, linux.Size, nil
}

func (e *Installer) Prepare(ctx context.Context, directory, build string) (Artifact, error) {
	a, size, err := e.resolve(ctx, build)
	if err != nil {
		return Artifact{}, err
	}
	return e.prepare(ctx, directory, a, size)
}

func (e *Installer) prepare(ctx context.Context, directory string, a Artifact, size int64) (Artifact, error) {
	var result Artifact
	var err error
	directory, err = filepath.Abs(directory)
	if err != nil {
		return result, err
	}
	directory, err = filepath.EvalSymlinks(directory)
	if err != nil {
		return result, err
	}
	stage, err := os.MkdirTemp(directory, "jetbrains-toolbox-")
	if err != nil {
		return result, err
	}
	keep := false
	defer func() {
		if !keep {
			os.RemoveAll(stage)
		}
	}()
	artifact := filepath.Join(stage, "jetbrains-toolbox-"+a.Version+".tar.gz")
	if _, err := e.fetch(ctx, a.Source, artifact, size); err != nil {
		return result, err
	}
	if err := checkArtifact(artifact, a.SHA256); err != nil {
		return result, err
	}
	keep = true
	a.Path = artifact
	return a, nil
}

// older compares two validated four-component build numbers.
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
