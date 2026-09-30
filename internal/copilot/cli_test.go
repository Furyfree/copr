package copilot

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestParseKeepsCommandLine(t *testing.T) {
	for _, args := range [][]string{{"install"}, {"update", "-y"}, {"install", "--app-version", "1.1.23", "--assumeyes"}, {"status"}, {"uninstall", "--assumeyes"}, {"apply", "--appimage", "a", "--sha256", strings.Repeat("a", 64), "--app-version", "1.1.23"}} {
		if _, err := Parse(args); err != nil {
			t.Errorf("%v: %v", args, err)
		}
	}
	for _, args := range [][]string{{"apply", "--rpm", "a"}, {"status", "-y"}, {"install", "--app-version", "1.1"}, {"prepare"}, {"install", "update"}} {
		if _, err := Parse(args); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
}

func TestStatusTextAndUpdate(t *testing.T) {
	f := setup(t)
	selectFixtureRelease(f, "1.13.1")
	var out bytes.Buffer
	f.e.out = &out
	if err := f.e.Execute(t.Context(), Options{Command: "update"}); err == nil || !strings.Contains(err.Error(), "use install") {
		t.Fatalf("update without installation: %v", err)
	}
	if err := f.e.Execute(t.Context(), Options{Command: "status"}); err != nil || !strings.Contains(out.String(), "Selected GitHub Copilot: 1.13.1\nSelected SHA-256: "+f.digest+"\nInstalled GitHub Copilot: not installed\n") {
		t.Fatalf("absent status: %q %v", out.String(), err)
	}
	f.install(t, "1.13.1")
	out.Reset()
	if err := f.e.Execute(t.Context(), Options{Command: "status"}); err != nil || !strings.HasSuffix(out.String(), "Installed GitHub Copilot: 1.13.1\n") {
		t.Fatalf("installed status: %q %v", out.String(), err)
	}
	f.e.legacyRPM = func(context.Context) (bool, error) { return false, nil }
	if err := f.e.Execute(t.Context(), Options{Command: "uninstall"}); err != nil || exists(f.e.app) {
		t.Fatalf("uninstall: %v", err)
	}
}
