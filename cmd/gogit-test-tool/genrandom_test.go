package main

import (
	"bytes"
	"testing"
)

func TestGenRandomMatchesUpstream(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	if err := genRandom(&buf, "foo", 8); err != nil {
		t.Fatalf("genRandom: %v", err)
	}

	// Golden bytes locked in from genRandom("foo", 8) output and verified
	// against a hand-trace of the algorithm for the first two bytes.
	want := []byte{0xd3, 0x1c, 0x75, 0x5b, 0xc4, 0x0f, 0x9d, 0xd0}

	if !bytes.Equal(buf.Bytes(), want) {
		t.Fatalf("genRandom(\"foo\", 8) = %x; want %x", buf.Bytes(), want)
	}
}

func TestGenRandomDeterministic(t *testing.T) {
	t.Parallel()

	var a, b bytes.Buffer

	if err := genRandom(&a, "seed", 64); err != nil {
		t.Fatalf("first: %v", err)
	}

	if err := genRandom(&b, "seed", 64); err != nil {
		t.Fatalf("second: %v", err)
	}

	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Fatal("two invocations with same seed/length differ")
	}

	if a.Len() != 64 {
		t.Fatalf("length = %d want 64", a.Len())
	}
}
