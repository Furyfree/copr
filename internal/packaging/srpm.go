package packaging

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func Copy(source, destination string) error {
	if err := Regular(source); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	return errors.Join(copyErr, out.Close())
}

func (b *Builder) SRPM(ctx context.Context, spec, outdir string) (string, error) {
	spec, err := filepath.Abs(spec)
	if err != nil {
		return "", err
	}
	if err = Regular(spec); err != nil {
		return "", err
	}
	if filepath.Ext(spec) != ".spec" {
		return "", errors.New("expected an RPM spec")
	}
	out, err := resolveOutput(outdir)
	if err != nil {
		return "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(spec))
	if err != nil {
		return "", err
	}
	if !Outside(out, parent) {
		return "", errors.New("outdir must be outside the package directory")
	}
	expanded, err := b.command(ctx, "", "rpmspec", "-P", spec)
	if err != nil {
		return "", err
	}
	var local []string
	pattern := regexp.MustCompile(`(?im)^(?:Source|Patch)[0-9]*:\s*(\S+)\s*$`)
	for _, match := range pattern.FindAllSubmatch(expanded, -1) {
		source := string(match[1])
		u, err := url.Parse(source)
		if err != nil {
			return "", err
		}
		if u.Scheme != "" || u.Host != "" {
			if u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
				return "", errors.New("remote sources require credential-free HTTPS")
			}
		} else {
			if filepath.Base(source) != source {
				return "", errors.New("local sources must be files beside the spec")
			}
			path := filepath.Join(parent, source)
			if err := Regular(path); err != nil {
				return "", err
			}
			local = append(local, path)
		}
	}
	root, err := os.MkdirTemp("", "copr-srpm-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(root)
	sources := filepath.Join(root, "SOURCES")
	if err := os.Mkdir(sources, 0o755); err != nil {
		return "", err
	}
	recipe := filepath.Join(root, filepath.Base(spec))
	if err := Copy(spec, recipe); err != nil {
		return "", err
	}
	seen := map[string]bool{}
	for _, path := range local {
		if seen[path] {
			continue
		}
		seen[path] = true
		if err := Copy(path, filepath.Join(sources, filepath.Base(path))); err != nil {
			return "", err
		}
	}
	if _, err := b.command(ctx, "", "spectool", "-g", "-C", sources, recipe); err != nil {
		return "", err
	}
	if _, err := b.command(ctx, "", "rpmbuild", "-bs", recipe, "--define", "_topdir "+root, "--define", "_sourcedir "+sources); err != nil {
		return "", err
	}
	archives, err := filepath.Glob(filepath.Join(root, "SRPMS", "*.src.rpm"))
	if err != nil || len(archives) != 1 {
		return "", errors.New("expected exactly one source RPM")
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return "", err
	}
	target := filepath.Join(out, filepath.Base(archives[0]))
	if err := Copy(archives[0], target); err != nil {
		return "", err
	}
	return target, nil
}

func specVersion(spec []byte) (string, error) {
	match := regexp.MustCompile(`(?m)^Version:\s+([0-9.]+)\s*$`).FindSubmatch(spec)
	if len(match) != 2 {
		return "", errors.New("missing numeric RPM version")
	}
	return strings.TrimSpace(string(match[1])), nil
}

func (b *Builder) VerifySpecs(ctx context.Context, name string) error {
	c, err := Load(b.Root)
	if err != nil {
		return err
	}
	if name != "" {
		p, ok := c.Projects[name]
		if !ok {
			return errors.New("unknown package")
		}
		c.Projects = map[string]Project{name: p}
	}
	for name := range c.Projects {
		if _, err := b.command(ctx, "", "rpmspec", "-P", filepath.Join(b.Root, "packages", name, name+".spec")); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}
