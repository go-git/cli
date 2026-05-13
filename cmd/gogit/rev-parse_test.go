package main

import (
	"strings"
	"testing"
)

func TestRevParseHEAD(t *testing.T) {
	t.Parallel()
	repo := setupRepoWithCommit(t)

	stdout, _, err := runGogit(t, repo, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}

	got := strings.TrimSpace(stdout)
	if len(got) != 40 {
		t.Fatalf("expected 40-char SHA, got %q", got)
	}
}
