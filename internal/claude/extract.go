package claude

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const binaryPath = "usr/lib/claude-desktop/claude-desktop"
const sandboxPath = "usr/lib/claude-desktop/chrome-sandbox"
const maxExtracted = 4 * 1024 * 1024 * 1024
const maxEntries = 100000

// Native ar and xz decode the package; Go validates each tar entry before writing.
// Debian maintainer scripts are read as metadata, never executed.
func withTar(ctx context.Context, artifact, member string, visit func(*tar.Reader) error) error {
	compressed, err := os.CreateTemp("", "claude-member-")
	if err != nil {
		return err
	}
	defer os.Remove(compressed.Name())
	ar := exec.CommandContext(ctx, "/usr/bin/ar", "p", artifact, member)
	ar.Stdout = compressed
	var stderr bytes.Buffer
	ar.Stderr = &stderr
	runErr := ar.Run()
	if err := errors.Join(runErr, compressed.Close()); err != nil {
		return fmt.Errorf("read Debian archive member: %w: %s", err, &stderr)
	}
	xz := exec.CommandContext(ctx, "/usr/bin/xz", "--decompress", "--stdout", "--memlimit-decompress=256MiB", compressed.Name())
	stderr.Reset()
	xz.Stderr = &stderr
	pipe, err := xz.StdoutPipe()
	if err != nil {
		return err
	}
	if err := xz.Start(); err != nil {
		return err
	}
	bounded := &io.LimitedReader{R: pipe, N: maxExtracted + 64*1024*1024 + 1}
	readErr := visit(tar.NewReader(bounded))
	if readErr == nil {
		_, readErr = io.Copy(io.Discard, bounded)
		if bounded.N == 0 {
			readErr = errors.New("decompressed archive exceeds maximum size")
		}
	}
	if readErr != nil {
		xz.Process.Kill()
	}
	pipe.Close()
	if err := errors.Join(readErr, xz.Wait()); err != nil {
		return fmt.Errorf("decode Debian archive member: %w: %s", err, &stderr)
	}
	return nil
}

func nativeExtract(ctx context.Context, artifact, destination, version string) error {
	listing, err := exec.CommandContext(ctx, "/usr/bin/ar", "t", artifact).Output()
	if err != nil {
		return fmt.Errorf("list Debian package: %w", err)
	}
	if string(listing) != "debian-binary\ncontrol.tar.xz\ndata.tar.xz\n" {
		return errors.New("unsupported Debian archive layout")
	}
	format, err := exec.CommandContext(ctx, "/usr/bin/ar", "p", artifact, "debian-binary").Output()
	if err != nil || string(format) != "2.0\n" {
		return errors.New("unsupported Debian package format")
	}
	found := false
	if err := withTar(ctx, artifact, "control.tar.xz", func(r *tar.Reader) error {
		for count := 0; ; count++ {
			h, err := r.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return err
			}
			if count >= 100 || h.Size < 0 || h.Size > maxMetadata {
				return errors.New("Debian control archive exceeds limits")
			}
			if strings.TrimPrefix(h.Name, "./") != "control" {
				continue
			}
			if found || h.Typeflag != tar.TypeReg {
				return errors.New("Debian control must be one regular file")
			}
			found = true
			data, err := io.ReadAll(r)
			if err != nil {
				return err
			}
			f, err := fields(string(data))
			if err != nil {
				return err
			}
			if f["Package"] != "claude-desktop" || f["Architecture"] != "amd64" || f["Version"] != version {
				return errors.New("Debian control differs from the approved Claude version or architecture")
			}
		}
		if !found {
			return errors.New("Debian package has no control file")
		}
		return nil
	}); err != nil {
		return err
	}
	if err := os.Mkdir(destination, 0o755); err != nil {
		return err
	}
	root, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.Chmod(".", 0o755); err != nil {
		return err
	}
	if err := withTar(ctx, artifact, "data.tar.xz", func(r *tar.Reader) error { return extractData(ctx, r, root) }); err != nil {
		return err
	}
	return validateApplication(destination)
}

// Create symlinks last. os.Root also prevents archive paths from escaping.
func extractData(ctx context.Context, r *tar.Reader, root *os.Root) error {
	type link struct{ name, target string }
	var links []link
	var total int64
	seen := map[string]bool{}
	for count := 0; ; count++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		h, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if count >= maxEntries {
			return errors.New("archive contains too many entries")
		}
		// Anthropic's archive starts with the ./ directory record.
		if (h.Name == "./" || h.Name == ".") && h.Typeflag == tar.TypeDir {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(h.Name, "./"), "/")
		if name == "" || !filepath.IsLocal(name) || filepath.Clean(name) != name || strings.ContainsRune(name, 0) || seen[name] {
			return fmt.Errorf("archive contains unsafe or repeated path: %q", h.Name)
		}
		seen[name] = true
		if name == sandboxPath && (h.Typeflag != tar.TypeReg || h.Mode != 0o4755 || h.Uid != 0 || h.Gid != 0) {
			return errors.New("archive sandbox does not match upstream's root-owned 4755 helper")
		}
		if h.Mode&0o7000 != 0 && !(name == sandboxPath && h.Typeflag == tar.TypeReg && h.Mode == 0o4755 && h.Uid == 0 && h.Gid == 0) {
			return fmt.Errorf("archive contains unexpected privileged permission bits: %q", name)
		}
		if err := ensureParents(root, filepath.Dir(name)); err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := ensureParents(root, name); err != nil {
				return err
			}
		case tar.TypeReg:
			if h.Size < 0 || h.Size > maxExtracted-total {
				return errors.New("archive exceeds maximum extracted size")
			}
			total += h.Size
			mode := fs.FileMode(0o644)
			if h.Mode&0o111 != 0 {
				mode = 0o755
			}
			out, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			n, copyErr := io.Copy(out, r)
			chmodErr := out.Chmod(mode)
			closeErr := out.Close()
			if err := errors.Join(copyErr, chmodErr, closeErr); err != nil {
				return err
			}
			if n != h.Size {
				return errors.New("archive entry size differs from its header")
			}
		case tar.TypeSymlink:
			target := h.Linkname
			if target == "" || strings.ContainsRune(target, 0) || filepath.IsAbs(target) || !filepath.IsLocal(filepath.Join(filepath.Dir(name), target)) {
				return fmt.Errorf("archive symlink escapes application: %q", name)
			}
			links = append(links, link{name, target})
		default:
			return fmt.Errorf("archive contains unsupported entry: %q", name)
		}
	}
	for _, l := range links {
		if err := root.Symlink(l.target, l.name); err != nil {
			return err
		}
	}
	// Only this verified upstream helper may become setuid; ordinary files stay 0644/0755.
	info, err := root.Lstat(sandboxPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o755 {
		return errors.New("archive has no regular executable Chromium sandbox")
	}
	return root.Chmod(sandboxPath, os.ModeSetuid|0o755)
}

func ensureParents(root *os.Root, directory string) error {
	if directory == "." || directory == "" {
		return nil
	}
	info, err := root.Lstat(directory)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("archive parent is not a real directory: %q", directory)
		}
		return nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := ensureParents(root, filepath.Dir(directory)); err != nil {
		return err
	}
	if err := root.Mkdir(directory, 0o755); err != nil {
		return err
	}
	return root.Chmod(directory, 0o755)
}

func validateApplication(destination string) error {
	path := filepath.Join(destination, binaryPath)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o755 {
		return errors.New("archive has no executable Claude Desktop")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	var header [20]byte
	_, readErr := io.ReadFull(f, header[:])
	closeErr := f.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return err
	}
	if string(header[:7]) != "\x7fELF\x02\x01\x01" || binary.LittleEndian.Uint16(header[18:]) != 62 {
		return errors.New("application is not a Linux x86_64 ELF executable")
	}
	for _, name := range []string{"usr/lib/claude-desktop/resources/app.asar", "usr/share/icons/hicolor/256x256/apps/claude-desktop.png", "usr/share/doc/claude-desktop/copyright", "usr/lib/claude-desktop/LICENSES.chromium.html"} {
		info, err := os.Lstat(filepath.Join(destination, name))
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("application is missing required payload or license: %s", name)
		}
	}
	return nil
}
