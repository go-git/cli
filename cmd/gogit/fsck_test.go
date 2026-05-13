package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFsckCleanRepo(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()

	if _, _, err := runGogit(t, repo, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repo, "f"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := runGogit(t, repo, "add", "f"); err != nil {
		t.Fatalf("add: %v", err)
	}

	if _, _, err := runGogitEnv(t, repo, gitIdentityEnv(repo), "commit", "-m", "x"); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if _, _, err := runGogit(t, repo, "fsck"); err != nil {
		t.Fatalf("fsck on clean repo: %v", err)
	}
}

func TestFsckDetectsCorruptedLooseObject(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()

	if _, _, err := runGogit(t, repo, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repo, "f"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := runGogit(t, repo, "add", "f"); err != nil {
		t.Fatalf("add: %v", err)
	}

	matches, _ := filepath.Glob(filepath.Join(repo, ".git", "objects", "[0-9a-f][0-9a-f]", "*"))
	if len(matches) == 0 {
		t.Skip("no loose objects to corrupt")
	}

	if err := os.WriteFile(matches[0], []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := runGogit(t, repo, "fsck"); err == nil {
		t.Fatal("fsck should have failed on corrupted object")
	}
}
