package toolbox

import (
	"archive/tar"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractionRejectsUnsafeArchives(t *testing.T) {
	top := "jetbrains-toolbox-" + current + "/"
	official := officialMembers()
	cases := map[string][]member{
		"absolute":            append(official, member{name: "/etc/passwd", body: "x", mode: 0o644}),
		"traversal":           append(official, member{name: top + "../escape", body: "x", mode: 0o644}),
		"symlink-escape":      append(official, member{name: top + "bin/escape", typ: tar.TypeSymlink, link: "../../outside"}),
		"symlink-absolute":    append(official, member{name: top + "bin/escape", typ: tar.TypeSymlink, link: "/etc"}),
		"write-through-link":  append(official, member{name: top + "lib", typ: tar.TypeSymlink, link: "../.."}, member{name: top + "lib/victim", body: "x", mode: 0o644}),
		"hardlink":            append(official, member{name: top + "bin/hard", typ: tar.TypeLink, link: top + "bin/payload"}),
		"device":              append(official, member{name: top + "bin/null", typ: tar.TypeChar, mode: 0o666}),
		"fifo":                append(official, member{name: top + "bin/pipe", typ: tar.TypeFifo, mode: 0o644}),
		"two-top-directories": append(official, member{name: "other/", typ: tar.TypeDir, mode: 0o755}),
		"top-level-file":      append([]member{{name: "README", body: "x", mode: 0o644}}, official...),
		"duplicate":           append(official, member{name: top + "bin/payload", body: "again", mode: 0o644}),
		"missing-executable":  official[:2],
		"empty":               nil,
	}
	for name, members := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "archive.tar.gz")
			if err := os.WriteFile(path, archive(t, members), 0o644); err != nil {
				t.Fatal(err)
			}
			victim := filepath.Join(root, "victim")
			destination := filepath.Join(root, "nested", "app")
			if err := os.Mkdir(filepath.Dir(destination), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := extractTarball(t.Context(), path, destination); err == nil {
				t.Fatal("unsafe archive accepted")
			}
			if exists(victim) || exists(filepath.Join(root, "escape")) || exists(filepath.Join(root, "nested", "escape")) {
				t.Fatal("archive escaped its destination")
			}
		})
	}
}

func TestExtractionProducesOrdinaryTree(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "archive.tar.gz")
	top := "jetbrains-toolbox-" + current + "/"
	members := append(officialMembers(), member{name: top + "bin/setuid", body: "x", mode: 0o4755}, member{name: top + "bin/late/", typ: tar.TypeDir, mode: 0o700}, member{name: top + "bin/late/file", body: "x", mode: 0o600})
	if err := os.WriteFile(path, archive(t, members), 0o644); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "app")
	if err := extractTarball(t.Context(), path, destination); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]os.FileMode{"bin": 0o755, "bin/jetbrains-toolbox": 0o755, "bin/payload": 0o644, "bin/setuid": 0o755, "bin/late": 0o755, "bin/late/file": 0o644} {
		info, err := os.Lstat(filepath.Join(destination, name))
		if err != nil || info.Mode().Perm() != want || info.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
			t.Fatalf("%s: %v %v", name, info, err)
		}
	}
	if target, err := os.Readlink(filepath.Join(destination, "bin/lib/link")); err != nil || target != "../jetbrains-toolbox" {
		t.Fatalf("symlink: %q %v", target, err)
	}
	if exists(filepath.Join(destination, top)) {
		t.Fatal("top-level directory was not stripped")
	}
	if err := extractTarball(t.Context(), path, destination); err == nil {
		t.Fatal("extraction into an existing directory accepted")
	}
}
