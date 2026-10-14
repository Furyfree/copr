package claude

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Anthropic's published fingerprint: 31DDDE24DDFAB679F42D7BD2BAA929FF1A7ECACE.
//
//go:embed key.asc
var signingKey []byte

func verifyRelease(ctx context.Context, signed, key []byte) ([]byte, error) {
	if len(signed) > maxMetadata || len(key) > maxMetadata {
		return nil, errors.New("signed metadata or key exceeds maximum size")
	}
	dir, err := os.MkdirTemp("", "claude-signature-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	for name, data := range map[string][]byte{"key.asc": key, "InRelease": signed} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			return nil, err
		}
	}
	commands := [][]string{
		{"/usr/bin/gpg", "--no-options", "--homedir", dir, "--batch", "--dearmor", "--output", filepath.Join(dir, "key.gpg"), filepath.Join(dir, "key.asc")},
		{"/usr/bin/gpgv", "--homedir", dir, "--keyring", filepath.Join(dir, "key.gpg"), "--output", filepath.Join(dir, "Release"), filepath.Join(dir, "InRelease")},
	}
	for _, args := range commands {
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("verify Anthropic repository signature: %w: %s", err, output)
		}
	}
	return os.ReadFile(filepath.Join(dir, "Release"))
}

// fields reads one Debian control paragraph, including continued hash lists.
func fields(data string) (map[string]string, error) {
	result := map[string]string{}
	previous := ""
	for line := range strings.SplitSeq(strings.ReplaceAll(data, "\r\n", "\n"), "\n") {
		if line == "" {
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			if previous == "" {
				return nil, errors.New("metadata continuation has no field")
			}
			result[previous] += "\n" + strings.TrimSpace(line)
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok || key == "" || strings.ContainsAny(key, " \t\r") {
			return nil, errors.New("invalid metadata field")
		}
		if _, exists := result[key]; exists {
			return nil, fmt.Errorf("duplicate metadata field: %s", key)
		}
		result[key] = strings.TrimSpace(value)
		previous = key
	}
	return result, nil
}

func releaseIndex(data []byte, now time.Time) (string, int64, error) {
	f, err := fields(string(data))
	if err != nil {
		return "", 0, err
	}
	if f["Origin"] != "Anthropic" || f["Suite"] != "stable" || f["Codename"] != "stable" || !slices.Contains(strings.Fields(f["Architectures"]), "amd64") || !slices.Contains(strings.Fields(f["Components"]), "main") {
		return "", 0, errors.New("signed metadata is not Anthropic's stable amd64 repository")
	}
	date, err := time.Parse(time.RFC1123Z, f["Date"])
	if err != nil || date.After(now.Add(5*time.Minute)) {
		return "", 0, errors.New("signed repository date is invalid or in the future")
	}
	expires, err := time.Parse(time.RFC1123, f["Valid-Until"])
	if err != nil || !expires.After(now) || !expires.After(date) {
		return "", 0, errors.New("signed repository metadata has expired or has invalid validity")
	}
	var digest string
	var size int64
	for line := range strings.SplitSeq(f["SHA256"], "\n") {
		parts := strings.Fields(line)
		if len(parts) != 3 || parts[2] != indexPath {
			continue
		}
		if digest != "" {
			return "", 0, errors.New("signed repository repeats the amd64 index")
		}
		digest = parts[0]
		size, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil || !digestPattern.MatchString(digest) || size <= 0 || size > maxMetadata {
			return "", 0, errors.New("signed repository has an invalid amd64 index hash or size")
		}
	}
	if digest == "" {
		return "", 0, errors.New("signed repository has no amd64 SHA-256 index")
	}
	return digest, size, nil
}

func parsePackages(data []byte, version string) (Artifact, int64, error) {
	var selected Artifact
	var selectedSize int64
	seen := map[string]bool{}
	for paragraph := range strings.SplitSeq(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n\n") {
		if strings.TrimSpace(paragraph) == "" {
			continue
		}
		f, err := fields(paragraph)
		if err != nil {
			return Artifact{}, 0, err
		}
		if f["Package"] != "claude-desktop" || f["Architecture"] != "amd64" {
			continue
		}
		v := f["Version"]
		if !stableVersion.MatchString(v) {
			return Artifact{}, 0, errors.New("unsupported official package version")
		}
		if seen[v] {
			return Artifact{}, 0, errors.New("package index repeats a Claude version")
		}
		seen[v] = true
		size, err := strconv.ParseInt(f["Size"], 10, 64)
		if err != nil || size < 128 || size > maxArtifact || !digestPattern.MatchString(f["SHA256"]) || aptBase+f["Filename"] != sourceURL(v) {
			return Artifact{}, 0, errors.New("package index has an invalid Claude source, size or digest")
		}
		if version != "" && version != v || selected.Version != "" && !older(selected.Version, v) {
			continue
		}
		selected = Artifact{1, v, "claude-desktop", "x86_64", f["SHA256"], "", sourceURL(v)}
		selectedSize = size
	}
	if selected.Version == "" {
		return Artifact{}, 0, errors.New("selected Claude release is absent from the signed package index")
	}
	return selected, selectedSize, nil
}

func (e *Installer) resolve(ctx context.Context, version string) (Artifact, int64, error) {
	if version != "" && !stableVersion.MatchString(version) {
		return Artifact{}, 0, errors.New("invalid stable application version")
	}
	signed, err := e.fetch(ctx, releaseURL, "", 0)
	if err != nil {
		return Artifact{}, 0, err
	}
	release, err := e.verify(ctx, signed)
	if err != nil {
		return Artifact{}, 0, err
	}
	digest, size, err := releaseIndex(release, time.Now())
	if err != nil {
		return Artifact{}, 0, err
	}
	data, err := e.fetch(ctx, indexURL, "", 0)
	if err != nil {
		return Artifact{}, 0, err
	}
	hash := sha256.Sum256(data)
	if int64(len(data)) != size || hex.EncodeToString(hash[:]) != digest {
		return Artifact{}, 0, errors.New("package index differs from Anthropic's signed SHA-256 or size")
	}
	return parsePackages(data, version)
}
