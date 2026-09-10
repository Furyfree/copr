package packaging

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const voxtypeFingerprint = "9CCF7915B750CAE8B095ED1AA3FC9F33FD209279"
const voxtypeSHA256 = "a4d0a256167f58ce90153077da82620794422f5172c918625d480ff9ffca625e"

var errOutput = errors.New("outdir must be outside the package source")

func fileDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func fetch(ctx context.Context, url, destination string) error {
	client := &http.Client{Timeout: 120 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 10 || r.URL.Scheme != "https" || r.URL.User != nil {
			return errors.New("unsafe source redirect")
		}
		return nil
	}}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("source download returned HTTP %d", resp.StatusCode)
	}
	f, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, resp.Body)
	return errors.Join(copyErr, f.Close())
}

func (b *Builder) verifyVoxtype(ctx context.Context, source, signature, key, home, digest string) error {
	actual, err := fileDigest(source)
	if err != nil {
		return err
	}
	if actual != digest {
		return errors.New("upstream source checksum differs from reviewed release")
	}
	if err := os.Mkdir(home, 0o700); err != nil {
		return err
	}
	if _, err := b.command(ctx, "", "gpg", "--batch", "--homedir", home, "--import", key); err != nil {
		return err
	}
	result, err := b.command(ctx, "", "gpg", "--batch", "--homedir", home, "--status-fd", "1", "--verify", signature, source)
	if err != nil {
		return err
	}
	var signers []string
	for line := range strings.SplitSeq(string(result), "\n") {
		if strings.HasPrefix(line, "[GNUPG:] VALIDSIG ") {
			fields := strings.Fields(line)
			if len(fields) > 2 {
				signers = append(signers, fields[2])
			}
		}
	}
	if len(signers) != 1 || signers[0] != voxtypeFingerprint {
		return errors.New("source signature does not match pinned signer")
	}
	return nil
}

func (b *Builder) prepareVoxtype(ctx context.Context, out string) (string, error) {
	packageDir := filepath.Join(b.Root, "packages/voxtype")
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
	spec, err := os.ReadFile(filepath.Join(packageDir, "voxtype.spec"))
	if err != nil {
		return "", err
	}
	version, err := specVersion(spec)
	if err != nil {
		return "", err
	}
	root, err := os.MkdirTemp("", "voxtype-source-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(root)
	source, signature := filepath.Join(root, "upstream.tar.gz"), filepath.Join(root, "upstream.tar.gz.asc")
	for path, url := range map[string]string{
		source:    "https://github.com/peteonrails/voxtype/archive/refs/tags/v" + version + ".tar.gz",
		signature: "https://github.com/peteonrails/voxtype/releases/download/v" + version + "/voxtype-" + version + ".tar.gz.asc",
	} {
		if err := fetch(ctx, url, path); err != nil {
			return "", err
		}
	}
	if err := b.verifyVoxtype(ctx, source, signature, filepath.Join(packageDir, "files/signing.asc"), filepath.Join(root, "gnupg"), voxtypeSHA256); err != nil {
		return "", err
	}
	if err := extractSource(source, root); err != nil {
		return "", err
	}
	tree := filepath.Join(root, "voxtype-"+version)
	lockPath := filepath.Join(tree, "Cargo.lock")
	lock, err := os.ReadFile(lockPath)
	if err != nil {
		return "", err
	}
	config, err := b.Run(ctx, "cargo", []string{"vendor", "--locked", "vendor"}, tree, append(CleanEnvironment(), "CARGO_HOME="+filepath.Join(root, "cargo-home")))
	if err != nil {
		return "", err
	}
	after, err := os.ReadFile(lockPath)
	if err != nil {
		return "", err
	}
	if !bytes.Equal(lock, after) {
		return "", errors.New("vendoring changed Cargo.lock")
	}
	if err := os.MkdirAll(filepath.Join(tree, ".cargo"), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(tree, ".cargo/config.toml"), config, 0o644); err != nil {
		return "", err
	}
	unit := filepath.Join(tree, "packaging/systemd/voxtype.service")
	data, err := os.ReadFile(unit)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(unit, bytes.ReplaceAll(data, []byte("Wants=ydotool.service\n"), nil), 0o644); err != nil {
		return "", err
	}
	prepared := filepath.Join(root, "recipe")
	if err := os.Mkdir(prepared, 0o755); err != nil {
		return "", err
	}
	archive := filepath.Join(prepared, "voxtype-"+version+"-vendor.tar.gz")
	if err := bundle(tree, archive, "voxtype-"+version); err != nil {
		return "", err
	}
	digest, err := fileDigest(archive)
	if err != nil {
		return "", err
	}
	recipe := filepath.Join(prepared, "voxtype.spec")
	if bytes.Count(spec, []byte("%global source_sha256 UNPREPARED")) != 1 {
		return "", errors.New("expected unprepared Voxtype spec")
	}
	spec = bytes.ReplaceAll(spec, []byte("%global source_sha256 UNPREPARED"), []byte("%global source_sha256 "+digest))
	if err := os.WriteFile(recipe, spec, 0o644); err != nil {
		return "", err
	}
	return b.SRPM(ctx, recipe, output)
}
