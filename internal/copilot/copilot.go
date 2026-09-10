// Package copilot verifies official GitHub Copilot RPMs and delegates to DNF.
package copilot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"time"
)

const Version = "0.3.0"
const assetName = "GitHub-Copilot-linux-x64.rpm"
const identityFormat = "%{NAME}\t%{VERSION}\t%{RELEASE}\t%{ARCH}\t%{LICENSE}\t%{SUMMARY}\t%{EPOCHNUM}\n"

var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+([._+-][A-Za-z0-9]+)*$`)
var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var rpmVersionPattern = regexp.MustCompile(`^[A-Za-z0-9._+~^-]+$`)
var releasePattern = regexp.MustCompile(`^[A-Za-z0-9._+~^]+$`)

type Options struct {
	Command, Version, Directory, RPM, SHA256 string
	Yes                                      bool
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
		case "--app-version", "--directory", "--rpm", "--sha256":
			i++
			if i >= len(args) {
				return o, fmt.Errorf("%s requires a value", arg)
			}
			switch arg {
			case "--app-version":
				o.Version = args[i]
			case "--directory":
				o.Directory = args[i]
			case "--rpm":
				o.RPM = args[i]
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
	if o.Version != "" && !versionPattern.MatchString(o.Version) {
		return o, errors.New("invalid application version")
	}
	if o.Directory != "" && o.Command != "prepare" {
		return o, errors.New("--directory is only valid with prepare")
	}
	if (o.RPM != "" || o.SHA256 != "") && o.Command != "apply" {
		return o, errors.New("--rpm and --sha256 are only valid with apply")
	}
	if o.Command == "prepare" && (o.Directory == "" || o.Yes) {
		return o, errors.New("prepare requires --directory and does not accept --assumeyes")
	}
	if o.Command == "apply" && (o.RPM == "" || o.Version == "" || !digestPattern.MatchString(o.SHA256)) {
		return o, errors.New("apply requires --rpm, --app-version and lowercase --sha256")
	}
	if slices.Contains([]string{"status", "uninstall", "version", "help"}, o.Command) && o.Version != "" {
		return o, errors.New("--app-version is not valid for this command")
	}
	if slices.Contains([]string{"status", "version", "help"}, o.Command) && o.Yes {
		return o, errors.New("--assumeyes is not valid for this command")
	}
	return o, nil
}

type Runner func(context.Context, string, []string, bool) ([]byte, error)
type Installer struct {
	run      Runner
	client   *http.Client
	out      io.Writer
	temp     string
	root     func() bool
	platform func() error
	release  func() (Artifact, error)
}

func New() *Installer {
	e := &Installer{out: os.Stdout, temp: "/var/tmp", root: func() bool { return os.Geteuid() == 0 }, platform: platformCheck, release: packageRelease}
	e.client = &http.Client{Timeout: 120 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !officialURL(r.URL.String()) {
			return errors.New("download redirected outside official GitHub source")
		}
		return nil
	}}
	e.run = func(ctx context.Context, name string, args []string, interactive bool) ([]byte, error) {
		cmd := exec.CommandContext(ctx, name, args...)
		if interactive {
			cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, e.out, os.Stderr
			return nil, cmd.Run()
		}
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		data, err := cmd.Output()
		if err != nil {
			return data, fmt.Errorf("%s failed: %w: %s", name, err, stderr.String())
		}
		return data, nil
	}
	return e
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
func sourceURL(version string) string {
	return "https://github.com/github/app/releases/download/v" + version + "/" + assetName
}
func officialURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return false
	}
	return u.Host == "api.github.com" && strings.HasPrefix(u.Path, "/repos/github/app/releases/") || u.Host == "github.com" && strings.HasPrefix(u.Path, "/github/app/releases/download/") || u.Host == "release-assets.githubusercontent.com"
}
func (e *Installer) fetch(ctx context.Context, raw string, destination string) ([]byte, error) {
	if !officialURL(raw) {
		return nil, errors.New("download is outside official GitHub source")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "github-copilot-installer")
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub returned HTTP %d", resp.StatusCode)
	}
	if destination == "" {
		data, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024+1))
		if len(data) > 2*1024*1024 {
			return nil, errors.New("release metadata is too large")
		}
		return data, err
	}
	f, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	_, copyErr := io.Copy(f, resp.Body)
	return nil, errors.Join(copyErr, f.Close())
}

type Artifact struct {
	SchemaVersion int    `json:"schema_version"`
	Version       string `json:"version"`
	Name          string `json:"name"`
	Arch          string `json:"arch"`
	SHA256        string `json:"sha256"`
	Path          string `json:"path"`
	Source        string `json:"source"`
}

func (e *Installer) resolve(ctx context.Context, version string) (Artifact, error) {
	var a Artifact
	endpoint := "latest"
	if version != "" {
		endpoint = "tags/v" + version
	}
	data, err := e.fetch(ctx, "https://api.github.com/repos/github/app/releases/"+endpoint, "")
	if err != nil {
		return a, err
	}
	var release struct {
		Draft      *bool  `json:"draft"`
		Prerelease *bool  `json:"prerelease"`
		Tag        string `json:"tag_name"`
		Assets     []struct {
			Name   string `json:"name"`
			URL    string `json:"browser_download_url"`
			Digest string `json:"digest"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(data, &release); err != nil {
		return a, err
	}
	selected := strings.TrimPrefix(release.Tag, "v")
	if release.Draft == nil || *release.Draft || release.Prerelease == nil || *release.Prerelease || release.Tag != "v"+selected || !versionPattern.MatchString(selected) || version != "" && selected != version {
		return a, errors.New("metadata must describe selected published stable release")
	}
	count := 0
	digest := ""
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			count++
			digest = strings.ToLower(asset.Digest)
			if asset.URL != sourceURL(selected) || !strings.HasPrefix(digest, "sha256:") || !digestPattern.MatchString(strings.TrimPrefix(digest, "sha256:")) {
				return a, errors.New("release asset has invalid source or SHA-256")
			}
		}
	}
	if count != 1 {
		return a, errors.New("release must contain exactly one official RPM")
	}
	return Artifact{1, selected, "github", "x86_64", strings.TrimPrefix(digest, "sha256:"), "", sourceURL(selected)}, nil
}

type identity struct{ version, release, header string }

func parseIdentity(data []byte) (identity, error) {
	fields := strings.Split(strings.TrimSuffix(string(data), "\n"), "\t")
	if len(fields) != 7 || fields[0] != "github" || fields[3] != "x86_64" || fields[4] != "Proprietary" || fields[5] != "Tauri Copilot Application" || fields[6] != "0" || !rpmVersionPattern.MatchString(fields[1]) || !releasePattern.MatchString(fields[2]) {
		return identity{}, errors.New("unrelated or unrecognized package named github")
	}
	return identity{version: fields[1], release: fields[2]}, nil
}
func (e *Installer) installed(ctx context.Context) (*identity, error) {
	data, err := e.run(ctx, "rpm", []string{"-q", "--queryformat", identityFormat, "github"}, false)
	if err != nil {
		if exit, ok := errors.AsType[*exec.ExitError](err); ok && exit.ExitCode() == 1 {
			return nil, nil
		}
		return nil, err
	}
	i, err := parseIdentity(data)
	return &i, err
}
func hash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func (e *Installer) verify(ctx context.Context, a Artifact) (identity, error) {
	i := identity{}
	info, err := os.Lstat(a.Path)
	if err != nil {
		return i, err
	}
	if !info.Mode().IsRegular() {
		return i, errors.New("prepared RPM must be a regular file, not a symlink")
	}
	digest, err := hash(a.Path)
	if err != nil {
		return i, err
	}
	if !digestPattern.MatchString(a.SHA256) || digest != a.SHA256 {
		return i, errors.New("RPM does not match approved SHA-256")
	}
	if _, err := e.run(ctx, "rpm", []string{"--checksig", "--nosignature", a.Path}, false); err != nil {
		return i, err
	}
	data, err := e.run(ctx, "rpm", []string{"-qp", "--queryformat", identityFormat, a.Path}, false)
	if err != nil {
		return i, err
	}
	i, err = parseIdentity(data)
	if err != nil {
		return i, err
	}
	if i.version != a.Version {
		return i, errors.New("RPM version does not match selected release")
	}
	header, err := e.run(ctx, "rpm", []string{"-qp", "--queryformat", "%{SHA256HEADER}\n", a.Path}, false)
	if err != nil {
		return i, err
	}
	i.header = strings.TrimSuffix(string(header), "\n")
	if !digestPattern.MatchString(i.header) {
		return i, errors.New("RPM has no usable SHA-256 header")
	}
	return i, nil
}
func (e *Installer) checkDowngrade(ctx context.Context, selected identity) (*identity, error) {
	old, err := e.installed(ctx)
	if err != nil || old == nil {
		return old, err
	}
	expression := fmt.Sprintf("%%{lua:print(rpm.vercmp('%s-%s', '%s-%s'))}", old.version, old.release, selected.version, selected.release)
	data, err := e.run(ctx, "rpm", []string{"--eval", expression}, false)
	if err != nil {
		return nil, err
	}
	switch strings.TrimSpace(string(data)) {
	case "1":
		return nil, errors.New("refusing to downgrade GitHub Copilot")
	case "0", "-1":
		return old, nil
	default:
		return nil, errors.New("unexpected RPM version comparison")
	}
}
func (e *Installer) verifyHeader(ctx context.Context, selected identity) error {
	data, err := e.run(ctx, "rpm", []string{"-q", "--queryformat", "%{SHA256HEADER}\n", "github"}, false)
	if err != nil {
		return err
	}
	if strings.TrimSuffix(string(data), "\n") != selected.header {
		return errors.New("installed RPM header does not match approved artifact")
	}
	return nil
}
func (e *Installer) dnf(ctx context.Context, yes bool, args ...string) error {
	if yes {
		args = append([]string{"-y"}, args...)
	}
	_, err := e.run(ctx, "dnf", args, true)
	return err
}
func (e *Installer) installVerified(ctx context.Context, a Artifact, selected identity, yes bool) error {
	old, err := e.checkDowngrade(ctx, selected)
	if err != nil {
		return err
	}
	if old != nil && old.version == selected.version && old.release == selected.release {
		if err := e.verifyHeader(ctx, selected); err != nil {
			return err
		}
	}
	if err := e.dnf(ctx, yes, "install", "--setopt=localpkg_gpgcheck=0", a.Path); err != nil {
		return err
	}
	after, err := e.installed(ctx)
	if err != nil {
		return err
	}
	if after == nil || after.version != selected.version || after.release != selected.release {
		return errors.New("DNF did not install selected version")
	}
	if err := e.verifyHeader(ctx, selected); err != nil {
		return err
	}
	fmt.Fprintf(e.out, "GitHub Copilot %s-%s is installed.\n", after.version, after.release)
	return nil
}

const usage = "Usage:\n  github-copilot-installer install [--app-version VERSION] [--assumeyes]\n  github-copilot-installer update [--app-version VERSION] [--assumeyes]\n  github-copilot-installer prepare --directory DIR [--app-version VERSION]\n  github-copilot-installer apply --rpm FILE --sha256 HEX --app-version VERSION [--assumeyes]\n  github-copilot-installer status\n  github-copilot-installer uninstall [--assumeyes]\n  github-copilot-installer version\n  github-copilot-installer help\n"

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
	if slices.Contains([]string{"install", "update", "apply", "uninstall"}, o.Command) && !e.root() {
		return errors.New("this command changes system packages; requires root")
	}
	var pinned *Artifact
	if (o.Command == "install" || o.Command == "update") && o.Version == "" {
		a, err := e.release()
		if err != nil {
			return err
		}
		pinned, o.Version = &a, a.Version
	}
	if o.Command == "status" {
		target, err := e.release()
		if err != nil {
			return err
		}
		i, err := e.installed(ctx)
		if err != nil {
			return err
		}
		version := "not installed"
		if i != nil {
			version = i.version + "-" + i.release
		}
		_, err = fmt.Fprintf(e.out, "Installer: %s\nSelected GitHub Copilot: %s\nSelected SHA-256: %s\nInstalled GitHub Copilot: %s\n", Version, target.Version, target.SHA256, version)
		return err
	}
	if o.Command == "uninstall" {
		i, err := e.installed(ctx)
		if err != nil {
			return err
		}
		if i == nil {
			fmt.Fprintln(e.out, "GitHub Copilot is not installed.")
			return nil
		}
		if err := e.dnf(ctx, o.Yes, "remove", "github"); err != nil {
			return err
		}
		i, err = e.installed(ctx)
		if err != nil {
			return err
		}
		if i != nil {
			return errors.New("DNF completed but GitHub Copilot is still installed")
		}
		fmt.Fprintln(e.out, "GitHub Copilot was removed. User data in ~/.copilot was retained.")
		return nil
	}
	if o.Command == "update" || o.Command == "install" {
		installed, err := e.installed(ctx)
		if err != nil {
			return err
		}
		if o.Command == "update" && installed == nil {
			return errors.New("GitHub Copilot is not installed; use install")
		}
	}
	parent := e.temp
	if o.Command == "prepare" {
		var err error
		parent, err = filepath.Abs(o.Directory)
		if err != nil {
			return err
		}
		parent, err = filepath.EvalSymlinks(parent)
		if err != nil {
			return err
		}
	}
	stage, err := os.MkdirTemp(parent, "github-copilot-installer.")
	if err != nil {
		return err
	}
	keep := false
	defer func() {
		if !keep {
			os.RemoveAll(stage)
		}
	}()
	a := Artifact{1, o.Version, "github", "x86_64", o.SHA256, filepath.Join(stage, assetName), sourceURL(o.Version)}
	if o.Command == "apply" {
		info, err := os.Lstat(o.RPM)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("prepared RPM must be regular, not a symlink")
		}
		in, err := os.OpenFile(o.RPM, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
		if err != nil {
			return err
		}
		defer in.Close()
		info, err = in.Stat()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("prepared RPM must be regular")
		}
		out, err := os.OpenFile(a.Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, in)
		if err := errors.Join(copyErr, out.Close()); err != nil {
			return err
		}
	} else {
		selected, err := e.resolve(ctx, o.Version)
		if err != nil {
			return err
		}
		if pinned != nil && selected.SHA256 != pinned.SHA256 {
			return errors.New("GitHub release no longer matches the SHA-256 selected by this package")
		}
		selected.Path = a.Path
		a = selected
		if _, err := e.fetch(ctx, a.Source, a.Path); err != nil {
			return err
		}
	}
	verified, err := e.verify(ctx, a)
	if err != nil {
		return err
	}
	if o.Command == "prepare" {
		if _, err := e.checkDowngrade(ctx, verified); err != nil {
			return err
		}
		if err := json.NewEncoder(e.out).Encode(a); err != nil {
			return err
		}
		keep = true
		return nil
	}
	return e.installVerified(ctx, a, verified, o.Yes)
}
