package copilot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"slices"
	"strings"
)

type Options struct {
	Command, Version, Directory, AppImage, SHA256 string
	Yes                                           bool
}

func Parse(args []string) (Options, error) {
	var o Options
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; arg {
		case "install", "update", "prepare", "apply", "status", "uninstall", "version", "help":
			if o.Command != "" {
				return o, errors.New("only one command may be specified")
			}
			o.Command = arg
		case "--help", "-h":
			o.Command = "help"
		case "--assumeyes", "-y":
			o.Yes = true
		case "--app-version", "--directory", "--appimage", "--sha256":
			i++
			if i >= len(args) {
				return o, fmt.Errorf("%s requires a value", arg)
			}
			switch arg {
			case "--app-version":
				o.Version = args[i]
			case "--directory":
				o.Directory = args[i]
			case "--appimage":
				o.AppImage = args[i]
			case "--sha256":
				o.SHA256 = args[i]
			}
		case "--":
			if i != len(args)-1 {
				return o, errors.New("positional arguments are not supported")
			}
		default:
			return o, fmt.Errorf("unknown command or option %q", arg)
		}
	}
	if o.Command == "" {
		o.Command = "help"
	}
	if o.Version != "" && !stableVersion.MatchString(o.Version) {
		return o, errors.New("invalid application version")
	}
	if o.Directory != "" && o.Command != "prepare" {
		return o, errors.New("--directory is only valid with prepare")
	}
	if (o.AppImage != "" || o.SHA256 != "") && o.Command != "apply" {
		return o, errors.New("--appimage and --sha256 are only valid with apply")
	}
	if o.Command == "prepare" && (o.Directory == "" || o.Yes) {
		return o, errors.New("prepare requires --directory and does not accept --assumeyes")
	}
	if o.Command == "apply" && (o.AppImage == "" || o.Version == "" || !digestPattern.MatchString(o.SHA256)) {
		return o, errors.New("apply requires --appimage, --app-version and lowercase --sha256")
	}
	if slices.Contains([]string{"status", "uninstall", "version", "help"}, o.Command) && o.Version != "" {
		return o, errors.New("--app-version is not valid for this command")
	}
	if slices.Contains([]string{"status", "version", "help"}, o.Command) && o.Yes {
		return o, errors.New("--assumeyes is not valid for this command")
	}
	return o, nil
}

func platformCheck() error {
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
	if values["ID"] != "fedora" || !slices.Contains([]string{"43", "44"}, values["VERSION_ID"]) || runtime.GOARCH != "amd64" {
		return errors.New("GitHub Copilot requires Fedora 43 or 44 x86_64")
	}
	return nil
}

const usage = "Usage:\n  github-copilot-installer install [--app-version VERSION] [--assumeyes]\n  github-copilot-installer update [--app-version VERSION] [--assumeyes]\n  github-copilot-installer prepare --directory DIR [--app-version VERSION]\n  github-copilot-installer apply --appimage FILE --sha256 HEX --app-version VERSION [--assumeyes]\n  github-copilot-installer status\n  github-copilot-installer uninstall [--assumeyes]\n  github-copilot-installer version\n  github-copilot-installer help\n"

// Execute keeps the command line and status text that Nimbus and Topgrade use.
// Approval happens before these commands run, so --assumeyes adds nothing.
func (e *Installer) Execute(ctx context.Context, o Options) error {
	switch o.Command {
	case "help":
		_, err := fmt.Fprint(e.out, usage)
		return err
	case "version":
		_, err := fmt.Fprintln(e.out, "github-copilot-installer", Version)
		return err
	}
	if err := e.platform(); err != nil {
		return err
	}
	switch o.Command {
	case "status":
		target, err := e.release()
		if err != nil {
			return err
		}
		s, err := e.Status()
		if err != nil {
			return err
		}
		installed := "not installed"
		switch {
		case s.CleanupPending:
			installed = "removal pending"
		case s.Installed:
			installed = s.Version
		}
		_, err = fmt.Fprintf(e.out, "Installer: %s\nSelected GitHub Copilot: %s\nSelected SHA-256: %s\nInstalled GitHub Copilot: %s\n", Version, target.Version, target.SHA256, installed)
		return err
	case "prepare":
		a, err := e.Prepare(ctx, o.Directory, o.Version)
		if err != nil {
			return err
		}
		return json.NewEncoder(e.out).Encode(a)
	case "apply":
		s, err := e.Apply(ctx, o.AppImage, o.SHA256, o.Version)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(e.out, "GitHub Copilot %s is installed.\n", s.Version)
		return err
	case "install", "update":
		if o.Command == "update" {
			s, err := e.Status()
			if err != nil {
				return err
			}
			if !s.Installed {
				return errors.New("GitHub Copilot is not installed; use install")
			}
		}
		s, err := e.Install(ctx, o.Version)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(e.out, "GitHub Copilot %s is installed.\n", s.Version)
		return err
	case "uninstall":
		if _, err := e.Uninstall(ctx); err != nil {
			return err
		}
		_, err := fmt.Fprintln(e.out, "GitHub Copilot is not installed. User data in ~/.copilot was retained.")
		return err
	}
	return fmt.Errorf("unknown command %q", o.Command)
}
