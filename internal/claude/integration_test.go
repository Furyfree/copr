//go:build integration

package claude

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func native(t *testing.T, directory, command string, args ...string) []byte {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), command, args...)
	cmd.Dir = directory
	data, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %s %v", command, data, err)
	}
	return data
}
func fixtureDeb(t *testing.T, version, arch string, payload []byte) string {
	t.Helper()
	directory := t.TempDir()
	control := []byte("Package: claude-desktop\nVersion: " + version + "\nArchitecture: " + arch + "\n")
	controlTar := tarBytes(t, []tarEntry{
		{header: tar.Header{Name: "./control", Typeflag: tar.TypeReg, Mode: 0o644}, data: control},
		{header: tar.Header{Name: "./postinst", Typeflag: tar.TypeReg, Mode: 0o755}, data: []byte("#!/bin/sh\ntouch " + filepath.Join(directory, "script-ran") + "\n")},
	})
	for name, data := range map[string][]byte{"control.tar.xz": controlTar, "data.tar.xz": payload} {
		cmd := exec.CommandContext(t.Context(), "/usr/bin/xz", "--compress", "--stdout")
		cmd.Stdin = bytes.NewReader(data)
		compressed, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, name), compressed, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(directory, "debian-binary"), []byte("2.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "fixture.deb")
	native(t, directory, "/usr/bin/ar", "qc", path, "debian-binary", "control.tar.xz", "data.tar.xz")
	return path
}

func TestNativeDebianPackageLifecycle(t *testing.T) {
	f := setup(t)
	f.e.extract = nativeExtract
	path := fixtureDeb(t, "2.10.0", "amd64", tarBytes(t, applicationEntries()))
	digest, err := fileHash(path)
	if err != nil {
		t.Fatal(err)
	}
	s, err := f.e.Apply(t.Context(), path, digest, "2.10.0")
	if err != nil || !s.Verified || !s.Integrated {
		t.Fatalf("native install: %+v %v", s, err)
	}
	if exists(filepath.Join(filepath.Dir(path), "script-ran")) {
		t.Fatal("Debian maintainer script was executed")
	}
	native(t, "", "desktop-file-validate", filepath.Join(f.e.app, "claude.desktop"))
	if s, err := f.e.Uninstall(t.Context()); err != nil || s.Installed {
		t.Fatalf("native removal: %+v %v", s, err)
	}
}

func TestNativeExtractionRejectsWrongIdentityAndDamagedXZ(t *testing.T) {
	for _, failure := range []string{"version", "architecture", "xz-checksum"} {
		t.Run(failure, func(t *testing.T) {
			f := setup(t)
			f.install(t, "2.9.0")
			f.e.extract = nativeExtract
			version, arch := "2.10.0", "amd64"
			if failure == "version" {
				version = "2.11.0"
			}
			if failure == "architecture" {
				arch = "arm64"
			}
			path := fixtureDeb(t, version, arch, tarBytes(t, applicationEntries()))
			if failure == "xz-checksum" {
				member := filepath.Join(filepath.Dir(path), "data.tar.xz")
				data, err := os.ReadFile(member)
				if err != nil {
					t.Fatal(err)
				}
				data[len(data)-8] ^= 1
				if err := os.WriteFile(member, data, 0o644); err != nil {
					t.Fatal(err)
				}
				native(t, filepath.Dir(path), "/usr/bin/ar", "r", path, "data.tar.xz")
			}
			digest, err := fileHash(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.e.Apply(t.Context(), path, digest, "2.10.0"); err == nil {
				t.Fatal("invalid native extraction accepted")
			}
			if s, err := f.e.Status(); err != nil || s.Version != "2.9.0" || !s.Verified || !s.Integrated {
				t.Fatalf("failed extraction changed installation: %+v %v", s, err)
			}
		})
	}
}

func TestNativeSignatureAndIndexChain(t *testing.T) {
	dir := t.TempDir()
	args := []string{"--no-options", "--homedir", dir, "--batch", "--pinentry-mode", "loopback", "--passphrase", ""}
	native(t, "", "/usr/bin/gpg", append(args, "--quick-generate-key", "Packaging fixture <fixture@example.invalid>", "ed25519", "sign", "0")...)
	t.Cleanup(func() {
		cmd := exec.CommandContext(context.WithoutCancel(t.Context()), "/usr/bin/gpgconf", "--homedir", dir, "--kill", "all")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("stop fixture GnuPG agent: %s %v", output, err)
		}
	})
	key := native(t, "", "/usr/bin/gpg", append(args, "--armor", "--export", "fixture@example.invalid")...)
	f := setup(t)
	index := fixtureIndex("2.10.0", f.digest, 256)
	release := fixtureRelease(index)
	if err := os.WriteFile(filepath.Join(dir, "Release"), release, 0o600); err != nil {
		t.Fatal(err)
	}
	native(t, "", "/usr/bin/gpg", append(args, "--armor", "--output", filepath.Join(dir, "InRelease"), "--clearsign", filepath.Join(dir, "Release"))...)
	signed, err := os.ReadFile(filepath.Join(dir, "InRelease"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := verifyRelease(t.Context(), signed, key)
	if err != nil || !bytes.Equal(data, release) {
		t.Fatalf("native signature verification: %s %v", data, err)
	}
	if _, err := verifyRelease(t.Context(), signed, signingKey); err == nil {
		t.Fatal("a fixture signature was trusted as Anthropic")
	}
	changed := bytes.Replace(signed, []byte("Origin: Anthropic"), []byte("Origin: Impostor"), 1)
	if _, err := verifyRelease(t.Context(), changed, key); err == nil {
		t.Fatal("modified signed release was accepted")
	}
	hash := sha256.Sum256(index)
	if !strings.Contains(string(data), fmt.Sprintf("%x %d main/binary-amd64/Packages", hash, len(index))) {
		t.Fatal("signed index hash was not retained")
	}
}
