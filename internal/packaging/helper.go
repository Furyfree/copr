package packaging

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"

	"github.com/Furyfree/copr/internal/copilot"
	"github.com/Furyfree/copr/internal/pins"
	"github.com/Furyfree/copr/internal/toolbox"
	"github.com/Furyfree/copr/internal/wowup"
)

// Each installer helper command builds on one self-contained implementation package.
var helperImplementations = map[string]string{"github-copilot-installer": "copilot", "wowup-cf-installer": "wowup", "jetbrains-toolbox-installer": "toolbox"}

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
	implementation, ok := helperImplementations[name]
	if !ok {
		return "", errors.New("unknown installer helper")
	}
	for _, path := range []string{filepath.Join("cmd", name), filepath.Join("internal", implementation), filepath.Join("internal", "pins")} {
		if err := copyTree(filepath.Join(b.Root, path), filepath.Join(tree, path)); err != nil {
			return "", err
		}
	}
	data, err := os.ReadFile(filepath.Join(b.Root, "internal", "pins", "pins.json"))
	if err != nil {
		return "", err
	}
	var override []byte
	switch name {
	case pins.Copilot:
		if b.CopilotRelease != nil {
			override, err = json.Marshal(b.CopilotRelease)
		}
	case pins.Wowup:
		if b.WowupRelease != nil {
			override, err = json.Marshal(b.WowupRelease)
		}
	case pins.Toolbox:
		if b.ToolboxRelease != nil {
			override, err = json.Marshal(b.ToolboxRelease)
		}
	}
	if err != nil {
		return "", err
	}
	if override != nil {
		data, err = pins.Replace(data, name, override)
		if err != nil {
			return "", err
		}
	}
	raw, err := pins.LookupIn(data, name)
	if err != nil {
		return "", err
	}
	var appVersion string
	switch name {
	case pins.Copilot:
		a, err := copilot.ParseRelease(raw)
		if err != nil {
			return "", err
		}
		appVersion = a.Version
	case pins.Wowup:
		a, err := wowup.ParseRelease(raw)
		if err != nil {
			return "", err
		}
		appVersion = a.Version
	case pins.Toolbox:
		a, err := toolbox.ParseRelease(raw)
		if err != nil {
			return "", err
		}
		appVersion = a.Version
	}
	macro := regexp.MustCompile(`(?m)^%global app_version \S+$`)
	if len(macro.FindAll(recipe, -1)) != 1 {
		return "", errors.New("expected one app_version macro")
	}
	recipe = macro.ReplaceAll(recipe, []byte("%global app_version "+appVersion))
	if err := os.WriteFile(filepath.Join(tree, "internal", "pins", "pins.json"), data, 0o644); err != nil {
		return "", err
	}
	if err := Copy(filepath.Join(packageDir, "files", name+".service"), filepath.Join(tree, name+".service")); err != nil {
		return "", err
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
