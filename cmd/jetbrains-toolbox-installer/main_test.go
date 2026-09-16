package main

import (
	"strings"
	"testing"
)

func TestMutationRequiresExplicitConfirmation(t *testing.T) {
	for _, command := range []string{"install", "update", "uninstall"} {
		if err := run(t.Context(), []string{command}); err == nil || !strings.Contains(err.Error(), "--assumeyes") {
			t.Fatalf("%s without confirmation accepted: %v", command, err)
		}
	}
	if err := run(t.Context(), []string{"apply", "--archive", "/nonexistent", "--sha256", strings.Repeat("a", 64), "--app-version", "3.8.0.87909"}); err == nil || !strings.Contains(err.Error(), "--assumeyes") {
		t.Fatalf("apply without confirmation accepted: %v", err)
	}
	if err := run(t.Context(), []string{"unknown"}); err == nil {
		t.Fatal("unknown command accepted")
	}
}
