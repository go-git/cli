package main

import (
	"os"
	"path/filepath"
	"testing"
)

const baseBlobOID = "df967b96a579e45a18b8251732d16804b2e56a55" // sha1 of "blob 5\0base\n"

func setupRepoWithBaseBlob(t *testing.T) string {
	t.Helper()

	repo := t.TempDir()

	if _, _, err := runGogit(t, repo, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repo, "file0"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := runGogit(t, repo, "add", "file0"); err != nil {
		t.Fatalf("add: %v", err)
	}

	if _, _, err := runGogitEnv(t, repo, gitIdentityEnv(repo), "commit", "-m", "x"); err != nil {
		t.Fatalf("commit: %v", err)
	}

	return repo
}

func TestCatFileExistsExitsZero(t *testing.T) {
	t.Parallel()

	repo := setupRepoWithBaseBlob(t)

	if _, _, err := runGogit(t, repo, "cat-file", "-e", baseBlobOID); err != nil {
		t.Fatalf("cat-file -e <existing>: expected exit 0, got %v", err)
	}
}

func TestCatFileMissingExitsOne(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()

	if _, _, err := runGogit(t, repo, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	stdout, _, err := runGogit(t, repo, "cat-file", "-e", "0000000000000000000000000000000000000000")
	if err == nil {
		t.Fatalf("expected non-zero exit, got success")
	}

	if stdout != "" {
		t.Fatalf("expected no stdout, got %q", stdout)
	}
}

func TestCatFileBatchCheck(t *testing.T) {
	t.Parallel()

	repo := setupRepoWithBaseBlob(t)

	const missingOID = "0000000000000000000000000000000000000000"

	input := baseBlobOID + "\n" + missingOID + "\n"
	want := baseBlobOID + " blob 5\n" + missingOID + " missing\n"

	stdout, stderr, err := runGogitStdin(t, repo, input, "cat-file", "--batch-check")
	if err != nil {
		t.Fatalf("cat-file --batch-check failed: %v\nstderr: %s", err, stderr)
	}

	if stdout != want {
		t.Fatalf("batch-check output mismatch:\n got: %q\nwant: %q", stdout, want)
	}
}
