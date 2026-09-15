package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/Furyfree/copr/internal/wowup"
)

func run(ctx context.Context, args []string) error {
	if len(args) == 1 && args[0] == "--version" {
		fmt.Println("wowup-cf-installer", wowup.Version)
		return nil
	}
	if len(args) == 0 {
		return errors.New("expected install, update, prepare, apply, release, status, uninstall or purge")
	}
	f := flag.NewFlagSet(args[0], flag.ContinueOnError)
	var directory, version, artifact, digest string
	var yes bool
	switch args[0] {
	case "prepare":
		f.StringVar(&directory, "directory", "", "existing preparation directory")
		f.StringVar(&version, "app-version", "", "stable application version")
	case "apply":
		f.StringVar(&artifact, "appimage", "", "prepared AppImage")
		f.StringVar(&digest, "sha256", "", "approved SHA-256")
		f.StringVar(&version, "app-version", "", "approved version")
		f.BoolVar(&yes, "assumeyes", false, "approved mutation")
	case "install", "update", "uninstall", "purge":
		f.BoolVar(&yes, "assumeyes", false, "approved mutation")
	case "release", "status":
		f.Bool("json", false, "print JSON")
	case "--help", "-h", "help":
		fmt.Println("wowup-cf-installer install --assumeyes\n  update --assumeyes\n  release --json\n  prepare --directory DIR [--app-version VERSION]\n  apply --appimage FILE --sha256 HEX --app-version VERSION --assumeyes\n  status --json\n  uninstall --assumeyes\n  purge --assumeyes (without sudo; deletes your WoWUp settings, sessions and caches)")
		return nil
	default:
		return errors.New("unknown command")
	}
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return errors.New("unexpected arguments")
	}
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return err
	}
	values := map[string]string{}
	for line := range strings.SplitSeq(string(data), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			values[key] = strings.Trim(value, "\"'")
		}
	}
	if values["ID"] != "fedora" || values["VERSION_ID"] != "44" || runtime.GOARCH != "amd64" {
		return errors.New("WoWUp requires Fedora 44 x86_64")
	}
	e := wowup.New()
	var result any
	switch args[0] {
	case "prepare":
		if directory == "" {
			return errors.New("prepare requires --directory")
		}
		result, err = e.Prepare(ctx, directory, version)
	case "apply":
		if artifact == "" || version == "" || digest == "" || !yes {
			return errors.New("apply requires --appimage, --sha256, --app-version and --assumeyes")
		}
		result, err = e.Apply(ctx, artifact, digest, version)
	case "install", "update":
		if !yes {
			return errors.New("install/update requires --assumeyes")
		}
		result, err = e.Install(ctx)
	case "release":
		result, err = wowup.PackagedRelease()
	case "uninstall":
		if !yes {
			return errors.New("uninstall requires --assumeyes")
		}
		result, err = e.Uninstall(ctx)
	case "purge":
		if !yes {
			return errors.New("purge requires --assumeyes to delete your WoWUp settings, sessions, logs and caches")
		}
		result, err = wowup.Purge(ctx)
	case "status":
		result, err = e.Status()
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "wowup-cf:", err)
		os.Exit(1)
	}
}
