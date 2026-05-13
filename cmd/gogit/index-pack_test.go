package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// emptyBlobPackEntry is the on-the-wire (zlib-compressed) packfile entry for the
// canonical empty blob. Lifted verbatim from upstream's t/lib-pack.sh, which is
// the same trick t5308 uses to construct test packs.
var emptyBlobPackEntry = []byte{0x30, 0x78, 0x9c, 0x03, 0x00, 0x00, 0x00, 0x00, 0x01}

// makePack returns a valid v2 packfile containing `count` copies of the empty
// blob entry, with a correct SHA1 trailer.
func makePack(t *testing.T, count int) []byte {
	t.Helper()

	var buf bytes.Buffer

	buf.WriteString("PACK")

	if err := binary.Write(&buf, binary.BigEndian, uint32(2)); err != nil {
		t.Fatal(err)
	}

	if err := binary.Write(&buf, binary.BigEndian, uint32(count)); err != nil {
		t.Fatal(err)
	}

	for range count {
		buf.Write(emptyBlobPackEntry)
	}

	h := sha1.Sum(buf.Bytes())
	buf.Write(h[:])

	return buf.Bytes()
}

func TestIndexPackStdinAcceptsCleanPack(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()

	if _, _, err := runGogit(t, repo, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	pack := makePack(t, 1)
	if _, stderr, err := runGogitStdin(t, repo, string(pack), "index-pack", "--stdin"); err != nil {
		t.Fatalf("index-pack --stdin failed: %v\nstderr: %s", err, stderr)
	}

	matches, err := filepath.Glob(filepath.Join(repo, ".git", "objects", "pack", "pack-*.pack"))
	if err != nil {
		t.Fatal(err)
	}

	if len(matches) != 1 {
		t.Fatalf("expected exactly one pack file, got %d: %v", len(matches), matches)
	}

	idxPath := matches[0][:len(matches[0])-5] + ".idx"
	if _, err := os.Stat(idxPath); err != nil {
		t.Fatalf("expected idx alongside pack: %v", err)
	}
}

func TestIndexPackStrictRejectsDuplicates(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()

	if _, _, err := runGogit(t, repo, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	pack := makePack(t, 2)
	if _, _, err := runGogitStdin(t, repo, string(pack), "index-pack", "--strict", "--stdin"); err == nil {
		t.Fatal("expected non-zero exit for duplicate-object pack under --strict")
	}

	matches, err := filepath.Glob(filepath.Join(repo, ".git", "objects", "pack", "pack-*.pack"))
	if err != nil {
		t.Fatal(err)
	}

	if len(matches) != 0 {
		t.Fatalf("expected no pack file left behind, got %d: %v", len(matches), matches)
	}
}
