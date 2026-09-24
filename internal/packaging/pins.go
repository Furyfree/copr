package packaging

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Furyfree/copr/internal/copilot"
	"github.com/Furyfree/copr/internal/pins"
	"github.com/Furyfree/copr/internal/toolbox"
	"github.com/Furyfree/copr/internal/wowup"
)

// Pin compares one installer helper's packaged application release with the
// latest stable upstream release.
type Pin struct {
	Package       string
	PinnedVersion string
	LatestVersion string
	PinnedSHA256  string
	LatestSHA256  string
}

// Current reports whether the packaged version and digest still match upstream.
func (p Pin) Current() bool {
	return p.PinnedVersion == p.LatestVersion && p.PinnedSHA256 == p.LatestSHA256
}

// Status names the difference for one report line.
func (p Pin) Status() string {
	switch {
	case p.Current():
		return "current"
	case p.PinnedVersion == p.LatestVersion:
		return "digest drift"
	default:
		return "stale"
	}
}

// PinResolver supplies one helper's packaged and latest release metadata.
type PinResolver struct {
	Package string
	Pinned  func() (version, digest string, err error)
	Latest  func(ctx context.Context) (version, digest string, err error)
}

// HelperPinResolvers returns one resolver per installer helper package.
func HelperPinResolvers() []PinResolver {
	return []PinResolver{
		{
			Package: "github-copilot-installer",
			Pinned:  func() (string, string, error) { a, err := copilot.PackagedRelease(); return a.Version, a.SHA256, err },
			Latest: func(ctx context.Context) (string, string, error) {
				a, err := copilot.LatestRelease(ctx)
				return a.Version, a.SHA256, err
			},
		},
		{
			Package: "wowup-cf-installer",
			Pinned:  func() (string, string, error) { a, err := wowup.PackagedRelease(); return a.Version, a.SHA256, err },
			Latest: func(ctx context.Context) (string, string, error) {
				a, err := wowup.LatestRelease(ctx)
				return a.Version, a.SHA256, err
			},
		},
		{
			Package: "jetbrains-toolbox-installer",
			Pinned:  func() (string, string, error) { a, err := toolbox.PackagedRelease(); return a.Version, a.SHA256, err },
			Latest: func(ctx context.Context) (string, string, error) {
				a, err := toolbox.LatestRelease(ctx)
				return a.Version, a.SHA256, err
			},
		},
	}
}

// RefreshPins writes the latest stable upstream release for every installer
// helper into internal/pins/pins.json. It never publishes.
func (b *Builder) RefreshPins(ctx context.Context) error {
	copilotRelease, err := copilot.LatestRelease(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", pins.Copilot, err)
	}
	toolboxRelease, err := toolbox.LatestRelease(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", pins.Toolbox, err)
	}
	wowupRelease, err := wowup.LatestRelease(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", pins.Wowup, err)
	}
	file := pins.File{Copilot: pins.Artifact(copilotRelease), Toolbox: pins.Artifact(toolboxRelease), Wowup: pins.Artifact(wowupRelease)}
	data, err := pins.Encode(file)
	if err != nil {
		return err
	}
	path := filepath.Join(b.Root, "internal", "pins", "pins.json")
	old, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	before, err := pins.Decode(old)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	var changed []string
	for _, p := range []struct {
		name     string
		old, new pins.Artifact
	}{
		{pins.Copilot, before.Copilot, file.Copilot},
		{pins.Toolbox, before.Toolbox, file.Toolbox},
		{pins.Wowup, before.Wowup, file.Wowup},
	} {
		if p.old == p.new {
			fmt.Printf("%-32s %s unchanged\n", p.name, p.new.Version)
			continue
		}
		changed = append(changed, p.name)
		fmt.Printf("%-32s %s -> %s\n", p.name, p.old.Version, p.new.Version)
	}
	if len(changed) == 0 {
		fmt.Println("all pins current")
		return nil
	}
	for _, name := range changed {
		fmt.Printf("next: just check-package %s\n", name)
	}
	return nil
}

// Pins resolves every pin. It reads release metadata only; it never downloads
// application bytes, publishes, or changes a pin.
func Pins(ctx context.Context, resolvers []PinResolver) ([]Pin, error) {
	pins := make([]Pin, 0, len(resolvers))
	for _, r := range resolvers {
		pinnedVersion, pinnedDigest, err := r.Pinned()
		if err != nil {
			return nil, fmt.Errorf("%s: packaged release: %w", r.Package, err)
		}
		latestVersion, latestDigest, err := r.Latest(ctx)
		if err != nil {
			return nil, fmt.Errorf("%s: latest release: %w", r.Package, err)
		}
		pins = append(pins, Pin{
			Package:       r.Package,
			PinnedVersion: pinnedVersion,
			LatestVersion: latestVersion,
			PinnedSHA256:  pinnedDigest,
			LatestSHA256:  latestDigest,
		})
	}
	return pins, nil
}
