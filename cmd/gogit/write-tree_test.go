package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteTreeFromIndex(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()

	if _, _, err := runGogit(t, repo, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repo, "a"), []byte("a-content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(repo, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(repo, "sub", "b"), []byte("b-content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := runGogit(t, repo, "add", "a", "sub/b"); err != nil {
		t.Fatalf("add: %v", err)
	}

	stdout, _, err := runGogit(t, repo, "write-tree")
	if err != nil {
		t.Fatalf("write-tree: %v", err)
	}

	treeOID := strings.TrimSpace(stdout)
	if len(treeOID) != 40 {
		t.Fatalf("expected 40-char hash, got %q", treeOID)
	}

	// Confirm the tree object is stored.
	if _, _, err := runGogit(t, repo, "cat-file", "tree", treeOID); err != nil {
		t.Fatalf("cat-file tree: %v", err)
	}
}

func TestWriteTreeRefusesNullSha(t *testing.T) {
	t.Parallel()
	repo := buildBogusTreeRepo(t)
	bogus := readBogusTreeHash(t, repo)

	env := []string{"GIT_ALLOW_NULL_SHA1=1"}
	if _, _, err := runGogitEnv(t, repo, env, "read-tree", bogus); err != nil {
		t.Fatalf("read-tree: %v", err)
	}

	if _, _, err := runGogit(t, repo, "write-tree"); err == nil {
		t.Fatal("expected write-tree to refuse null sha")
	}
}
