package claude

import (
	"archive/tar"
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

type tarEntry struct {
	header tar.Header
	data   []byte
}

func applicationEntries() []tarEntry {
	elf := make([]byte, 64)
	copy(elf, "\x7fELF\x02\x01\x01")
	binary.LittleEndian.PutUint16(elf[18:], 62)
	entries := []tarEntry{
		{header: tar.Header{Name: "./", Mode: 0o755, Typeflag: tar.TypeDir}},
		{header: tar.Header{Name: "./" + binaryPath, Mode: 0o755, Typeflag: tar.TypeReg}, data: elf},
		{header: tar.Header{Name: "./" + sandboxPath, Mode: 0o4755, Typeflag: tar.TypeReg}, data: elf},
		{header: tar.Header{Name: "./usr/bin/claude-desktop", Typeflag: tar.TypeSymlink, Linkname: "../lib/claude-desktop/claude-desktop"}},
	}
	for _, name := range []string{"usr/lib/claude-desktop/resources/app.asar", "usr/share/icons/hicolor/256x256/apps/claude-desktop.png", "usr/share/doc/claude-desktop/copyright", "usr/lib/claude-desktop/LICENSES.chromium.html"} {
		entries = append(entries, tarEntry{header: tar.Header{Name: "./" + name, Mode: 0o644, Typeflag: tar.TypeReg}, data: []byte("fixture")})
	}
	return entries
}
func tarBytes(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var data bytes.Buffer
	writer := tar.NewWriter(&data)
	for _, entry := range entries {
		h := entry.header
		h.Size = int64(len(entry.data))
		if err := writer.WriteHeader(&h); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(entry.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestArchiveRejectsPathsLinksAndPrivilegedFiles(t *testing.T) {
	for _, tc := range []struct {
		name   string
		header tar.Header
		want   string
	}{
		{"traversal", tar.Header{Name: "../victim", Typeflag: tar.TypeReg, Mode: 0o644}, "unsafe"},
		{"absolute", tar.Header{Name: "/tmp/victim", Typeflag: tar.TypeReg, Mode: 0o644}, "unsafe"},
		{"unclean", tar.Header{Name: "usr/../victim", Typeflag: tar.TypeReg, Mode: 0o644}, "unsafe"},
		{"absolute-link", tar.Header{Name: "outside", Typeflag: tar.TypeSymlink, Linkname: "/tmp/victim"}, "escapes"},
		{"escaping-link", tar.Header{Name: "outside", Typeflag: tar.TypeSymlink, Linkname: "../victim"}, "escapes"},
		{"hardlink", tar.Header{Name: "hard", Typeflag: tar.TypeLink, Linkname: binaryPath}, "unsupported"},
		{"fifo", tar.Header{Name: "fifo", Typeflag: tar.TypeFifo}, "unsupported"},
		{"device", tar.Header{Name: "device", Typeflag: tar.TypeChar}, "unsupported"},
		{"foreign-setuid", tar.Header{Name: "privileged", Typeflag: tar.TypeReg, Mode: 0o4755}, "privileged"},
		{"repeated-file", tar.Header{Name: binaryPath, Typeflag: tar.TypeReg, Mode: 0o755}, "repeated"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			destination := t.TempDir()
			root, err := os.OpenRoot(destination)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			entries := append(applicationEntries(), tarEntry{header: tc.header})
			err = extractData(t.Context(), tar.NewReader(bytes.NewReader(tarBytes(t, entries))), root)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("wrong rejection: %v; want %q", err, tc.want)
			}
		})
	}
}

func TestSandboxRequiresExactUpstreamIdentity(t *testing.T) {
	for _, tc := range []struct {
		mode int64
		uid  int
	}{{0o755, 0}, {0o4755, 1000}, {0o6755, 0}, {0o4777, 0}} {
		entries := applicationEntries()
		entries[2].header.Mode, entries[2].header.Uid = tc.mode, tc.uid
		root, err := os.OpenRoot(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		err = extractData(t.Context(), tar.NewReader(bytes.NewReader(tarBytes(t, entries))), root)
		root.Close()
		if err == nil || !strings.Contains(err.Error(), "sandbox does not match") {
			t.Fatalf("unexpected sandbox accepted or wrong rejection: %v", err)
		}
	}
}

func TestExtractionPreservesSandboxLicensesAndInternalLinks(t *testing.T) {
	destination := t.TempDir()
	root, err := os.OpenRoot(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	old := unix.Umask(0o077)
	defer unix.Umask(old)
	if err := extractData(t.Context(), tar.NewReader(bytes.NewReader(tarBytes(t, applicationEntries()))), root); err != nil {
		t.Fatal(err)
	}
	if err := validateApplication(destination); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(filepath.Join(destination, sandboxPath))
	if err != nil || info.Mode() != os.ModeSetuid|0o755 {
		t.Fatalf("sandbox mode: %v %v", info, err)
	}
	for _, path := range []string{binaryPath, "usr/lib", "usr/share/icons/hicolor/256x256/apps"} {
		info, err := os.Stat(filepath.Join(destination, path))
		if err != nil || info.Mode().Perm() != 0o755 {
			t.Fatalf("inaccessible extracted path: %s %v %v", path, info, err)
		}
	}
	file, err := os.Open(filepath.Join(destination, "usr/bin/claude-desktop"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(file)
	file.Close()
	if err != nil || !bytes.HasPrefix(data, []byte("\x7fELF")) {
		t.Fatal("internal executable symlink was not preserved")
	}
}
