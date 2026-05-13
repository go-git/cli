package main

import (
	"os"
	"path/filepath"
	"testing"
)

func setupRepoWithCommit(t *testing.T) string {
	t.Helper()

	repo := t.TempDir()

	if _, _, err := runGogit(t, repo, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repo, "file0"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(repo, "dir1"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(repo, "dir1", "file1"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := runGogit(t, repo, "add", "file0", "dir1/file1"); err != nil {
		t.Fatalf("add: %v", err)
	}

	if _, stderr, err := runGogitEnv(t, repo, gitIdentityEnv(repo), "commit", "-m", "init"); err != nil {
		t.Fatalf("commit: %v\nstderr: %s", err, stderr)
	}

	return repo
}

func TestCheckoutRestoresFile(t *testing.T) {
	t.Parallel()

	repo := setupRepoWithCommit(t)

	if err := os.Remove(filepath.Join(repo, "file0")); err != nil {
		t.Fatal(err)
	}

	if _, stderr, err := runGogit(t, repo, "checkout", "HEAD", "--", "file0"); err != nil {
		t.Fatalf("checkout: %v\nstderr: %s", err, stderr)
	}

	got, err := os.ReadFile(filepath.Join(repo, "file0"))
	if err != nil {
		t.Fatalf("file0 not restored: %v", err)
	}

	if string(got) != "base\n" {
		t.Fatalf("file0 content = %q want %q", got, "base\n")
	}
}

func TestCheckoutRestoresDirectory(t *testing.T) {
	t.Parallel()

	repo := setupRepoWithCommit(t)

	if err := os.Remove(filepath.Join(repo, "dir1", "file1")); err != nil {
		t.Fatal(err)
	}

	if _, stderr, err := runGogit(t, repo, "checkout", "HEAD", "--", "dir1"); err != nil {
		t.Fatalf("checkout: %v\nstderr: %s", err, stderr)
	}

	got, err := os.ReadFile(filepath.Join(repo, "dir1", "file1"))
	if err != nil {
		t.Fatalf("dir1/file1 not restored: %v", err)
	}

	if string(got) != "hello\n" {
		t.Fatalf("dir1/file1 content = %q want %q", got, "hello\n")
	}
}

func TestCheckoutEscapeFails(t *testing.T) {
	t.Parallel()

	repo := setupRepoWithCommit(t)

	if _, _, err := runGogit(t, repo, "checkout", "HEAD", "--", "../../etc/passwd"); err == nil {
		t.Fatal("expected non-zero exit for path outside worktree")
	}
}
