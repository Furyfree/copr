package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPurgeRequiresExplicitConfirmation(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", directory)
	profile := filepath.Join(directory, "WowUpCf")
	if err := os.Mkdir(profile, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := run(t.Context(), []string{"purge"}); err == nil || !strings.Contains(err.Error(), "--assumeyes") {
		t.Fatalf("missing confirmation accepted: %v", err)
	}
	if _, err := os.Stat(profile); err != nil {
		t.Fatal("unconfirmed purge changed profile:", err)
	}
}
