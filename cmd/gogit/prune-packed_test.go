package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrunePackedRemovesLooseCopies(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()

	if _, _, err := runGogit(t, repo, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	stdout, _, err := runGogitStdin(t, repo, "base\n", "hash-object", "-w", "--stdin")
	if err != nil {
		t.Fatalf("hash-object: %v", err)
	}

	blobOID := strings.TrimSpace(stdout)
	loosePath := filepath.Join(repo, ".git", "objects", blobOID[:2], blobOID[2:])

	if _, err := os.Stat(loosePath); err != nil {
		t.Fatalf("expected loose object at %s: %v", loosePath, err)
	}

	if _, _, err := runGogitStdin(t, repo, blobOID+"\n",
		"pack-objects", filepath.Join(repo, ".git", "objects", "pack", "pack")); err != nil {
		t.Fatalf("pack-objects: %v", err)
	}

	if _, _, err := runGogit(t, repo, "prune-packed"); err != nil {
		t.Fatalf("prune-packed: %v", err)
	}

	if _, err := os.Stat(loosePath); !os.IsNotExist(err) {
		t.Fatalf("expected loose object removed, stat err = %v", err)
	}

	if _, _, err := runGogit(t, repo, "cat-file", "-e", blobOID); err != nil {
		t.Fatalf("object should still be reachable via pack: %v", err)
	}
}
