//go:build integration

package wowup

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNativeSquashFSInstallation(t *testing.T) {
	f := setup(t)
	source := filepath.Join(f.root, "squashfs-source")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(source, "AppRun"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("AppRun", filepath.Join(source, "internal-link")); err != nil {
		t.Fatal(err)
	}
	squashfs := filepath.Join(f.root, "filesystem.squashfs")
	cmd := exec.CommandContext(t.Context(), "/usr/bin/mksquashfs", source, squashfs, "-noappend", "-no-progress", "-processors", "1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create fixture: %s %v", output, err)
	}
	filesystem, err := os.ReadFile(squashfs)
	if err != nil {
		t.Fatal(err)
	}
	image := append(artifactBytes()[:128], filesystem...)
	if err := os.WriteFile(f.artifact, image, 0o644); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(image)
	f.digest = hex.EncodeToString(hash[:])
	f.e.extract = nativeExtract
	if !f.install(t, "2.23.1").Verified {
		t.Fatal("native installation not verified")
	}
	if status, err := f.e.Uninstall(t.Context()); err != nil || status.Installed {
		t.Fatalf("native removal: %+v %v", status, err)
	}
}
