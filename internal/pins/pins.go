// Package pins is the single checked-in installer application pin file.
package pins

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	_ "embed"
)

//go:embed pins.json
var packed []byte

const (
	Copilot = "github-copilot-installer"
	Toolbox = "jetbrains-toolbox-installer"
	Wowup   = "wowup-cf-installer"
)

// Artifact is one helper's packaged application release.
type Artifact struct {
	SchemaVersion int    `json:"schema_version"`
	Version       string `json:"version"`
	Name          string `json:"name"`
	Arch          string `json:"arch"`
	SHA256        string `json:"sha256"`
	Path          string `json:"path"`
	Source        string `json:"source"`
}

// File is the complete installer pin set.
type File struct {
	Copilot Artifact `json:"github-copilot-installer"`
	Toolbox Artifact `json:"jetbrains-toolbox-installer"`
	Wowup   Artifact `json:"wowup-cf-installer"`
}

func Decode(data []byte) (File, error) {
	var f File
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&f); err != nil {
		return f, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return f, errors.New("expected one pins object")
	}
	if f.Copilot.Version == "" || f.Toolbox.Version == "" || f.Wowup.Version == "" {
		return f, errors.New("pins file must name every installer helper")
	}
	return f, nil
}

func Encode(f File) ([]byte, error) {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func Lookup(name string) ([]byte, error) {
	return LookupIn(packed, name)
}

func LookupIn(data []byte, name string) ([]byte, error) {
	f, err := Decode(data)
	if err != nil {
		return nil, err
	}
	a, err := f.artifact(name)
	if err != nil {
		return nil, err
	}
	return json.Marshal(a)
}

func Replace(data []byte, name string, artifact []byte) ([]byte, error) {
	f, err := Decode(data)
	if err != nil {
		return nil, err
	}
	var a Artifact
	d := json.NewDecoder(bytes.NewReader(artifact))
	d.DisallowUnknownFields()
	if err := d.Decode(&a); err != nil {
		return nil, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return nil, errors.New("expected one release object")
	}
	switch name {
	case Copilot:
		f.Copilot = a
	case Toolbox:
		f.Toolbox = a
	case Wowup:
		f.Wowup = a
	default:
		return nil, fmt.Errorf("unknown installer helper %q", name)
	}
	return Encode(f)
}

func (f File) artifact(name string) (Artifact, error) {
	switch name {
	case Copilot:
		return f.Copilot, nil
	case Toolbox:
		return f.Toolbox, nil
	case Wowup:
		return f.Wowup, nil
	default:
		return Artifact{}, fmt.Errorf("unknown installer helper %q", name)
	}
}
