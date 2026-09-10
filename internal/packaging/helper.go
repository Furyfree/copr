package packaging

import (
	"context"
	"os"
	"path/filepath"
)

func (b *Builder) prepareHelper(ctx context.Context, name, out string) (string, error) {
	packageDir := filepath.Join(b.Root, "packages", name)
	output, err := resolveOutput(out)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(packageDir)
	if err != nil {
		return "", err
	}
	if !Outside(output, resolved) {
		return "", errOutput
	}
	recipe, err := os.ReadFile(filepath.Join(packageDir, name+".spec"))
	if err != nil {
		return "", err
	}
	version, err := specVersion(recipe)
	if err != nil {
		return "", err
	}
	root, err := os.MkdirTemp("", "copr-helper-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(root)
	tree := filepath.Join(root, "source")
	if err := os.Mkdir(tree, 0o755); err != nil {
		return "", err
	}
	for _, file := range []string{"go.mod", "go.sum"} {
		if err := Copy(filepath.Join(b.Root, file), filepath.Join(tree, file)); err != nil {
			return "", err
		}
	}
	implementation := "copilot"
	if name == "wowup-cf-installer" {
		implementation = "wowup"
	}
	for _, path := range []string{filepath.Join("cmd", name), filepath.Join("internal", implementation)} {
		if err := copyTree(filepath.Join(b.Root, path), filepath.Join(tree, path)); err != nil {
			return "", err
		}
	}
	for _, file := range []string{"LICENSE", "README.md"} {
		if err := Copy(filepath.Join(packageDir, file), filepath.Join(tree, file)); err != nil {
			return "", err
		}
	}
	if err := Copy(filepath.Join(packageDir, "files", name+".1"), filepath.Join(tree, name+".1")); err != nil {
		return "", err
	}
	// Resolve only this command's dependency graph; never ship unrelated helpers.
	env := append(CleanEnvironment(), "GOWORK=off", "GOTOOLCHAIN=local")
	for _, args := range [][]string{{"mod", "tidy"}, {"mod", "vendor"}} {
		if _, err := b.Run(ctx, "go", args, tree, env); err != nil {
			return "", err
		}
	}
	prepared := filepath.Join(root, "recipe")
	if err := os.Mkdir(prepared, 0o755); err != nil {
		return "", err
	}
	if err := bundle(tree, filepath.Join(prepared, name+"-"+version+"-vendor.tar.gz"), name+"-"+version); err != nil {
		return "", err
	}
	spec := filepath.Join(prepared, name+".spec")
	if err := os.WriteFile(spec, recipe, 0o644); err != nil {
		return "", err
	}
	return b.SRPM(ctx, spec, output)
}
