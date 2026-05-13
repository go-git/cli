package main

import (
	"strings"
	"testing"
)

func TestRevListAllObjects(t *testing.T) {
	t.Parallel()
	repo := setupRepoWithCommit(t)

	stdout, _, err := runGogit(t, repo, "rev-list", "--objects", "--no-object-names", "--all")
	if err != nil {
		t.Fatalf("rev-list: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 objects (commit, tree, blob), got %d: %v", len(lines), lines)
	}

	for _, line := range lines {
		if len(line) != 40 {
			t.Fatalf("expected 40-char SHA per line, got %q", line)
		}
	}
}
