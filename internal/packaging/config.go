// Package packaging prepares source RPMs and publishes selected COPR projects.
package packaging

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/Furyfree/copr/internal/copilot"
	"github.com/pelletier/go-toml/v2"
)

type Project struct {
	Name        string   `toml:"name"`
	Chroots     []string `toml:"chroots"`
	Description string   `toml:"description"`
	Homepage    string   `toml:"homepage"`
}

type Config struct {
	Projects map[string]Project `toml:"projects"`
}

func Load(root string) (Config, error) {
	var c Config
	b, err := os.ReadFile(filepath.Join(root, ".copr/projects.toml"))
	if err != nil {
		return c, err
	}
	err = toml.NewDecoder(bytes.NewReader(b)).DisallowUnknownFields().Decode(&c)
	if err != nil {
		return c, err
	}
	if len(c.Projects) == 0 {
		return c, errors.New("no configured packages")
	}
	for name, p := range c.Projects {
		if !regexp.MustCompile(`^[a-z][a-z0-9-]*$`).MatchString(name) || p.Name != name || len(p.Chroots) == 0 {
			return c, fmt.Errorf("invalid project %q", name)
		}
	}
	return c, nil
}

// Runner keeps native tool invocation visible and replaceable by test fixtures.
type Runner func(context.Context, string, []string, string, []string) ([]byte, error)

func Run(ctx context.Context, name string, args []string, dir string, env []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir, cmd.Env = dir, env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s failed: %w: %s", name, err, stderr.String())
	}
	return output, nil
}

func CleanEnvironment() []string {
	return slices.DeleteFunc(os.Environ(), func(s string) bool { return strings.HasPrefix(s, "COPR_CONFIG=") })
}

type Builder struct {
	Root           string
	Run            Runner
	CopilotRelease *copilot.Artifact
}

func New(root string) *Builder { return &Builder{Root: root, Run: Run} }

func (b *Builder) command(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	return b.Run(ctx, name, args, dir, CleanEnvironment())
}

func (b *Builder) Prepare(ctx context.Context, name, out string) (string, error) {
	c, err := Load(b.Root)
	if err != nil {
		return "", err
	}
	if _, ok := c.Projects[name]; !ok {
		return "", errors.New("select a configured package")
	}
	switch name {
	case "voxtype":
		return b.prepareVoxtype(ctx, out)
	case "github-copilot-installer", "wowup-cf-installer":
		return b.prepareHelper(ctx, name, out)
	default:
		return b.SRPM(ctx, filepath.Join(b.Root, "packages", name, name+".spec"), out)
	}
}

func Regular(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular file: %s", path)
	}
	return nil
}

func Outside(path, parent string) bool {
	rel, err := filepath.Rel(parent, path)
	return err == nil && (rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

func resolveOutput(path string) (string, error) {
	if path == "" {
		return "", errors.New("output directory is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	parent, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return parent, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if filepath.Dir(abs) == abs {
		return "", err
	}
	parent, err = resolveOutput(filepath.Dir(abs))
	return filepath.Join(parent, filepath.Base(abs)), err
}
