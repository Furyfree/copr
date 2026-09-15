package wowup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"golang.org/x/sys/unix"
)

func profileFixture(t *testing.T) (string, string) {
	t.Helper()
	directory := filepath.Join(t.TempDir(), "config")
	profile := filepath.Join(directory, "WowUpCf")
	if err := os.MkdirAll(filepath.Join(profile, "Cache"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"preferences.json", "Cookies", "Cache/data"} {
		if err := os.WriteFile(filepath.Join(profile, name), []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return directory, profile
}

func TestPurgePreservesOtherDataAndDoesNotFollowLinks(t *testing.T) {
	directory, profile := profileFixture(t)
	game := filepath.Join(filepath.Dir(directory), "WoW", "Interface", "AddOns", "addon")
	other := filepath.Join(directory, "OtherApp", "settings")
	for _, path := range []string{game, other} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Dir(game), filepath.Join(profile, "external")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/missing/socket", filepath.Join(profile, "SingletonSocket")); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		result, err := purgeProfile(t.Context(), directory, uint32(os.Getuid()))
		if err != nil || !result.Purged || result.Path != profile {
			t.Fatalf("purge: %+v %v", result, err)
		}
	}
	if _, err := os.Lstat(profile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("profile remains: %v", err)
	}
	for _, path := range []string{game, other} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != "keep" {
			t.Fatalf("unrelated data changed: %s %v", path, err)
		}
	}
}

func TestPurgeRejectsUnsafeProfileWithoutDeletingData(t *testing.T) {
	for _, kind := range []string{"symlink", "file", "foreign-owner", "special-file", "active-session", "unknown-session", "cancelled", "relative"} {
		t.Run(kind, func(t *testing.T) {
			directory, profile := profileFixture(t)
			uid := uint32(os.Getuid())
			ctx := t.Context()
			switch kind {
			case "symlink", "file":
				saved := filepath.Join(filepath.Dir(directory), "retained")
				if err := os.Rename(profile, saved); err != nil {
					t.Fatal(err)
				}
				var err error
				if kind == "symlink" {
					err = os.Symlink(saved, profile)
				} else {
					err = os.WriteFile(profile, []byte("not a profile"), 0o600)
				}
				if err != nil {
					t.Fatal(err)
				}
				profile = saved
			case "foreign-owner":
				uid++
			case "special-file":
				if err := unix.Mkfifo(filepath.Join(profile, "pipe"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "active-session", "unknown-session":
				host, err := os.Hostname()
				if err != nil {
					t.Fatal(err)
				}
				target := host + "-" + strconv.Itoa(os.Getpid())
				if kind == "unknown-session" {
					target = "another-computer-123"
				}
				if err := os.Symlink(target, filepath.Join(profile, "SingletonLock")); err != nil {
					t.Fatal(err)
				}
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "relative":
				directory = "relative/config"
			}
			result, err := purgeProfile(ctx, directory, uid)
			if err == nil || result.Purged {
				t.Fatalf("unsafe purge accepted: %+v %v", result, err)
			}
			for _, name := range []string{"preferences.json", "Cookies", "Cache/data"} {
				if data, err := os.ReadFile(filepath.Join(profile, name)); err != nil || string(data) != "fixture" {
					t.Fatalf("profile changed before rejection: %s %v", name, err)
				}
			}
		})
	}
}

func TestPurgeRetryAfterPartialRemoval(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission failure requires an unprivileged fixture")
	}
	directory, profile := profileFixture(t)
	if err := os.Chmod(directory, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(directory, 0o700) })
	result, err := purgeProfile(t.Context(), directory, uint32(os.Getuid()))
	if err == nil || result.Purged {
		t.Fatalf("failed final directory removal reported success: %+v %v", result, err)
	}
	entries, err := os.ReadDir(profile)
	if err != nil || len(entries) != 0 {
		t.Fatalf("expected partial removal: %v %v", entries, err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	result, err = purgeProfile(t.Context(), directory, uint32(os.Getuid()))
	if err != nil || !result.Purged {
		t.Fatalf("retry failed: %+v %v", result, err)
	}
}

func TestPurgeUsesCurrentUserConfigDirectory(t *testing.T) {
	directory, profile := profileFixture(t)
	t.Setenv("XDG_CONFIG_HOME", directory)
	t.Setenv("SUDO_USER", "someone-else")
	result, err := Purge(t.Context())
	if os.Getuid() == 0 {
		if err == nil || result.Purged {
			t.Fatal("root purge accepted")
		}
		return
	}
	if err != nil || !result.Purged || result.Path != profile {
		t.Fatalf("current user purge: %+v %v", result, err)
	}
}

func TestPurgeMissingConfigDoesNotCreateDirectories(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "absent")
	result, err := purgeProfile(t.Context(), directory, uint32(os.Getuid()))
	if err != nil || !result.Purged {
		t.Fatalf("absent: %+v %v", result, err)
	}
	if _, err := os.Stat(directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("config directory created: %v", err)
	}
}

func TestPurgeAcceptsStaleLocalSessionLock(t *testing.T) {
	directory, profile := profileFixture(t)
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	// This positive PID is beyond Linux's supported PID range.
	if err := os.Symlink(host+"-2147483647", filepath.Join(profile, "SingletonLock")); err != nil {
		t.Fatal(err)
	}
	result, err := purgeProfile(t.Context(), directory, uint32(os.Getuid()))
	if err != nil || !result.Purged {
		t.Fatalf("stale session prevented purge: %+v %v", result, err)
	}
}
