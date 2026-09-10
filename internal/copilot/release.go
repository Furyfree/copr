package copilot

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
)

//go:embed release.json
var packagedRelease []byte

// ParseRelease validates the release metadata bundled into the signed helper RPM.
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
	if a.SchemaVersion != 1 || !versionPattern.MatchString(a.Version) ||
		a.Name != "github" || a.Arch != "x86_64" || a.Path != "" ||
		!digestPattern.MatchString(a.SHA256) || a.Source != sourceURL(a.Version) {
		return a, errors.New("invalid packaged Copilot release")
	}
	return a, nil
}

func packageRelease() (Artifact, error) { return ParseRelease(packagedRelease) }

// LatestRelease resolves metadata only; the proprietary RPM never enters an SRPM.
func LatestRelease(ctx context.Context) (Artifact, error) { return New().resolve(ctx, "") }
