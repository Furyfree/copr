package wowup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const launcher = "#!/bin/sh\nexec env -u APPIMAGE -u APPIMAGE_EXIT_AFTER_INSTALL -u DEBUG \\\n  APPDIR=/opt/wowup-cf/app /opt/wowup-cf/app/AppRun \"$@\"\n"
const desktop = "[Desktop Entry]\nType=Application\nName=WoWUp with CurseForge\nComment=Manage World of Warcraft addons\nExec=/usr/local/bin/wowup-cf %U\nTryExec=/opt/wowup-cf/app/AppRun\nIcon=/opt/wowup-cf/app/usr/share/icons/hicolor/256x256/apps/wowup-cf.png\nTerminal=false\nCategories=Game;Utility;\n"

type link struct{ path, target string }
type Installer struct {
	app, removing, journal string
	links                  []link
	uid, gid               uint32
	fixtureRoot            string
	client                 *http.Client
	extract                func(context.Context, string, string) error
	mkdir                  func(string, os.FileMode) error
	remove                 func(string) error
	syncDir                func(string) error
}

func New() *Installer {
	return &Installer{app: "/opt/wowup-cf", removing: "/opt/.nimbus-wowup-cf-removing", journal: "/opt/.nimbus-wowup-cf-removal.json", links: []link{{"/usr/local/bin/wowup-cf", "/opt/wowup-cf/launcher"}, {"/usr/local/share/applications/wowup-cf.desktop", "/opt/wowup-cf/wowup-cf.desktop"}}, client: httpClient(), extract: nativeExtract, mkdir: os.Mkdir, remove: os.Remove, syncDir: syncDirectory}
}

type Status struct {
	SchemaVersion  int    `json:"schema_version"`
	Name           string `json:"name"`
	Installed      bool   `json:"installed"`
	Verified       bool   `json:"verified"`
	Integrated     bool   `json:"integrated"`
	CleanupPending bool   `json:"cleanup_pending"`
	Version        string `json:"version,omitempty"`
	SHA256         string `json:"sha256,omitempty"`
	Path           string `json:"path,omitempty"`
	Source         string `json:"source,omitempty"`
}

func (e *Installer) linksValid(installed bool) (bool, error) {
	complete := true
	for _, link := range e.links {
		if err := e.directoryChain(filepath.Dir(link.path), false); err != nil {
			return false, err
		}
		info, err := os.Lstat(link.path)
		if errors.Is(err, os.ErrNotExist) {
			complete = false
			continue
		}
		if err != nil {
			return false, err
		}
		if err := e.owned(info, link.path); err != nil {
			return false, err
		}
		target, err := os.Readlink(link.path)
		if !installed || err != nil || info.Mode()&os.ModeSymlink == 0 || target != link.target {
			return false, fmt.Errorf("foreign desktop integration path: %s", link.path)
		}
	}
	return complete, nil
}
func (e *Installer) installed() (*Receipt, error) {
	if err := e.directoryChain(filepath.Dir(e.app), false); err != nil {
		return nil, err
	}
	info, err := os.Lstat(e.app)
	if errors.Is(err, os.ErrNotExist) {
		_, err := e.linksValid(false)
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("existing WoWUp path is not an owned directory")
	}
	if err := e.owned(info, e.app); err != nil {
		return nil, err
	}
	r, err := e.readReceipt(filepath.Join(e.app, "receipt.json"))
	if err != nil {
		return nil, err
	}
	entries, err := e.inventory(e.app)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(entries, r.Entries) {
		return nil, errors.New("installed WoWUp content differs from ownership receipt")
	}
	return r, nil
}
func (e *Installer) removalPending() (*Receipt, error) {
	if err := e.directoryChain(filepath.Dir(e.app), false); err != nil {
		return nil, err
	}
	if !exists(e.journal) {
		if exists(e.removing) {
			return nil, errors.New("cleanup directory has no ownership journal")
		}
		return nil, nil
	}
	r, err := e.readReceipt(e.journal)
	if err != nil {
		return nil, err
	}
	if exists(e.app) {
		current, err := e.installed()
		if err != nil {
			return nil, err
		}
		if exists(e.removing) || !sameReceipt(current, r) {
			return nil, errors.New("removal journal conflicts with installed bundle")
		}
		_, err = e.linksValid(true)
		return r, err
	}
	if _, err := e.linksValid(false); err != nil {
		return nil, err
	}
	if exists(e.removing) {
		info, err := os.Lstat(e.removing)
		if err != nil {
			return nil, err
		}
		if err := e.owned(info, e.removing); err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, errors.New("cleanup path is not an owned directory")
		}
		entries, err := e.inventory(e.removing)
		if err != nil {
			return nil, err
		}
		for name, entry := range entries {
			if !reflect.DeepEqual(entry, r.Entries[name]) {
				return nil, errors.New("cleanup contains changed or unknown entries")
			}
		}
		receipt := filepath.Join(e.removing, "receipt.json")
		if exists(receipt) {
			other, err := e.readReceipt(receipt)
			if err != nil {
				return nil, err
			}
			if !sameReceipt(other, r) {
				return nil, errors.New("cleanup receipt differs from journal")
			}
		}
	}
	return r, nil
}
func (e *Installer) Status() (Status, error) {
	result := Status{SchemaVersion: 1, Name: "wowup-cf"}
	pending, err := e.removalPending()
	if err != nil {
		return result, err
	}
	r := pending
	if r == nil {
		r, err = e.installed()
		if err != nil {
			return result, err
		}
	}
	result.Installed, result.Verified, result.CleanupPending = r != nil, r != nil, pending != nil
	if pending == nil {
		result.Integrated, err = e.linksValid(r != nil)
		if err != nil {
			return result, err
		}
	}
	if r != nil {
		result.Version, result.SHA256, result.Source, result.Path = r.Version, r.SHA256, r.Source, filepath.Join(e.app, "WowUp-CF.AppImage")
	}
	return result, nil
}
func (e *Installer) lock(ctx context.Context) (*os.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if e.fixtureRoot == "" && os.Geteuid() != 0 {
		return nil, errors.New("this command changes system files and requires root")
	}
	if err := e.directoryChain(filepath.Dir(e.app), true); err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Dir(e.app))
	if err != nil {
		return nil, err
	}
	for {
		err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, unix.EWOULDBLOCK) {
			f.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}

	return f, nil
}
func writeFile(path string, data []byte, mode os.FileMode) error {
	if err := os.WriteFile(path, data, mode); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}
func copyArtifact(source, destination string) error {
	if err := regular(source); err != nil {
		return err
	}
	in, err := os.OpenFile(source, os.O_RDONLY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > maxArtifact {
		return errors.New("prepared AppImage is not a bounded regular file")
	}
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(out, io.LimitReader(in, maxArtifact+1))
	closeErr := out.Close()
	if n > maxArtifact {
		return errors.New("prepared AppImage exceeds maximum size")
	}
	return errors.Join(copyErr, closeErr)
}
func (e *Installer) Apply(ctx context.Context, artifact, digest, version string) (result Status, err error) {
	if !stableVersion.MatchString(version) || !digestPattern.MatchString(digest) {
		return result, errors.New("apply requires stable version and lowercase SHA-256")
	}
	lock, err := e.lock(ctx)
	if err != nil {
		return result, err
	}
	defer lock.Close()
	pending, err := e.removalPending()
	if err != nil {
		return result, err
	}
	if pending != nil {
		return result, errors.New("complete interrupted WoWUp removal before installing")
	}
	previous, err := e.installed()
	if err != nil {
		return result, err
	}
	if _, err := e.linksValid(previous != nil); err != nil {
		return result, err
	}
	if previous != nil && older(version, previous.Version) {
		return result, errors.New("refusing to downgrade WoWUp")
	}
	stage, err := os.MkdirTemp(filepath.Dir(e.app), ".nimbus-wowup-cf-")
	if err != nil {
		return result, err
	}
	defer func() {
		if exists(stage) {
			err = errors.Join(err, os.RemoveAll(stage), e.syncDir(filepath.Dir(e.app)))
		}
	}()
	copied := filepath.Join(stage, "WowUp-CF.AppImage")
	if err := copyArtifact(artifact, copied); err != nil {
		return result, err
	}
	if err := checkArtifact(copied, digest); err != nil {
		return result, err
	}
	if err := os.Chmod(copied, 0o644); err != nil {
		return result, err
	}
	if err := e.extract(ctx, copied, filepath.Join(stage, "app")); err != nil {
		return result, err
	}
	if err := writeFile(filepath.Join(stage, "launcher"), []byte(launcher), 0o755); err != nil {
		return result, err
	}
	if err := writeFile(filepath.Join(stage, "wowup-cf.desktop"), []byte(desktop), 0o644); err != nil {
		return result, err
	}
	entries, err := e.inventory(stage)
	if err != nil {
		return result, err
	}
	r := Receipt{1, "nimbus-wowup-cf", version, digest, sourceURL(version), entries}
	data, err := json.Marshal(r)
	if err != nil {
		return result, err
	}
	if err := writeFile(filepath.Join(stage, "receipt.json"), append(data, '\n'), 0o644); err != nil {
		return result, err
	}
	if err := os.Chmod(stage, 0o755); err != nil {
		return result, err
	}
	if err := syncBundle(stage); err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := rename(stage, e.app, previous != nil); err != nil {
		return result, err
	}
	if err := e.syncDir(filepath.Dir(e.app)); err != nil {
		return result, err
	}
	for _, link := range e.links {
		if err := e.directoryChain(filepath.Dir(link.path), true); err != nil {
			return result, err
		}
		if !exists(link.path) {
			if err := os.Symlink(link.target, link.path); err != nil {
				return result, err
			}
			if err := e.syncDir(filepath.Dir(link.path)); err != nil {
				return result, err
			}
		}
	}
	return e.Status()
}
func (e *Installer) writeJournal(r *Receipt) error {
	f, err := os.CreateTemp(filepath.Dir(e.app), ".nimbus-wowup-journal-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	encodeErr := json.NewEncoder(f).Encode(r)
	chmodErr := f.Chmod(0o644)
	syncErr := f.Sync()
	closeErr := f.Close()
	if err := errors.Join(encodeErr, chmodErr, syncErr, closeErr); err != nil {
		return err
	}
	if err := rename(f.Name(), e.journal, false); err != nil {
		return err
	}
	return e.syncDir(filepath.Dir(e.app))
}
func (e *Installer) Uninstall(ctx context.Context) (Status, error) {
	var result Status
	lock, err := e.lock(ctx)
	if err != nil {
		return result, err
	}
	defer lock.Close()
	r, err := e.removalPending()
	if err != nil {
		return result, err
	}
	if r == nil {
		r, err = e.installed()
		if err != nil {
			return result, err
		}
		if _, err := e.linksValid(r != nil); err != nil {
			return result, err
		}
		if r == nil {
			return e.Status()
		}
		for _, link := range e.links {
			if err := e.remove(link.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return result, err
			}
			if err := e.syncDir(filepath.Dir(link.path)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return result, err
			}
		}
		if err := e.writeJournal(r); err != nil {
			return result, err
		}
	}
	if exists(e.app) {
		if err := rename(e.app, e.removing, false); err != nil {
			return result, err
		}
		if err := e.syncDir(filepath.Dir(e.app)); err != nil {
			return result, err
		}
	}
	if exists(e.removing) {
		if _, err := e.removalPending(); err != nil {
			return result, err
		}
		names := make([]string, 0, len(r.Entries))
		for name := range r.Entries {
			names = append(names, name)
		}
		slices.SortFunc(names, func(a, b string) int {
			if d := strings.Count(b, "/") - strings.Count(a, "/"); d != 0 {
				return d
			}
			return strings.Compare(b, a)
		})
		for _, name := range names {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			if err := e.remove(filepath.Join(e.removing, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return result, err
			}
		}
		if err := e.remove(filepath.Join(e.removing, "receipt.json")); err != nil && !errors.Is(err, os.ErrNotExist) {
			return result, err
		}
		if err := e.remove(e.removing); err != nil {
			return result, err
		}
		if err := e.syncDir(filepath.Dir(e.app)); err != nil {
			return result, err
		}
	}
	if err := e.remove(e.journal); err != nil {
		return result, err
	}
	if err := e.syncDir(filepath.Dir(e.app)); err != nil {
		return result, err
	}
	return e.Status()
}
