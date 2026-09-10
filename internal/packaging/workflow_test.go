package packaging

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestWorkflowPackageRouting(t *testing.T) {
	root := repository(t)
	data, err := os.ReadFile(filepath.Join(root, ".github/workflows/copr.yml"))
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	config, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	choice := regexp.MustCompile(`(?s)      package:\n.*?options: \[([^]]+)\]`).FindStringSubmatch(workflow)
	if len(choice) != 2 {
		t.Fatal("missing package choices")
	}
	var names []string
	for name := range config.Projects {
		names = append(names, name)
	}
	slices.Sort(names)
	choices := strings.Split(choice[1], ", ")
	slices.Sort(choices)
	if !slices.Equal(names, choices) {
		t.Fatalf("workflow choices %v differ from projects %v", choices, names)
	}
	// Execute the actual publication shell with a recording Docker replacement.
	marker := "      - name: Create project or submit build\n"
	_, step, ok := strings.Cut(workflow, marker)
	if !ok {
		t.Fatal("missing publication step")
	}
	_, body, ok := strings.Cut(step, "        run: |\n")
	if !ok {
		t.Fatal("missing publication script")
	}
	var lines []string
	for line := range strings.SplitSeq(body, "\n") {
		lines = append(lines, strings.TrimPrefix(line, "          "))
	}
	script := strings.Join(lines, "\n")
	for _, name := range names {
		for _, operation := range []string{"project", "build", "invalid"} {
			t.Run(name+"/"+operation, func(t *testing.T) {
				temp := t.TempDir()
				bin := filepath.Join(temp, "bin")
				write(t, filepath.Join(bin, "docker"), "#!/bin/sh\nprintf '%s\\n' \"$@\"\n")
				if err := os.Chmod(filepath.Join(bin, "docker"), 0o755); err != nil {
					t.Fatal(err)
				}
				write(t, filepath.Join(temp, "copr-srpm", name+".src.rpm"), "fixture")
				command := func() ([]byte, error) {
					cmd := exec.CommandContext(t.Context(), "bash", "-euo", "pipefail", "-c", script)
					cmd.Env = append(CleanEnvironment(), "PATH="+bin+":"+os.Getenv("PATH"), "RUNNER_TEMP="+temp, "GITHUB_WORKSPACE="+root, "PACKAGE="+name, "OPERATION="+operation)
					return cmd.CombinedOutput()
				}
				output, err := command()
				if operation == "invalid" {
					if err == nil || len(output) != 0 {
						t.Fatalf("invalid action reached Docker: %s %v", output, err)
					}
					return
				}
				if err != nil {
					t.Fatalf("publication script: %s %v", output, err)
				}
				want := "project\n" + name + "\n"
				if operation == "build" {
					want = "publish\n" + name + "\n/input/" + name + ".src.rpm\n"
				}
				if !strings.HasSuffix(string(output), want) {
					t.Fatalf("wrong routing: %s", output)
				}
				if operation == "build" {
					write(t, filepath.Join(temp, "copr-srpm/extra.src.rpm"), "fixture")
					if output, err := command(); err == nil || len(output) != 0 {
						t.Fatalf("ambiguous artifacts submitted: %s %v", output, err)
					}
				}
			})
		}
	}
}

func TestWorkflowCheckRouting(t *testing.T) {
	root := repository(t)
	data, err := os.ReadFile(filepath.Join(root, ".github/workflows/copr.yml"))
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	for _, tc := range []struct{ step, condition, suffix string }{
		{"Run the complete offline gate", "github.event_name != 'workflow_dispatch'", "just\ncheck\n"},
		{"Run selected package checks", "github.event_name == 'workflow_dispatch'", "just\ncheck-package\nvoxtype\n"},
	} {
		t.Run(tc.step, func(t *testing.T) {
			_, step, ok := strings.Cut(workflow, "      - name: "+tc.step+"\n")
			if !ok {
				t.Fatal("missing check step")
			}
			step, _, _ = strings.Cut(step, "\n      - ")
			step, _, _ = strings.Cut(step, "\n  publish:")
			if !strings.HasPrefix(step, "        if: "+tc.condition+"\n") {
				t.Fatal("check runs for the wrong event")
			}
			_, script, ok := strings.Cut(step, "        run: |\n")
			if !ok {
				t.Fatal("missing check script")
			}
			bin := t.TempDir()
			write(t, filepath.Join(bin, "docker"), "#!/bin/sh\nprintf '%s\\n' \"$@\"\n")
			if err := os.Chmod(filepath.Join(bin, "docker"), 0o755); err != nil {
				t.Fatal(err)
			}
			cmd := exec.CommandContext(t.Context(), "bash", "-euo", "pipefail", "-c", script)
			cmd.Env = append(CleanEnvironment(), "PATH="+bin+":"+os.Getenv("PATH"), "GITHUB_WORKSPACE="+root, "PACKAGE=voxtype")
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("check script: %s %v", output, err)
			}
			if !strings.HasSuffix(string(output), tc.suffix) || !strings.Contains(string(output), "--network\nnone\n") || !strings.Contains(string(output), root+":/work:ro\n") {
				t.Fatalf("wrong check invocation: %s", output)
			}
		})
	}
}

func TestWorkflowSelectsLatestCopilotOnly(t *testing.T) {
	root := repository(t)
	data, err := os.ReadFile(filepath.Join(root, ".github/workflows/copr.yml"))
	if err != nil {
		t.Fatal(err)
	}
	_, step, ok := strings.Cut(string(data), "      - name: Prepare selected source RPM\n")
	if !ok {
		t.Fatal("missing source preparation step")
	}
	step, _, _ = strings.Cut(step, "\n      - ")
	_, script, ok := strings.Cut(step, "        run: |\n")
	if !ok {
		t.Fatal("missing preparation script")
	}
	for _, name := range []string{"nimbus", "voxtype", "github-copilot-installer", "wowup-cf-installer"} {
		t.Run(name, func(t *testing.T) {
			temp := t.TempDir()
			bin := filepath.Join(temp, "bin")
			write(t, filepath.Join(bin, "docker"), "#!/bin/sh\nprintf '%s\\n' \"$@\"\n")
			if err := os.Chmod(filepath.Join(bin, "docker"), 0o755); err != nil {
				t.Fatal(err)
			}
			cmd := exec.CommandContext(t.Context(), "bash", "-euo", "pipefail", "-c", script)
			cmd.Env = append(CleanEnvironment(), "PATH="+bin+":"+os.Getenv("PATH"), "RUNNER_TEMP="+temp, "GITHUB_WORKSPACE="+root, "PACKAGE="+name)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("preparation failed: %v %s", err, out)
			}
			want := "prepare\n" + name + "\n/output\n"
			if name == "github-copilot-installer" {
				want += "--latest\n"
			}
			if !strings.HasSuffix(string(out), want) {
				t.Fatalf("wrong release selection: %s", out)
			}
		})
	}
}
