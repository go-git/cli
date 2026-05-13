package main

import (
	"crypto/sha1"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupRepoWithBlob(t *testing.T, content string) string {
	t.Helper()

	repo := t.TempDir()

	if _, _, err := runGogit(t, repo, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	if _, _, err := runGogitStdin(t, repo, content, "hash-object", "-w", "--stdin"); err != nil {
		t.Fatalf("hash-object: %v", err)
	}

	return repo
}

func TestPackObjectsFromStdin(t *testing.T) {
	t.Parallel()
	repo := setupRepoWithBlob(t, "base\n")

	stdout, _, err := runGogitStdin(t, repo, baseBlobHash+"\n", "pack-objects", filepath.Join(repo, "test-1"))
	if err != nil {
		t.Fatalf("pack-objects: %v", err)
	}

	packSHA := strings.TrimSpace(stdout)
	if len(packSHA) != 2*sha1.Size {
		t.Fatalf("pack sha length = %d want %d", len(packSHA), 2*sha1.Size)
	}

	for _, ext := range []string{".pack", ".idx"} {
		if _, err := os.Stat(filepath.Join(repo, "test-1-"+packSHA+ext)); err != nil {
			t.Fatalf("missing %s: %v", ext, err)
		}
	}
}

func TestPackObjectsIndexVersion1(t *testing.T) {
	t.Parallel()
	repo := setupRepoWithBlob(t, "base\n")

	stdout, _, err := runGogitStdin(t, repo, baseBlobHash+"\n",
		"pack-objects", "--index-version=1", filepath.Join(repo, "test-v1"))
	if err != nil {
		t.Fatalf("pack-objects: %v", err)
	}

	packSHA := strings.TrimSpace(stdout)
	idxPath := filepath.Join(repo, "test-v1-"+packSHA+".idx")

	idxBytes, err := os.ReadFile(idxPath)
	if err != nil {
		t.Fatal(err)
	}

	if string(idxBytes[:4]) == "\xfftOc" {
		t.Fatalf("expected v1 idx, got v2 (magic header present)")
	}
}

func TestPackObjectsAllReachable(t *testing.T) {
	t.Parallel()
	repo := setupRepoWithCommit(t)

	stdout, _, err := runGogit(t, repo, "pack-objects", "--all", filepath.Join(repo, "all"))
	if err != nil {
		t.Fatalf("pack-objects --all: %v", err)
	}

	if len(strings.TrimSpace(stdout)) != 2*sha1.Size {
		t.Fatalf("expected sha output, got %q", stdout)
	}
}

func TestPackObjectsRevIndex(t *testing.T) {
	t.Parallel()
	repo := setupRepoWithBlob(t, "base\n")

	stdout, _, err := runGogitStdin(t, repo, baseBlobHash+"\n",
		"-c", "pack.writeReverseIndex=true",
		"pack-objects", filepath.Join(repo, "test-rev"))
	if err != nil {
		t.Fatalf("pack-objects: %v", err)
	}

	packSHA := strings.TrimSpace(stdout)
	if _, err := os.Stat(filepath.Join(repo, "test-rev-"+packSHA+".rev")); err != nil {
		t.Fatalf("expected .rev file: %v", err)
	}
}
