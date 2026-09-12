//go:build integration

package packaging

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestBleshChecksumsBeforeExtraction(t *testing.T) {
	selected(t, "blesh")
	recipe, err := os.ReadFile(filepath.Join(repository(t), "packages/blesh/blesh.spec"))
	if err != nil {
		t.Fatal(err)
	}
	for _, changed := range []string{"", "source", "contrib"} {
		t.Run("changed="+changed, func(t *testing.T) {
			root := t.TempDir()
			spec := string(recipe)
			for _, source := range []struct{ name, macro, prefix string }{
				{"source", "commit", "ble.sh-"},
				{"contrib", "contrib_commit", "blesh-contrib-"},
			} {
				commit := regexp.MustCompile(`(?m)^%global ` + source.macro + ` ([a-f0-9]+)$`).FindStringSubmatch(spec)
				if len(commit) != 2 {
					t.Fatal("missing source commit")
				}
				dir := filepath.Join(root, source.name)
				write(t, filepath.Join(dir, "extraction-marker"), "approved\n")
				// The core archive reserves an empty directory for its submodule.
				if err := os.MkdirAll(filepath.Join(dir, "contrib"), 0o755); err != nil {
					t.Fatal(err)
				}
				archive := filepath.Join(root, commit[1]+".tar.gz")
				if err := bundle(dir, archive, source.prefix+commit[1]); err != nil {
					t.Fatal(err)
				}
				digest, err := fileDigest(archive)
				if err != nil {
					t.Fatal(err)
				}
				pattern := regexp.MustCompile(`(?m)^%global ` + source.name + `_sha256 [a-f0-9]+$`)
				spec = pattern.ReplaceAllString(spec, "%global "+source.name+"_sha256 "+digest)
				if changed == source.name {
					// Keep a valid tarball but change its bytes after approving the digest.
					write(t, filepath.Join(dir, "extraction-marker"), "changed\n")
					if err := os.Remove(archive); err != nil {
						t.Fatal(err)
					}
					if err := bundle(dir, archive, source.prefix+commit[1]); err != nil {
						t.Fatal(err)
					}
				}
			}
			path := filepath.Join(root, "blesh.spec")
			write(t, path, spec)
			build := filepath.Join(root, "build")
			out, err := Run(t.Context(), "rpmbuild", []string{"-bp", "--nodeps", path, "--define", "_topdir " + build, "--define", "_sourcedir " + root}, "", CleanEnvironment())
			markers := 0
			if walkErr := filepath.WalkDir(build, func(path string, d os.DirEntry, err error) error {
				if err == nil && d.Name() == "extraction-marker" {
					markers++
				}
				return err
			}); walkErr != nil {
				t.Fatal(walkErr)
			}
			if changed == "" {
				if err != nil || markers != 2 {
					t.Fatalf("approved archives: %v, extracted=%d\n%s", err, markers, out)
				}
			} else if err == nil || markers != 0 || !strings.Contains(err.Error(), "checksum did NOT match") {
				t.Fatalf("changed %s accepted or extracted: %v, extracted=%d\n%s", changed, err, markers, out)
			}
		})
	}
}
