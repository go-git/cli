package main

import (
	"os"
	"path/filepath"
	"strings"
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

func TestAddWarnsBogusEnvVersion(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()

	if _, _, err := runGogit(t, repo, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repo, "f"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	env := append(os.Environ(), "GIT_INDEX_VERSION=2bogus")

	_, stderr, err := runGogitEnv(t, repo, env, "add", "f")
	if err != nil {
		t.Fatalf("add: %v\nstderr: %s", err, stderr)
	}

	wantPrefix := "warning: GIT_INDEX_VERSION set, but the value is invalid.\nUsing version 2\n"
	if !strings.HasPrefix(stderr, wantPrefix) {
		t.Fatalf("stderr = %q; want prefix %q", stderr, wantPrefix)
	}
}

func TestAddNoWarnWithExistingIndex(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()

	if _, _, err := runGogit(t, repo, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repo, "f"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := runGogit(t, repo, "add", "f"); err != nil {
		t.Fatalf("first add: %v", err)
	}

	env := append(os.Environ(), "GIT_INDEX_VERSION=1")

	_, stderr, err := runGogitEnv(t, repo, env, "add", "f")
	if err != nil {
		t.Fatalf("second add: %v", err)
	}

	if stderr != "" {
		t.Fatalf("expected empty stderr with existing index, got %q", stderr)
	}
}

func TestAddWarnsBogusConfigVersion(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()

	if _, _, err := runGogit(t, repo, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repo, "f"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := runGogit(t, repo, "config", "index.version", "5"); err != nil {
		t.Fatalf("config: %v", err)
	}

	_, stderr, err := runGogit(t, repo, "add", "f")
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	wantPrefix := "warning: index.version set, but the value is invalid.\nUsing version 2\n"
	if !strings.HasPrefix(stderr, wantPrefix) {
		t.Fatalf("stderr = %q; want prefix %q", stderr, wantPrefix)
	}
}
