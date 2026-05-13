package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateIndexShowsVersion2(t *testing.T) {
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

	stdout, _, err := runGogit(t, repo, "update-index", "--show-index-version")
	if err != nil {
		t.Fatalf("update-index: %v", err)
	}

	if strings.TrimSpace(stdout) != "2" {
		t.Fatalf("stdout = %q; want \"2\"", stdout)
	}
}

func TestUpdateIndexShowsVersion4WithManyFiles(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()

	if _, _, err := runGogit(t, repo, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	if _, _, err := runGogit(t, repo, "config", "feature.manyFiles", "true"); err != nil {
		t.Fatalf("config: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repo, "f"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := runGogit(t, repo, "add", "f"); err != nil {
		t.Fatalf("add: %v", err)
	}

	stdout, _, err := runGogit(t, repo, "update-index", "--show-index-version")
	if err != nil {
		t.Fatalf("update-index: %v", err)
	}

	if strings.TrimSpace(stdout) != "4" {
		t.Fatalf("stdout = %q; want \"4\"", stdout)
	}
}
