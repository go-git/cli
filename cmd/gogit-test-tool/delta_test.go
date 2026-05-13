package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v6/plumbing/format/packfile"
)

func TestDeltaApplyRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	src := []byte("hello, world\nthis is the base text\n")
	tgt := []byte("hello, world\nthis is the target text with edits\n")

	deltaBytes := packfile.DiffDelta(src, tgt)

	srcPath := filepath.Join(dir, "src")
	deltaPath := filepath.Join(dir, "delta")
	outPath := filepath.Join(dir, "out")

	if err := os.WriteFile(srcPath, src, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(deltaPath, deltaBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runDelta([]string{"-p", srcPath, deltaPath, outPath}); err != nil {
		t.Fatalf("runDelta: %v", err)
	}

	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != string(tgt) {
		t.Fatalf("round-trip mismatch:\n got: %q\nwant: %q", got, tgt)
	}
}

func TestDeltaApplyRejectsCorrupt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	src := []byte("base")
	corruptDelta := []byte{0xff, 0xff, 0xff} // not a valid delta header

	srcPath := filepath.Join(dir, "src")
	deltaPath := filepath.Join(dir, "delta")
	outPath := filepath.Join(dir, "out")

	if err := os.WriteFile(srcPath, src, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(deltaPath, corruptDelta, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runDelta([]string{"-p", srcPath, deltaPath, outPath}); err == nil {
		t.Fatal("expected non-nil error on corrupt delta")
	}
}
