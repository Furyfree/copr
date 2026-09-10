package packaging

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func bundle(source, destination, prefix string) (err error) {
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	zipped := gzip.NewWriter(out)
	archive := tar.NewWriter(zipped)
	defer func() { err = errors.Join(err, archive.Close(), zipped.Close(), out.Close()) }()
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		link := ""
		if info.Mode()&os.ModeSymlink != 0 {
			link, err = os.Readlink(path)
			if err != nil {
				return err
			}
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil || Outside(resolved, source) {
				return errors.New("source symlink escapes bundle")
			}
		} else if !info.IsDir() && !info.Mode().IsRegular() {
			return errors.New("unsupported source file type")
		}
		header, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(filepath.Join(prefix, rel))
		header.Uid, header.Gid, header.Uname, header.Gname = 0, 0, "root", "root"
		header.ModTime, header.AccessTime, header.ChangeTime = time.Unix(0, 0), time.Time{}, time.Time{}
		header.Mode = 0o644
		if info.IsDir() || info.Mode()&0o111 != 0 {
			header.Mode = 0o755
		}
		if err := archive.WriteHeader(header); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			in, err := os.Open(path)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(archive, in)
			return errors.Join(copyErr, in.Close())
		}
		return nil
	})
}

func extractSource(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	zipped, err := gzip.NewReader(in)
	if err != nil {
		return err
	}
	defer zipped.Close()
	root, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer root.Close()
	archive := tar.NewReader(zipped)
	for {
		h, err := archive.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		// GitHub archives include a global PAX header carrying the commit ID.
		if h.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		name := filepath.Clean(h.Name)
		if !filepath.IsLocal(name) {
			return errors.New("source archive contains an unsafe path")
		}
		if err := root.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(name, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			mode := fs.FileMode(0o644)
			if h.Mode&0o111 != 0 {
				mode = 0o755
			}
			out, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, archive)
			if err := errors.Join(copyErr, out.Close()); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if filepath.IsAbs(h.Linkname) || !filepath.IsLocal(filepath.Join(filepath.Dir(name), h.Linkname)) {
				return errors.New("source symlink escapes archive")
			}
			if err := root.Symlink(h.Linkname, name); err != nil {
				return err
			}
		case tar.TypeLink:
			if !filepath.IsLocal(h.Linkname) {
				return errors.New("source hardlink escapes archive")
			}
			if err := root.Link(h.Linkname, name); err != nil {
				return err
			}
		default:
			return errors.New("source archive contains an unsupported file")
		}
	}
}

func copyTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !strings.HasSuffix(path, ".go") && !strings.HasPrefix(rel, "testdata"+string(os.PathSeparator)) {
			return nil
		}
		return Copy(path, target)
	})
}
