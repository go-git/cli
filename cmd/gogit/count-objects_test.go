package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestCountObjectsBasic(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()

	if _, _, err := runGogit(t, repo, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repo, "f"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := runGogit(t, repo, "add", "f"); err != nil {
		t.Fatalf("add: %v", err)
	}

	if _, _, err := runGogitEnv(t, repo, gitIdentityEnv(repo), "commit", "-m", "x"); err != nil {
		t.Fatalf("commit: %v", err)
	}

	stdout, _, err := runGogit(t, repo, "count-objects")
	if err != nil {
		t.Fatalf("count-objects: %v", err)
	}

	re := regexp.MustCompile(`^\d+ objects, \d+ kilobytes\n$`)
	if !re.MatchString(stdout) {
		t.Fatalf("stdout = %q does not match `<N> objects, <K> kilobytes`", stdout)
	}
}
