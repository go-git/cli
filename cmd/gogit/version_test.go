package main

import (
	"strings"
	"testing"
)

func TestVersionBuildOptions(t *testing.T) {
	t.Parallel()

	stdout, _, err := runGogit(t, t.TempDir(), "version", "--build-options")
	if err != nil {
		t.Fatalf("version --build-options failed: %v", err)
	}

	if !strings.Contains(stdout, "default-hash: sha1") {
		t.Errorf("expected output to contain 'default-hash: sha1', got:\n%s", stdout)
	}
}
