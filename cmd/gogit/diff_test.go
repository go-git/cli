package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiffNoIndexEqual(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")

	if err := os.WriteFile(a, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(b, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := runGogit(t, dir, "diff", "--no-index", "--", a, b); err != nil {
		t.Fatalf("expected exit 0 for equal files: %v", err)
	}
}

func TestDiffNoIndexDifferentExitsOne(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")

	if err := os.WriteFile(a, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(b, []byte("world\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := runGogit(t, dir, "diff", "--no-index", "--", a, b); err == nil {
		t.Fatal("expected non-zero exit for different files")
	}
}

func TestDiffNoIndexIgnoresCRAtEOL(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")

	if err := os.WriteFile(a, []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(b, []byte("hello\r\nworld\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := runGogit(t, dir, "diff", "--no-index", "--ignore-cr-at-eol", "--", a, b); err != nil {
		t.Fatalf("expected exit 0 with --ignore-cr-at-eol: %v", err)
	}
}
