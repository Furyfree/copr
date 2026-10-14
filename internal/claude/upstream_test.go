//go:build upstream

package claude

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Inspect reviewed downloads offline; never launch or install on the host.
func TestOfficialDebianPackage(t *testing.T) {
	path := os.Getenv("CLAUDE_TEST_DEB")
	if path == "" {
		t.Fatal("CLAUDE_TEST_DEB must name the package-selected official Debian package")
	}
	a, err := PackagedRelease()
	if err != nil {
		t.Fatal(err)
	}
	f := setup(t)
	f.e.extract = nativeExtract
	s, err := f.e.Apply(t.Context(), path, a.SHA256, a.Version)
	if err != nil || !s.Verified || !s.Integrated {
		t.Fatalf("official package: %+v %v", s, err)
	}
	if err := validateApplication(filepath.Join(f.e.app, "app")); err != nil {
		t.Fatal(err)
	}
	if s, err := f.e.Uninstall(t.Context()); err != nil || s.Installed {
		t.Fatalf("official package removal: %+v %v", s, err)
	}
}

func TestOfficialRepositorySignature(t *testing.T) {
	directory := os.Getenv("CLAUDE_TEST_REPOSITORY")
	if directory == "" {
		t.Fatal("CLAUDE_TEST_REPOSITORY must contain reviewed InRelease and Packages downloads")
	}
	signed, err := os.ReadFile(filepath.Join(directory, "InRelease"))
	if err != nil {
		t.Fatal(err)
	}
	release, err := verifyRelease(t.Context(), signed, signingKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := releaseIndex(release, time.Now()); err != nil {
		t.Fatal(err)
	}
	index, err := os.ReadFile(filepath.Join(directory, "Packages"))
	if err != nil {
		t.Fatal(err)
	}
	f := setup(t)
	serveRelease(f, "", index, nil, release)
	if _, _, err := f.e.resolve(t.Context(), ""); err != nil {
		t.Fatal(err)
	}
}
