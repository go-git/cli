package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadTreeRefusesNullSha(t *testing.T) {
	t.Parallel()
	repo := buildBogusTreeRepo(t)
	bogus := readBogusTreeHash(t, repo)

	if _, _, err := runGogit(t, repo, "read-tree", bogus); err == nil {
		t.Fatal("expected non-zero exit without GIT_ALLOW_NULL_SHA1")
	}
}

func TestReadTreeAllowsNullSha(t *testing.T) {
	t.Parallel()
	repo := buildBogusTreeRepo(t)
	bogus := readBogusTreeHash(t, repo)

	if _, _, err := runGogitEnv(t, repo, []string{"GIT_ALLOW_NULL_SHA1=1"}, "read-tree", bogus); err != nil {
		t.Fatalf("read-tree with GIT_ALLOW_NULL_SHA1: %v", err)
	}

	if _, err := os.Stat(filepath.Join(repo, ".git", "index")); err != nil {
		t.Fatalf("expected .git/index after read-tree: %v", err)
	}
}

func buildBogusTreeRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()

	if _, _, err := runGogit(t, repo, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	zero := strings.Repeat("0", 40)
	input := "160000 commit " + zero + "\tbroken\n"

	if _, _, err := runGogitStdin(t, repo, input, "mktree"); err != nil {
		t.Fatalf("mktree: %v", err)
	}

	return repo
}

func readBogusTreeHash(t *testing.T, repo string) string {
	t.Helper()

	zero := strings.Repeat("0", 40)
	input := "160000 commit " + zero + "\tbroken\n"

	stdout, _, err := runGogitStdin(t, repo, input, "mktree")
	if err != nil {
		t.Fatalf("mktree: %v", err)
	}

	return strings.TrimSpace(stdout)
}
