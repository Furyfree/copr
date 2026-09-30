package copilot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/Furyfree/copr/internal/pins"
)

// ParseRelease validates the release selected by the signed installer RPM.
func ParseRelease(data []byte) (Artifact, error) {
	var a Artifact
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&a); err != nil {
		return a, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return a, errors.New("expected one release object")
	}
	if a.SchemaVersion != 1 || !stableVersion.MatchString(a.Version) || a.Name != "github" || a.Arch != "x86_64" || a.Path != "" || !digestPattern.MatchString(a.SHA256) || a.Source != sourceURL(a.Version) {
		return a, errors.New("invalid packaged Copilot release")
	}
	return a, nil
}

func PackagedRelease() (Artifact, error) {
	data, err := pins.Lookup(pins.Copilot)
	if err != nil {
		return Artifact{}, err
	}
	return ParseRelease(data)
}

// LatestRelease reads metadata only; application bytes never enter the SRPM.
func LatestRelease(ctx context.Context) (Artifact, error) {
	a, _, err := New().resolve(ctx, "")
	return a, err
}

// Install reconciles the package-selected application under the lifecycle lock.
// The RPM's automatic job invokes this after installation or upgrade. An
// explicit version replaces the package selection with GitHub's published digest.
func (e *Installer) Install(ctx context.Context, version string) (Status, error) {
	var result Status
	a, err := e.release()
	if err != nil {
		return result, err
	}
	pinned := version == "" || version == a.Version
	lock, err := e.lock(ctx)
	if err != nil {
		return result, err
	}
	defer lock.Close()
	if err := e.checkLegacyRPM(ctx); err != nil {
		return result, err
	}
	current, err := e.Status()
	if err != nil {
		return result, err
	}
	if current.CleanupPending {
		return result, errors.New("complete interrupted Copilot removal before installing")
	}
	if !pinned {
		a.Version = version
	}
	if current.Installed {
		if older(a.Version, current.Version) {
			return result, errors.New("refusing to downgrade GitHub Copilot")
		}
		if a.Version == current.Version {
			if pinned && a.SHA256 != current.SHA256 {
				return result, errors.New("installed Copilot digest differs from the release selected by this package")
			}
			if current.Integrated {
				return current, nil
			}
			// The retained verified artifact can repair integration without a download.
			return e.apply(ctx, current.Path, current.SHA256, current.Version)
		}
	}
	resolved, size, err := e.resolve(ctx, a.Version)
	if err != nil {
		return result, err
	}
	if pinned && resolved != a {
		return result, errors.New("GitHub release no longer matches the SHA-256 selected by this package")
	}
	stage, err := os.MkdirTemp(filepath.Dir(e.app), ".github-copilot-download-")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(stage)
	prepared, err := e.prepare(ctx, stage, resolved, size)
	if err != nil {
		return result, err
	}
	return e.apply(ctx, prepared.Path, resolved.SHA256, resolved.Version)
}
