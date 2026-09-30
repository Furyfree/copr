//go:build upstream

package copilot

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Run with a separately downloaded AppImage; never installs on the host.
func TestOfficialAppImage(t *testing.T) {
	artifact := os.Getenv("COPILOT_TEST_APPIMAGE")
	if artifact == "" {
		t.Fatal("COPILOT_TEST_APPIMAGE must name the package-selected official AppImage")
	}
	a, err := PackagedRelease()
	if err != nil {
		t.Fatal(err)
	}
	f := setup(t)
	f.e.extract = nativeExtract
	s, err := f.e.Apply(t.Context(), artifact, a.SHA256, a.Version)
	if err != nil || !s.Verified || !s.Integrated {
		t.Fatalf("official AppImage installation: %+v %v", s, err)
	}
	for _, path := range []string{"app/AppRun", "app/usr/bin/github", "app/usr/share/icons/hicolor/128x128/apps/github.png"} {
		if _, err := os.Stat(filepath.Join(f.e.app, path)); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.CommandContext(t.Context(), "desktop-file-validate", filepath.Join(f.e.app, "github-copilot.desktop"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("desktop integration: %s %v", output, err)
	}
	if s, err := f.e.Uninstall(t.Context()); err != nil || s.Installed {
		t.Fatalf("official AppImage removal: %+v %v", s, err)
	}
}
