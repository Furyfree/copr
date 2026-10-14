// Package claude delivers Anthropic's official Linux package with explicit ownership.
package claude

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

	"github.com/Furyfree/copr/internal/pins"
)

const Version = "0.1.1"
const aptBase = "https://downloads.claude.ai/claude-desktop/apt/stable/"
const indexPath = "main/binary-amd64/Packages"
const indexURL = aptBase + "dists/stable/" + indexPath
const releaseURL = aptBase + "dists/stable/InRelease"
const maxArtifact = 1024 * 1024 * 1024
const maxMetadata = 2 * 1024 * 1024

var stableVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Artifact pins.Artifact

func sourceURL(version string) string {
	return aptBase + "pool/main/c/claude-desktop/claude-desktop_" + version + "_amd64.deb"
}
func officialURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "downloads.claude.ai" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	if raw == indexURL || raw == releaseURL {
		return true
	}
	version := strings.TrimSuffix(strings.TrimPrefix(raw, aptBase+"pool/main/c/claude-desktop/claude-desktop_"), "_amd64.deb")
	return stableVersion.MatchString(version) && raw == sourceURL(version)
}
func httpClient() *http.Client {
	return &http.Client{Timeout: 120 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 10 || !officialURL(r.URL.String()) {
			return errors.New("download redirected outside Anthropic's package repository")
		}
		return nil
	}}
}
func (e *Installer) fetch(ctx context.Context, raw, destination string, size int64) ([]byte, error) {
	if !officialURL(raw) {
		return nil, errors.New("download is outside Anthropic's package repository")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "claude-desktop-installer")
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Anthropic returned HTTP %d", resp.StatusCode)
	}
	if destination == "" {
		data, err := io.ReadAll(io.LimitReader(resp.Body, maxMetadata+1))
		if len(data) > maxMetadata {
			return nil, errors.New("release metadata is too large")
		}
		return data, err
	}
	if size < 128 || size > maxArtifact {
		return nil, errors.New("invalid Debian package size")
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
		return nil, errors.New("package is shorter than the signed release size")
	}
	var extra [1]byte
	nExtra, readErr := io.ReadFull(resp.Body, extra[:])
	if nExtra != 0 {
		return nil, errors.New("package exceeds the signed release size")
	}
	if !errors.Is(readErr, io.EOF) {
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
	if !info.Mode().IsRegular() || info.Size() < 128 || info.Size() > maxArtifact {
		return errors.New("Debian package must be a bounded regular file, not a symlink")
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
		return errors.New("Debian package does not match approved SHA-256")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var header [8]byte
	if _, err := io.ReadFull(f, header[:]); err != nil {
		return err
	}
	if string(header[:]) != "!<arch>\n" {
		return errors.New("artifact is not a Debian ar archive")
	}
	return nil
}
func (e *Installer) Prepare(ctx context.Context, directory, version string) (Artifact, error) {
	a, size, err := e.resolve(ctx, version)
	if err != nil {
		return Artifact{}, err
	}
	return e.prepare(ctx, directory, a, size)
}
func (e *Installer) prepare(ctx context.Context, directory string, a Artifact, size int64) (Artifact, error) {
	directory, err := filepath.Abs(directory)
	if err != nil {
		return Artifact{}, err
	}
	directory, err = filepath.EvalSymlinks(directory)
	if err != nil {
		return Artifact{}, err
	}
	stage, err := os.MkdirTemp(directory, "claude-desktop-")
	if err != nil {
		return Artifact{}, err
	}
	keep := false
	defer func() {
		if !keep {
			os.RemoveAll(stage)
		}
	}()
	artifact := filepath.Join(stage, "claude-desktop_"+a.Version+"_amd64.deb")
	if _, err := e.fetch(ctx, a.Source, artifact, size); err != nil {
		return Artifact{}, err
	}
	if err := checkArtifact(artifact, a.SHA256); err != nil {
		return Artifact{}, err
	}
	keep = true
	a.Path = artifact
	return a, nil
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
