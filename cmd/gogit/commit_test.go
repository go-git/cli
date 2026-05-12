package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCommitWithEnvIdentity(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()

	if _, _, err := runGogit(t, repo, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repo, "file0"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := runGogit(t, repo, "add", "file0"); err != nil {
		t.Fatalf("add: %v", err)
	}

	if _, stderr, err := runGogitEnv(t, repo, gitIdentityEnv(repo), "commit", "-m", "populate tree"); err != nil {
		t.Fatalf("commit failed: %v\nstderr: %s", err, stderr)
	}
}
