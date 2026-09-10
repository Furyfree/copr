package packaging

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func repository(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}
func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
func TestSourceValidationPrecedesTools(t *testing.T) {
	for _, source := range []string{"http://example.invalid/a", "https://user:password@example.invalid/a", "../secret", "/secret", "linked", "missing"} {
		t.Run(source, func(t *testing.T) {
			root := t.TempDir()
			spec := filepath.Join(root, "package/test.spec")
			write(t, spec, "fixture")
			os.Symlink(spec, filepath.Join(root, "package/linked"))
			calls := 0
			b := New(repository(t))
			b.Run = func(context.Context, string, []string, string, []string) ([]byte, error) {
				calls++
				return []byte("Source0: " + source + "\n"), nil
			}
			if _, err := b.SRPM(t.Context(), spec, filepath.Join(root, "out")); err == nil {
				t.Fatal("unsafe source accepted")
			}
			if calls != 1 {
				t.Fatalf("called tools %d times", calls)
			}
		})
	}
	root := t.TempDir()
	spec := filepath.Join(root, "test.spec")
	write(t, spec, "fixture")
	b := New(repository(t))
	b.Run = func(context.Context, string, []string, string, []string) ([]byte, error) {
		t.Fatal("tool called for invalid output")
		return nil, nil
	}
	if _, err := b.SRPM(t.Context(), spec, filepath.Join(root, "out")); err == nil {
		t.Fatal("in-source output accepted")
	}
}
func TestSourceSymlinkOutputRejected(t *testing.T) {
	root := t.TempDir()
	pkg := filepath.Join(root, "package")
	spec := filepath.Join(pkg, "test.spec")
	write(t, spec, "fixture")
	os.Symlink(pkg, filepath.Join(root, "linked"))
	b := New(repository(t))
	b.Run = func(context.Context, string, []string, string, []string) ([]byte, error) {
		t.Fatal("native tool called")
		return nil, nil
	}
	if _, err := b.SRPM(t.Context(), spec, filepath.Join(root, "linked/output")); err == nil {
		t.Fatal("symlink output accepted")
	}
}
func TestVoxtypeVerification(t *testing.T) {
	for _, kind := range []string{"valid", "checksum", "signer", "missing", "bad-signature"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "source")
			write(t, source, "reviewed source")
			digest, _ := fileDigest(source)
			calls := 0
			b := New(repository(t))
			b.Run = func(_ context.Context, _ string, args []string, _ string, _ []string) ([]byte, error) {
				calls++
				if slices.Contains(args, "--verify") {
					if kind == "bad-signature" {
						return nil, errors.New("signature rejected")
					}
					signer := voxtypeFingerprint
					if kind == "signer" {
						signer = strings.Repeat("A", 40)
					}
					if kind == "missing" {
						return nil, nil
					}
					return []byte("[GNUPG:] VALIDSIG " + signer + " timestamp\n"), nil
				}
				return nil, nil
			}
			if kind == "checksum" {
				digest = strings.Repeat("0", 64)
			}
			err := b.verifyVoxtype(t.Context(), source, "sig", "key", filepath.Join(root, "gpg"), digest)
			if (err == nil) != (kind == "valid") {
				t.Fatalf("verification: %v", err)
			}
			if kind == "checksum" && calls != 0 {
				t.Fatal("checksum failure invoked GPG")
			}
		})
	}
}
func TestArchiveBoundariesAndDeterminism(t *testing.T) {
	root := t.TempDir()
	tree := filepath.Join(root, "tree")
	write(t, filepath.Join(tree, "file"), "payload")
	one, two := filepath.Join(root, "one.gz"), filepath.Join(root, "two.gz")
	if err := bundle(tree, one, "fixture-1"); err != nil {
		t.Fatal(err)
	}
	if err := bundle(tree, two, "fixture-1"); err != nil {
		t.Fatal(err)
	}
	a, _ := os.ReadFile(one)
	b, _ := os.ReadFile(two)
	if !bytes.Equal(a, b) {
		t.Fatal("source bundle is nondeterministic")
	}
	for _, kind := range []string{"traversal", "absolute", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(root, kind+".gz")
			f, _ := os.Create(path)
			g := gzip.NewWriter(f)
			ar := tar.NewWriter(g)
			h := &tar.Header{Name: "../escape", Mode: 0o644, Size: 1}
			if kind == "absolute" {
				h.Name = "/escape"
			}
			if kind == "symlink" {
				h.Name = "link"
				h.Typeflag = tar.TypeSymlink
				h.Linkname = "../escape"
				h.Size = 0
			}
			if err := ar.WriteHeader(h); err != nil {
				t.Fatal(err)
			}
			if h.Size > 0 {
				ar.Write([]byte("x"))
			}
			ar.Close()
			g.Close()
			f.Close()
			if err := extractSource(path, t.TempDir()); err == nil {
				t.Fatal("unsafe extraction accepted")
			}
		})
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func publication(t *testing.T) (*Publisher, *int, *[]string) {
	t.Helper()
	t.Setenv("COPR_OWNER", "owner")
	t.Setenv("COPR_CONFIG", "[copr-cli]\nusername=owner\nlogin=fixture-login\ntoken=fixture-token\ncopr_url="+coprURL+"\n")
	p := NewPublisher(repository(t))
	calls := new(0)
	paths := new([]string)
	p.Client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		*calls++
		if r.URL.Host != "copr.fedorainfracloud.org" {
			t.Fatal("foreign COPR host")
		}
		if r.Method == "POST" {
			login, token, ok := r.BasicAuth()
			if !ok || login != "fixture-login" || token != "fixture-token" {
				t.Fatal("bad authentication")
			}
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["enable_net"] != false || body["devel_mode"] != false {
				t.Fatal("unsafe project mutation")
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`)), Header: http.Header{}}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"chroot_repos":{"fedora-44-x86_64":[]}}`)), Header: http.Header{}}, nil
	})
	p.Submit = func(_ context.Context, args, env []string) error {
		for _, item := range env {
			if strings.HasPrefix(item, "COPR_CONFIG=") {
				t.Fatal("credentials in subprocess environment")
			}
		}
		if slices.Contains(args, "--nowait") || strings.Contains(strings.Join(args, " "), "fixture-token") {
			t.Fatal("unsafe build invocation")
		}
		path := args[1]
		*paths = append(*paths, path)
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatal("unsafe credential file")
		}
		if args[slices.Index(args, "--enable-net")+1] != "off" {
			t.Fatal("network enabled")
		}
		return nil
	}
	return p, calls, paths
}

func TestProjectCreationAllowsExistingProjects(t *testing.T) {
	for _, state := range []string{"new", "existing"} {
		t.Run(state, func(t *testing.T) {
			p, _, paths := publication(t)
			exists := state == "existing"
			native := p.Client.Transport
			p.Client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
				if r.Method == http.MethodPost {
					// Match COPR's case-sensitive exist_ok handling, including HTTP 400.
					// frontend/coprs_frontend/coprs/views/apiv3_ns/apiv3_projects.py
					if exists && r.URL.Query().Get("exist_ok") != "True" {
						return &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader(`{"error":"You already have a project with this name"}`)), Header: http.Header{}}, nil
					}
					exists = true
				}
				return native.RoundTrip(r)
			})
			for range 2 {
				if err := p.Publish(t.Context(), "project", "nimbus", ""); err != nil {
					t.Fatal(err)
				}
			}
			if len(*paths) != 0 {
				t.Fatal("project setup submitted a build")
			}
		})
	}
}

func TestPublicationRoutesAndCleansCredentials(t *testing.T) {
	c, err := Load(repository(t))
	if err != nil {
		t.Fatal(err)
	}
	for name := range c.Projects {
		t.Run(name, func(t *testing.T) {
			p, calls, paths := publication(t)
			path := filepath.Join(t.TempDir(), "candidate.src.rpm")
			write(t, path, "fixture")
			p.Run = func(_ context.Context, nameTool string, args []string, _ string, env []string) ([]byte, error) {
				if nameTool != "rpm" {
					t.Fatal(nameTool)
				}
				return []byte(name + "\n1\n"), nil
			}
			submit := p.Submit
			p.Submit = func(ctx context.Context, args, env []string) error {
				if args[len(args)-2] != "owner/"+name {
					t.Fatal("wrong project")
				}
				return submit(ctx, args, env)
			}
			if err := p.Publish(t.Context(), "build", name, path); err != nil {
				t.Fatal(err)
			}
			if *calls != 2 || len(*paths) != 1 {
				t.Fatal("wrong publication flow")
			}
			if _, err := os.Stat((*paths)[0]); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("credentials left behind")
			}
		})
	}
}
func TestInvalidPublicationDoesNotMutate(t *testing.T) {
	p, calls, _ := publication(t)
	for _, args := range [][3]string{{"delete", "nimbus", ""}, {"project", "../nimbus", ""}, {"project", "nimbus", "extra.src.rpm"}} {
		if err := p.Publish(t.Context(), args[0], args[1], args[2]); err == nil {
			t.Fatal("invalid operation accepted")
		}
	}
	path := filepath.Join(t.TempDir(), "candidate.src.rpm")
	write(t, path, "fixture")
	p.Run = func(context.Context, string, []string, string, []string) ([]byte, error) {
		return []byte("wrong\n1\n"), nil
	}
	if err := p.Publish(t.Context(), "build", "nimbus", path); err == nil {
		t.Fatal("wrong SRPM accepted")
	}
	t.Setenv("COPR_OWNER", "another")
	if err := p.Publish(t.Context(), "project", "nimbus", ""); err == nil {
		t.Fatal("wrong account accepted")
	}
	if *calls != 0 {
		t.Fatal("invalid request mutated project")
	}
}
func TestPublicationFailures(t *testing.T) {
	for _, kind := range []string{"chroot", "build"} {
		t.Run(kind, func(t *testing.T) {
			p, _, paths := publication(t)
			path := filepath.Join(t.TempDir(), "candidate.src.rpm")
			write(t, path, "fixture")
			p.Run = func(context.Context, string, []string, string, []string) ([]byte, error) {
				return []byte("nimbus\n1\n"), nil
			}
			if kind == "chroot" {
				native := p.Client.Transport
				p.Client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
					if r.Method == "GET" {
						return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"chroot_repos":{}}`)), Header: http.Header{}}, nil
					}
					return native.RoundTrip(r)
				})
			} else {
				native := p.Submit
				p.Submit = func(ctx context.Context, args, env []string) error {
					native(ctx, args, env)
					return errors.New("build failure")
				}
			}
			if err := p.Publish(t.Context(), "build", "nimbus", path); err == nil {
				t.Fatal("failure lost")
			}
			for _, path := range *paths {
				if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("credentials remain")
				}
			}
		})
	}
}

func TestGitHubArchiveMetadata(t *testing.T) {
	var data bytes.Buffer
	z := gzip.NewWriter(&data)
	ar := tar.NewWriter(z)
	for _, h := range []*tar.Header{
		{Name: "pax_global_header", Typeflag: tar.TypeXGlobalHeader, PAXRecords: map[string]string{"comment": "reviewed-commit"}},
		{Name: "source/README", Typeflag: tar.TypeReg, Mode: 0o644, Size: 7},
	} {
		if err := ar.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ar.Write([]byte("payload")); err != nil {
		t.Fatal(err)
	}
	if err := ar.Close(); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	archive := filepath.Join(root, "github.tar.gz")
	write(t, archive, data.String())
	out := filepath.Join(root, "out")
	if err := os.Mkdir(out, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := extractSource(archive, out); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(out, "source/README"))
	if err != nil || string(content) != "payload" {
		t.Fatalf("extracted content: %q %v", content, err)
	}
	if _, err := os.Stat(filepath.Join(out, "pax_global_header")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("metadata was materialized")
	}
}
