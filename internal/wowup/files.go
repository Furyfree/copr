package wowup

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

type Entry struct {
	Type   string  `json:"type"`
	Mode   *uint32 `json:"mode,omitempty"`
	SHA256 string  `json:"sha256,omitempty"`
	Target string  `json:"target,omitempty"`
}
type Receipt struct {
	SchemaVersion int              `json:"schema_version"`
	Owner         string           `json:"owner"`
	Version       string           `json:"version"`
	SHA256        string           `json:"sha256"`
	Source        string           `json:"source"`
	Entries       map[string]Entry `json:"entries"`
}

func exists(path string) bool { _, err := os.Lstat(path); return !errors.Is(err, os.ErrNotExist) }
func (e *Installer) owned(info fs.FileInfo, path string) error {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != e.uid || st.Gid != e.gid || info.Mode()&os.ModeSymlink == 0 && info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("unsafe ownership or permissions: %s", path)
	}
	return nil
}
func (e *Installer) directoryChain(path string, create bool) error {
	var missing []string
	for current := path; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			missing = append(missing, current)
		} else if err != nil {
			return err
		} else {
			if !info.IsDir() {
				return fmt.Errorf("parent is not a real directory: %s", current)
			}
			if e.fixtureRoot == "" || within(e.fixtureRoot, current) {
				if err := e.owned(info, current); err != nil {
					return err
				}
			}
		}
		if current == filepath.Dir(current) {
			break
		}
	}
	if create {
		for i := len(missing) - 1; i >= 0; i-- {
			if err := e.mkdir(missing[i], 0o755); err != nil {
				return err
			}
			if err := os.Chmod(missing[i], 0o755); err != nil {
				return err
			}
		}
	}
	return nil
}
func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && (rel == "." || filepath.IsLocal(rel))
}

func (e *Installer) inventory(directory string) (map[string]Entry, error) {
	entries := map[string]Entry{}
	err := filepath.WalkDir(directory, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == directory {
			return nil
		}
		rel, err := filepath.Rel(directory, path)
		if err != nil {
			return err
		}
		if rel == "receipt.json" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if err := e.owned(info, path); err != nil {
			return err
		}
		entry := Entry{}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			resolved, err := resolveMissing(path, map[string]bool{})
			if err != nil || !within(directory, resolved) {
				return fmt.Errorf("application symlink escapes its directory: %s", rel)
			}
			entry = Entry{Type: "symlink", Target: target}
		} else {
			if info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
				return fmt.Errorf("application has privileged permission bits: %s", rel)
			}
			entry.Mode = new(uint32(info.Mode().Perm()))
			switch {
			case info.IsDir():
				entry.Type = "directory"
			case info.Mode().IsRegular() && info.Sys().(*syscall.Stat_t).Nlink == 1:
				entry.Type = "file"
				entry.SHA256, err = fileHash(path)
				if err != nil {
					return err
				}
			default:
				return fmt.Errorf("unsupported application file: %s", rel)
			}
		}
		entries[filepath.ToSlash(rel)] = entry
		return nil
	})
	return entries, err
}

// Resolve existing symlinks even when their final target was removed during retry.
func resolveMissing(path string, seen map[string]bool) (string, error) {
	if seen[path] {
		return "", errors.New("symlink loop")
	}
	info, err := os.Lstat(path)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		seen[path] = true
		target, err := os.Readlink(path)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}
		return resolveMissing(target, seen)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return path, nil
	}
	resolved, err := resolveMissing(parent, seen)
	return filepath.Join(resolved, filepath.Base(path)), err
}

func (e *Installer) readReceipt(path string) (*Receipt, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if err := e.owned(info, path); err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Sys().(*syscall.Stat_t).Nlink != 1 || info.Mode() != 0o644 || info.Size() > 16*1024*1024 {
		return nil, errors.New("WoWUp receipt is not a regular bounded 0644 file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r Receipt
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&r); err != nil {
		return nil, err
	}
	if d.Decode(new(any)) != io.EOF {
		return nil, errors.New("receipt must contain one JSON object")
	}
	if r.SchemaVersion != 1 || r.Owner != "nimbus-wowup-cf" || !stableVersion.MatchString(r.Version) || !digestPattern.MatchString(r.SHA256) || r.Source != sourceURL(r.Version) || r.Entries == nil {
		return nil, errors.New("invalid WoWUp receipt identity")
	}
	for name, entry := range r.Entries {
		if name == "receipt.json" || name == "." || !filepath.IsLocal(name) || filepath.Clean(name) != name || strings.ContainsRune(name, 0) {
			return nil, errors.New("receipt contains unsafe relative path")
		}
		switch entry.Type {
		case "symlink":
			if entry.Mode != nil || entry.SHA256 != "" || entry.Target == "" || strings.ContainsRune(entry.Target, 0) || filepath.IsAbs(entry.Target) || !filepath.IsLocal(filepath.Join(filepath.Dir(name), entry.Target)) {
				return nil, errors.New("receipt contains unsafe symlink")
			}
		case "file", "directory":
			if entry.Target != "" || entry.Mode == nil || *entry.Mode & ^uint32(0o755) != 0 || entry.Type == "file" && !digestPattern.MatchString(entry.SHA256) || entry.Type == "directory" && entry.SHA256 != "" {
				return nil, errors.New("receipt contains unsafe file or mode")
			}
		default:
			return nil, errors.New("receipt contains unsupported entry")
		}
	}
	if r.Entries["WowUp-CF.AppImage"].Type != "file" || r.Entries["WowUp-CF.AppImage"].SHA256 != r.SHA256 || r.Entries["app"].Type != "directory" || r.Entries["launcher"].Type != "file" || r.Entries["wowup-cf.desktop"].Type != "file" {
		return nil, errors.New("receipt does not describe complete application bundle")
	}
	return &r, nil
}

func squashfsOffset(path string) (uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	var header [64]byte
	if _, err := io.ReadFull(f, header[:]); err != nil {
		return 0, err
	}
	start := binary.LittleEndian.Uint64(header[40:])
	size := binary.LittleEndian.Uint16(header[58:])
	count := binary.LittleEndian.Uint16(header[60:])
	if size != 64 || count == 0 || start > uint64(info.Size()) || uint64(size)*uint64(count) > uint64(info.Size())-start {
		return 0, errors.New("unsupported AppImage ELF section table")
	}
	offset := start + uint64(size)*uint64(count)
	if uint64(info.Size())-offset < 96 {
		return 0, errors.New("truncated SquashFS archive")
	}
	var block [96]byte
	if _, err := f.ReadAt(block[:], int64(offset)); err != nil {
		return 0, err
	}
	if string(block[:4]) != "hsqs" || binary.LittleEndian.Uint16(block[28:]) != 4 || binary.LittleEndian.Uint16(block[30:]) != 0 || binary.LittleEndian.Uint64(block[40:]) > uint64(info.Size())-offset {
		return 0, errors.New("unsupported SquashFS archive")
	}
	return offset, nil
}
func nativeExtract(ctx context.Context, artifact, destination string) error {
	offset, err := squashfsOffset(artifact)
	if err != nil {
		return err
	}
	offsetArg := strconv.FormatUint(offset, 10)
	cmd := exec.CommandContext(ctx, "/usr/bin/unsquashfs", "-lln", "-o", offsetArg, artifact)
	listing, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("list AppImage: %w", err)
	}
	if regexp.MustCompile(`(?m)^[bcps][rwxStTs-]{9}\s`).Match(listing) {
		return errors.New("AppImage contains a special file")
	}
	cmd = exec.CommandContext(ctx, "/usr/bin/unsquashfs", "-no-progress", "-no-xattrs", "-strict-errors", "-processors", "1", "-o", offsetArg, "-d", destination, artifact)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("extract AppImage: %w", err)
	}
	if err := filepath.WalkDir(destination, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if os.Geteuid() == 0 {
			if err := os.Lchown(path, 0, 0); err != nil {
				return err
			}
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		mode := fs.FileMode(0o644)
		if info.IsDir() || info.Mode()&0o111 != 0 {
			mode = 0o755
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return errors.New("extraction produced special file")
		}
		return os.Chmod(path, mode)
	}); err != nil {
		return err
	}
	info, err := os.Stat(filepath.Join(destination, "AppRun"))
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return errors.New("AppImage has no executable AppRun")
	}
	return nil
}
func syncDirectory(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}
func syncBundle(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		return syncDirectory(path)
	})
}
func rename(source, target string, exchange bool) error {
	flags := uint(unix.RENAME_NOREPLACE)
	if exchange {
		flags = unix.RENAME_EXCHANGE
	}
	return unix.Renameat2(unix.AT_FDCWD, source, unix.AT_FDCWD, target, flags)
}
func sameReceipt(a, b *Receipt) bool { return reflect.DeepEqual(a, b) }
