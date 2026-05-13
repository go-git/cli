package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMktreeBuildsTree(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()

	if _, _, err := runGogit(t, repo, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repo, "f"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	hashStdout, _, err := runGogitStdin(t, repo, "hello\n", "hash-object", "-w", "--stdin")
	if err != nil {
		t.Fatalf("hash-object: %v", err)
	}

	blobOID := strings.TrimSpace(hashStdout)
	input := "100644 blob " + blobOID + "\tf\n"

	stdout, _, err := runGogitStdin(t, repo, input, "mktree")
	if err != nil {
		t.Fatalf("mktree: %v", err)
	}

	treeOID := strings.TrimSpace(stdout)
	if len(treeOID) != 40 {
		t.Fatalf("expected 40-char hash, got %q", treeOID)
	}

	if _, _, err := runGogit(t, repo, "cat-file", "tree", treeOID); err != nil {
		t.Fatalf("cat-file tree: %v", err)
	}
}

func TestMktreeAcceptsNullSha(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()

	if _, _, err := runGogit(t, repo, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	zero := strings.Repeat("0", 40)
	input := "160000 commit " + zero + "\tbroken\n"

	stdout, _, err := runGogitStdin(t, repo, input, "mktree")
	if err != nil {
		t.Fatalf("mktree (null sha): %v", err)
	}

	if len(strings.TrimSpace(stdout)) != 40 {
		t.Fatalf("expected 40-char hash, got %q", stdout)
	}
}
