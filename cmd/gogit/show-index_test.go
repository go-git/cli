package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestShowIndexParsesV2Idx(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()

	if _, _, err := runGogit(t, repo, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Build a minimal v2 pack via index-pack --stdin, then read its idx and pipe through show-index.
	pack := makePack(t, 1)
	if _, _, err := runGogitStdin(t, repo, string(pack), "index-pack", "--stdin"); err != nil {
		t.Fatalf("index-pack: %v", err)
	}

	matches, _ := filepath.Glob(filepath.Join(repo, ".git", "objects", "pack", "pack-*.idx"))
	if len(matches) != 1 {
		t.Fatalf("expected 1 idx, got %d", len(matches))
	}

	idxBytes, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}

	stdout, _, err := runGogitStdin(t, repo, string(idxBytes), "show-index")
	if err != nil {
		t.Fatalf("show-index: %v", err)
	}

	// One line per object: <decimal offset> <hex sha40> <crc32 8-hex>
	re := regexp.MustCompile(`^\d+ [0-9a-f]{40} [0-9a-f]{8}\n$`)
	if !re.MatchString(stdout) {
		t.Fatalf("show-index line = %q does not match expected v2 format", stdout)
	}
}
