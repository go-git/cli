package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTagCreatesLightweightTag(t *testing.T) {
	t.Parallel()
	repo := setupRepoWithCommit(t)

	if _, _, err := runGogit(t, repo, "tag", "v1"); err != nil {
		t.Fatalf("tag: %v", err)
	}

	data, err := readFile(t, filepath.Join(repo, ".git", "refs", "tags", "v1"))
	if err != nil {
		t.Fatalf("expected refs/tags/v1: %v", err)
	}

	if len(strings.TrimSpace(data)) != 40 {
		t.Fatalf("ref contents not a 40-char SHA: %q", data)
	}
}

func readFile(t *testing.T, path string) (string, error) {
	t.Helper()

	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	return string(b), nil
}
