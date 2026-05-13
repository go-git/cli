package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const baseBlobHash = "df967b96a579e45a18b8251732d16804b2e56a55"

func TestHashObjectStdinBlob(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()

	if _, _, err := runGogit(t, repo, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	stdout, _, err := runGogitStdin(t, repo, "base\n", "hash-object", "-w", "--stdin")
	if err != nil {
		t.Fatalf("hash-object: %v", err)
	}

	got := strings.TrimSpace(stdout)
	if got != baseBlobHash {
		t.Fatalf("hash = %q want %q", got, baseBlobHash)
	}

	objPath := filepath.Join(repo, ".git", "objects", baseBlobHash[:2], baseBlobHash[2:])
	if _, err := os.Stat(objPath); err != nil {
		t.Fatalf("loose object not written: %v", err)
	}
}

func TestHashObjectLiterallyAcceptsMalformedTag(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()

	if _, _, err := runGogit(t, repo, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	_, _, err := runGogitStdin(t, repo, "this is not a tag\n",
		"hash-object", "-t", "tag", "-w", "--stdin", "--literally")
	if err != nil {
		t.Fatalf("hash-object --literally: %v", err)
	}
}
