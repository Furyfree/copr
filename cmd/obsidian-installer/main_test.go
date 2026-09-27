package main

import (
	"strings"
	"testing"
)

func TestMutationRequiresConfirmation(t *testing.T) {
	for _, command := range []string{"install", "update", "apply", "uninstall"} {
		t.Run(command, func(t *testing.T) {
			if err := run(t.Context(), []string{command}); err == nil || !strings.Contains(err.Error(), "--assumeyes") {
				t.Fatalf("unconfirmed command: %v", err)
			}
		})
	}
}
