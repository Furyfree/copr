package packaging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const coprURL = "https://copr.fedorainfracloud.org"

type account struct{ owner, login, token string }

func parseAccount(owner, config string) (account, error) {
	a := account{owner: owner}
	if !regexp.MustCompile(`^[a-z][a-z0-9_-]*$`).MatchString(owner) {
		return a, errors.New("COPR_OWNER must be the Fedora account name")
	}
	section := ""
	values := map[string]string{}
	for line := range strings.SplitSeq(config, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = line[1 : len(line)-1]
			continue
		}
		if section != "copr-cli" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return a, errors.New("malformed COPR configuration")
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if _, duplicate := values[key]; duplicate {
			return a, errors.New("duplicate COPR setting")
		}
		values[key] = value
	}
	if values["username"] != owner || strings.TrimRight(values["copr_url"], "/") != coprURL || values["login"] == "" || values["token"] == "" {
		return a, errors.New("COPR_CONFIG must select the matching official Fedora COPR account")
	}
	a.login, a.token = values["login"], values["token"]
	return a, nil
}

type Publisher struct {
	Root   string
	Client *http.Client
	// Submit waits for the native build and streams its public progress.
	Submit func(context.Context, []string, []string) error
	Run    Runner
}

func NewPublisher(root string) *Publisher {
	return &Publisher{Root: root, Run: Run, Client: &http.Client{Timeout: 120 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, Submit: func(ctx context.Context, args, env []string) error {
		cmd := exec.CommandContext(ctx, "copr-cli", args...)
		cmd.Env, cmd.Stdout, cmd.Stderr = env, os.Stdout, os.Stderr
		return cmd.Run()
	}}
}

func (p *Publisher) request(ctx context.Context, method, path string, a account, body any, destination any) error {
	var input io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		input = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, coprURL+"/api_3"+path, input)
	if err != nil {
		return errors.New("cannot create COPR request")
	}
	if method == http.MethodPost {
		req.SetBasicAuth(a.login, a.token)
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := p.Client.Do(req)
	if err != nil {
		return errors.New("COPR request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("COPR returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024+1))
	if err != nil || len(data) > 2*1024*1024 {
		return errors.New("invalid COPR response")
	}
	var failure struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(data, &failure) != nil || len(failure.Error) > 0 {
		return errors.New("COPR operation rejected")
	}
	if destination != nil && json.Unmarshal(data, destination) != nil {
		return errors.New("invalid COPR project response")
	}
	return nil
}

func (p *Publisher) Publish(ctx context.Context, action, name, srpm string) error {
	if action != "project" && action != "build" {
		return errors.New("unknown COPR operation")
	}
	c, err := Load(p.Root)
	if err != nil {
		return err
	}
	project, ok := c.Projects[name]
	if !ok {
		return errors.New("select a configured COPR project")
	}
	if action == "project" && srpm != "" {
		return errors.New("project operation does not accept an SRPM")
	}
	a, err := parseAccount(os.Getenv("COPR_OWNER"), os.Getenv("COPR_CONFIG"))
	if err != nil {
		return err
	}
	env := CleanEnvironment()
	if action == "build" {
		if !strings.HasSuffix(srpm, ".src.rpm") {
			return errors.New("build requires a source RPM")
		}
		if err := Regular(srpm); err != nil {
			return err
		}
		srpm, err = filepath.Abs(srpm)
		if err != nil {
			return err
		}
		identity, err := p.Run(ctx, "rpm", []string{"-qp", "--qf", "%{NAME}\n%{SOURCEPACKAGE}\n", srpm}, "", env)
		if err != nil {
			return errors.New("cannot verify SRPM identity")
		}
		if string(identity) != name+"\n1\n" {
			return errors.New("SRPM identity does not match selected package")
		}
	}
	request := map[string]any{"name": project.Name, "chroots": project.Chroots, "description": project.Description, "homepage": project.Homepage, "instructions": "sudo dnf copr enable " + a.owner + "/" + project.Name, "enable_net": false, "devel_mode": false, "unlisted_on_hp": false}
	if err := p.request(ctx, "POST", "/project/add/"+a.owner+"?exist_ok=true", a, request, nil); err != nil {
		return err
	}
	var existing struct {
		ChrootRepos map[string]json.RawMessage `json:"chroot_repos"`
	}
	query := url.Values{"ownername": {a.owner}, "projectname": {project.Name}}
	if err := p.request(ctx, "GET", "/project?"+query.Encode(), a, nil, &existing); err != nil {
		return err
	}
	for _, chroot := range project.Chroots {
		if _, ok := existing.ChrootRepos[chroot]; !ok {
			return errors.New("existing project lacks configured chroots")
		}
	}
	fmt.Printf("COPR project ready: %s/%s\n", a.owner, project.Name)
	if action == "project" {
		return nil
	}
	root, err := os.MkdirTemp("", "copr-publish-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	config := filepath.Join(root, "config")
	// Rewrite only validated fields; unrelated settings cannot alter native routing.
	data := fmt.Sprintf("[copr-cli]\nusername = %s\nlogin = %s\ntoken = %s\ncopr_url = %s\n", a.owner, a.login, a.token, coprURL)
	if err := os.WriteFile(config, []byte(data), 0o600); err != nil {
		return err
	}
	args := []string{"--config", config, "build", "--enable-net", "off"}
	for _, chroot := range project.Chroots {
		args = append(args, "--chroot", chroot)
	}
	args = append(args, a.owner+"/"+project.Name, srpm)
	if err := p.Submit(ctx, args, env); err != nil {
		return errors.New("COPR build failed; see native build output")
	}
	return nil
}
