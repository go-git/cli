package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInit(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	repo := filepath.Join(tmp, "repo")

	if _, _, err := runGogit(t, tmp, "init", repo); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	info, err := os.Stat(filepath.Join(repo, ".git"))
	if err != nil {
		t.Fatalf("expected .git dir: %v", err)
	}

	if !info.IsDir() {
		t.Fatal(".git is not a directory")
	}
}

func TestInitTemplateFlagIgnored(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	repo := filepath.Join(tmp, "repo")

	if _, _, err := runGogit(t, tmp, "init", "--template=", repo); err != nil {
		t.Fatalf("init --template= failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(repo, ".git")); err != nil {
		t.Fatalf("expected .git dir: %v", err)
	}
}
