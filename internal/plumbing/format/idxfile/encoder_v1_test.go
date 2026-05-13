package idxfile_test

import (
	"bytes"
	"crypto"
	"encoding/binary"
	"errors"
	"testing"

	internalidxfile "github.com/go-git/cli/internal/plumbing/format/idxfile"
	"github.com/go-git/go-git/v6/plumbing"
	gogitidxfile "github.com/go-git/go-git/v6/plumbing/format/idxfile"
	"github.com/go-git/go-git/v6/plumbing/hash"
)

func TestEncodeV1Roundtrip(t *testing.T) {
	t.Parallel()

	// Build a small in-memory idx via go-git's Writer-from-pack pipeline by
	// hand: write three (hash, offset) entries via the encoder directly.
	w := &gogitidxfile.Writer{}
	if err := w.OnHeader(3); err != nil {
		t.Fatal(err)
	}

	hashes := []plumbing.Hash{
		plumbing.NewHash("0000000000000000000000000000000000000001"),
		plumbing.NewHash("0000000000000000000000000000000000000002"),
		plumbing.NewHash("00000000000000000000000000000000000000ff"),
	}

	for i, h := range hashes {
		if err := w.OnInflatedObjectContent(h, int64(100*i), 0, nil); err != nil {
			t.Fatal(err)
		}
	}

	if err := w.OnFooter(plumbing.ZeroHash); err != nil {
		t.Fatal(err)
	}

	idx, err := w.Index()
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer

	packHash := plumbing.NewHash("aabbccddeeff00112233445566778899aabbccdd")
	if err := internalidxfile.EncodeV1(&buf, idx, packHash); err != nil {
		t.Fatalf("EncodeV1: %v", err)
	}

	out := buf.Bytes()
	if got := len(out); got < 256*4+3*24+20+20 {
		t.Fatalf("output too short: %d bytes", got)
	}

	// v2 idx starts with the magic \xfftOc; v1 must NOT.
	if bytes.Equal(out[:4], []byte{0xff, 't', 'O', 'c'}) {
		t.Fatalf("v1 idx must not start with v2 magic header")
	}

	// First 256 uint32s are the fanout. Last entry is total object count.
	totalCount := binary.BigEndian.Uint32(out[255*4 : 256*4])
	if totalCount != 3 {
		t.Fatalf("fanout total = %d; want 3", totalCount)
	}
}

func TestEncodeV1OffsetTooLarge(t *testing.T) {
	t.Parallel()

	w := &gogitidxfile.Writer{}
	_ = w.OnHeader(1)

	if err := w.OnInflatedObjectContent(
		plumbing.NewHash("0000000000000000000000000000000000000001"),
		int64(1)<<33, // > 2^32-1
		0, nil); err != nil {
		t.Fatal(err)
	}

	_ = w.OnFooter(plumbing.ZeroHash)

	built, err := w.Index()
	if err != nil {
		t.Fatal(err)
	}

	err = internalidxfile.EncodeV1(&bytes.Buffer{}, built, plumbing.ZeroHash)
	if !errors.Is(err, internalidxfile.ErrOffsetTooLargeForV1) {
		t.Fatalf("got %v; want ErrOffsetTooLargeForV1", err)
	}

	_ = hash.New(crypto.SHA1) // keep imports tidy if the test trims later
}
