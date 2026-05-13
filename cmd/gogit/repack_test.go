package main

import (
	"path/filepath"
	"testing"
)

func TestRepackADCleansLooseObjects(t *testing.T) {
	t.Parallel()
	repo := setupRepoWithCommit(t)

	loose, _ := filepath.Glob(filepath.Join(repo, ".git", "objects", "[0-9a-f][0-9a-f]", "*"))
	if len(loose) == 0 {
		t.Skip("expected loose objects to pack")
	}

	if _, _, err := runGogit(t, repo, "repack", "-ad"); err != nil {
		t.Fatalf("repack -ad: %v", err)
	}

	packs, _ := filepath.Glob(filepath.Join(repo, ".git", "objects", "pack", "pack-*.pack"))
	if len(packs) != 1 {
		t.Fatalf("expected 1 pack, got %d: %v", len(packs), packs)
	}

	stillLoose, _ := filepath.Glob(filepath.Join(repo, ".git", "objects", "[0-9a-f][0-9a-f]", "*"))
	if len(stillLoose) != 0 {
		t.Fatalf("expected no loose objects after repack -d, got %v", stillLoose)
	}
}
