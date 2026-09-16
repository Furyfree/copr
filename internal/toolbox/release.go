package toolbox

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

//go:embed release.json
var packagedRelease []byte

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
	if a.SchemaVersion != 1 || !buildPattern.MatchString(a.Version) || a.Name != "jetbrains-toolbox" || a.Arch != "x86_64" || a.Path != "" || !digestPattern.MatchString(a.SHA256) || a.Source != sourceURL(a.Version) {
		return a, errors.New("invalid packaged JetBrains Toolbox release")
	}
	return a, nil
}

func PackagedRelease() (Artifact, error) { return ParseRelease(packagedRelease) }

// LatestRelease reads metadata only; application bytes never enter the SRPM.
func LatestRelease(ctx context.Context) (Artifact, error) {
	a, _, err := New().resolve(ctx, "")
	return a, err
}

// Install reconciles the package-selected application under the lifecycle lock.
// The RPM's automatic job invokes this after installation or upgrade.
func (e *Installer) Install(ctx context.Context) (Status, error) {
	var result Status
	a, err := e.release()
	if err != nil {
		return result, err
	}
	lock, err := e.lock(ctx)
	if err != nil {
		return result, err
	}
	defer lock.Close()
	current, err := e.Status()
	if err != nil {
		return result, err
	}
	if current.CleanupPending {
		return result, errors.New("complete interrupted JetBrains Toolbox removal before installing")
	}
	if current.Installed {
		if older(a.Version, current.Version) {
			return result, errors.New("refusing to downgrade JetBrains Toolbox")
		}
		if a.Version == current.Version {
			if a.SHA256 != current.SHA256 {
				return result, errors.New("installed JetBrains Toolbox digest differs from the release selected by this package")
			}
			if current.Integrated {
				return current, nil
			}
			// The retained verified artifact can repair integration without a download.
			return e.apply(ctx, current.Path, a.SHA256, a.Version)
		}
	}
	resolved, size, err := e.resolve(ctx, a.Version)
	if err != nil {
		return result, err
	}
	if resolved != a {
		return result, errors.New("upstream metadata differs from the JetBrains Toolbox release selected by this package")
	}
	stage, err := os.MkdirTemp(filepath.Dir(e.app), ".jetbrains-toolbox-download-")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(stage)
	prepared, err := e.prepare(ctx, stage, a, size)
	if err != nil {
		return result, err
	}
	return e.apply(ctx, prepared.Path, a.SHA256, a.Version)
}
