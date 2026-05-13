package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAddSingleFile(t *testing.T) {
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
}

func TestAddMultiplePaths(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()

	if _, _, err := runGogit(t, repo, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(repo, "dir1"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(repo, "file0"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(repo, "dir1", "file1"), []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := runGogit(t, repo, "add", "file0", "dir1/file1"); err != nil {
		t.Fatalf("add: %v", err)
	}
}
