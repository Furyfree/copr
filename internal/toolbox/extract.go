package toolbox

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const maxExtracted = 4 * 1024 * 1024 * 1024
const maxEntries = 100000

// extractTarball unpacks the single top-level directory of a verified archive
// into a new destination. Only directories, regular files and internal relative
// symlinks are accepted. Symlinks are created last, so no archive member can be
// written through a link, and every path is opened relative to an os.Root.
func extractTarball(ctx context.Context, artifact, destination string) error {
	f, err := os.Open(artifact)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer gz.Close()
	if err := os.Mkdir(destination, 0o755); err != nil {
		return err
	}
	root, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer root.Close()
	type link struct{ name, target string }
	var links []link
	var total int64
	count, prefix := 0, ""
	reader := tar.NewReader(gz)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read archive: %w", err)
		}
		if header.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		count++
		if count > maxEntries {
			return errors.New("archive contains too many entries")
		}
		name := strings.TrimSuffix(header.Name, "/")
		if name == "" || strings.ContainsRune(name, 0) || filepath.IsAbs(name) || !filepath.IsLocal(name) || filepath.Clean(name) != name {
			return fmt.Errorf("archive contains unsafe path: %q", header.Name)
		}
		top, rest, nested := strings.Cut(name, "/")
		if prefix == "" {
			prefix = top
		}
		if top != prefix {
			return errors.New("archive must contain one top-level directory")
		}
		if !nested {
			if header.Typeflag != tar.TypeDir {
				return errors.New("archive must contain one top-level directory")
			}
			continue
		}
		if err := ensureParents(root, filepath.Dir(rest)); err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := root.Mkdir(rest, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if header.Size < 0 {
				return errors.New("archive entry has a negative size")
			}
			total += header.Size
			if total > maxExtracted {
				return errors.New("archive exceeds maximum extracted size")
			}
			mode := fs.FileMode(0o644)
			if header.Mode&0o111 != 0 {
				mode = 0o755
			}
			out, err := root.OpenFile(rest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			n, copyErr := io.Copy(out, io.LimitReader(reader, header.Size+1))
			closeErr := out.Close()
			if err := errors.Join(copyErr, closeErr); err != nil {
				return err
			}
			if n != header.Size {
				return errors.New("archive entry size differs from its header")
			}
		case tar.TypeSymlink:
			target := header.Linkname
			if target == "" || strings.ContainsRune(target, 0) || filepath.IsAbs(target) || !filepath.IsLocal(filepath.Join(filepath.Dir(rest), target)) {
				return fmt.Errorf("archive symlink escapes the application: %q", rest)
			}
			links = append(links, link{rest, target})
		default:
			return fmt.Errorf("archive contains unsupported entry: %q", rest)
		}
	}
	for _, l := range links {
		if err := ensureParents(root, filepath.Dir(l.name)); err != nil {
			return err
		}
		if err := root.Symlink(l.target, l.name); err != nil {
			return err
		}
	}
	if err := normalizeModes(destination); err != nil {
		return err
	}
	info, err := os.Lstat(filepath.Join(destination, "bin", "jetbrains-toolbox"))
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return errors.New("archive has no executable jetbrains-toolbox")
	}
	return nil
}

// ensureParents creates missing directories under the root; existing entries
// must be real directories because symlinks are only created after all files.
func ensureParents(root *os.Root, directory string) error {
	if directory == "." || directory == "" {
		return nil
	}
	info, err := root.Lstat(directory)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("archive path is not a directory: %q", directory)
		}
		return nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := ensureParents(root, filepath.Dir(directory)); err != nil {
		return err
	}
	return root.Mkdir(directory, 0o755)
}

// normalizeModes applies ordinary root-owned modes regardless of umask.
func normalizeModes(destination string) error {
	return filepath.WalkDir(destination, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return errors.New("extraction produced special file")
		}
		mode := fs.FileMode(0o644)
		if info.IsDir() || info.Mode()&0o111 != 0 {
			mode = 0o755
		}
		return os.Chmod(path, mode)
	})
}
