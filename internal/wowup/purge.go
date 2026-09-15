package wowup

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

type PurgeResult struct {
	SchemaVersion int    `json:"schema_version"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	Purged        bool   `json:"purged"`
}

// Purge removes only the invoking user's Electron profile. It deliberately
// does not use SUDO_USER or share the privileged application-removal path.
func Purge(ctx context.Context) (PurgeResult, error) {
	if os.Getuid() == 0 || os.Geteuid() != os.Getuid() {
		return PurgeResult{}, errors.New("run purge as your normal user, without sudo")
	}
	directory, err := os.UserConfigDir()
	if err != nil {
		return PurgeResult{}, err
	}
	return purgeProfile(ctx, directory, uint32(os.Getuid()))
}

func purgeProfile(ctx context.Context, directory string, uid uint32) (PurgeResult, error) {
	result := PurgeResult{SchemaVersion: 1, Name: "wowup-cf", Path: filepath.Join(directory, "WowUpCf")}
	if !filepath.IsAbs(directory) {
		return result, errors.New("user configuration directory must be absolute")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	parent, err := os.OpenRoot(directory)
	if errors.Is(err, fs.ErrNotExist) {
		result.Purged = true
		return result, nil
	}
	if err != nil {
		return result, err
	}
	defer parent.Close()
	info, err := parent.Lstat("WowUpCf")
	if errors.Is(err, fs.ErrNotExist) {
		result.Purged = true
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if !info.IsDir() {
		return result, errors.New("refusing to purge a symlink or non-directory WoWUp profile")
	}
	root, err := parent.OpenRoot("WowUpCf")
	if err != nil {
		return result, err
	}
	defer root.Close()
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		return result, errors.New("WoWUp profile changed while opening it")
	}
	if err := profileIdle(root); err != nil {
		return result, err
	}
	device := info.Sys().(*syscall.Stat_t).Dev
	type entry struct {
		name string
		info fs.FileInfo
	}
	var entries []entry
	// Validate the entire tree before deleting anything. WalkDir does not follow
	// symlinks; Root confines subsequent lookups to this opened profile.
	err = fs.WalkDir(root.FS(), ".", func(name string, _ fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		current, err := root.Lstat(name)
		if err != nil {
			return err
		}
		stat := current.Sys().(*syscall.Stat_t)
		if stat.Uid != uid || stat.Dev != device {
			return fmt.Errorf("refusing foreign ownership or a mounted filesystem in WoWUp profile: %s", name)
		}
		if !current.IsDir() && !current.Mode().IsRegular() && current.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("refusing special file in WoWUp profile: %s", name)
		}
		entries = append(entries, entry{name, current})
		return nil
	})
	if err != nil {
		return result, err
	}
	for i := len(entries) - 1; i >= 0; i-- {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		e := entries[i]
		base, name := root, e.name
		if name == "." {
			base, name = parent, "WowUpCf"
		}
		current, err := base.Lstat(name)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return result, err
		}
		if !os.SameFile(e.info, current) || current.Sys().(*syscall.Stat_t).Uid != uid {
			return result, errors.New("WoWUp profile changed during purge; close WoWUp and retry")
		}
		// Remove one validated entry at a time, never recursively following links
		// or deleting files created after the inventory was taken.
		if err := base.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return result, err
		}
	}
	result.Purged = true
	return result, nil
}

func profileIdle(root *os.Root) error {
	_, err := root.Lstat("SingletonLock")
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	target, err := root.Readlink("SingletonLock")
	if err != nil {
		return errors.New("cannot verify WoWUp's session lock; close WoWUp and inspect SingletonLock")
	}
	host, err := os.Hostname()
	if err != nil {
		return err
	}
	pidText, local := strings.CutPrefix(target, host+"-")
	pid, err := strconv.Atoi(pidText)
	if !local || err != nil || pid <= 0 {
		return errors.New("cannot verify WoWUp's session lock; close WoWUp on all computers using this profile")
	}
	if err := unix.Kill(pid, 0); !errors.Is(err, unix.ESRCH) {
		return errors.New("WoWUp's session is still active; close WoWUp before purging")
	}
	return nil
}
