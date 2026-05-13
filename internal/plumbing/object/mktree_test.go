package object_test

import (
	"bytes"
	"strings"
	"testing"

	internalobject "github.com/go-git/cli/internal/plumbing/object"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	gogitobject "github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/plumbing/storer"
	"github.com/go-git/go-git/v6/storage/memory"
)

func TestParseMktreeInput(t *testing.T) {
	t.Parallel()

	zeros := strings.Repeat("0", 40)
	in := strings.Join([]string{
		"100644 blob 0123456789abcdef0123456789abcdef01234567\tfile1",
		"040000 tree fedcba9876543210fedcba9876543210fedcba98\tdir",
		"160000 commit " + zeros + "\tbroken",
		"",
	}, "\n")

	entries, err := internalobject.ParseMktreeInput(strings.NewReader(in))
	if err != nil {
		t.Fatalf("ParseMktreeInput: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("got %d entries; want 3", len(entries))
	}

	if entries[0].Mode != filemode.Regular || entries[0].Name != "file1" {
		t.Fatalf("entries[0] = %+v", entries[0])
	}

	if entries[1].Mode != filemode.Dir || entries[1].Name != "dir" {
		t.Fatalf("entries[1] = %+v", entries[1])
	}

	if entries[2].Mode != filemode.Submodule || entries[2].Name != "broken" {
		t.Fatalf("entries[2] = %+v", entries[2])
	}

	if entries[2].Hash != plumbing.ZeroHash {
		t.Fatalf("entries[2].Hash = %v; want ZeroHash", entries[2].Hash)
	}
}

func TestParseMktreeInputRejectsMalformed(t *testing.T) {
	t.Parallel()

	cases := []string{
		"no-tab-here",
		"too few spaces\tname",
		"",
	}

	for _, c := range cases {
		_, err := internalobject.ParseMktreeInput(strings.NewReader(c))
		if err == nil {
			t.Fatalf("ParseMktreeInput(%q): expected error", c)
		}
	}
}

func TestWriteTreeRawAcceptsNullHash(t *testing.T) {
	t.Parallel()

	store := memory.NewStorage()
	_ = (storer.EncodedObjectStorer)(store) // type assertion check

	obj := store.NewEncodedObject()
	obj.SetType(plumbing.TreeObject)

	entries := []gogitobject.TreeEntry{
		{Name: "broken", Mode: filemode.Submodule, Hash: plumbing.ZeroHash},
	}

	if err := internalobject.WriteTreeRaw(obj, entries); err != nil {
		t.Fatalf("WriteTreeRaw: %v", err)
	}

	hash, err := store.SetEncodedObject(obj)
	if err != nil {
		t.Fatalf("SetEncodedObject: %v", err)
	}

	if hash == plumbing.ZeroHash {
		t.Fatal("expected non-zero tree hash")
	}

	// Round-trip: load and verify the entry is preserved.
	read, err := store.EncodedObject(plumbing.TreeObject, hash)
	if err != nil {
		t.Fatal(err)
	}

	tree := &gogitobject.Tree{}
	if err := tree.Decode(read); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(tree.Entries) != 1 {
		t.Fatalf("got %d entries; want 1", len(tree.Entries))
	}

	if tree.Entries[0].Hash != plumbing.ZeroHash {
		t.Fatalf("entry hash = %v; want ZeroHash", tree.Entries[0].Hash)
	}

	// Reading back the raw bytes should also work.
	var buf bytes.Buffer

	rd, err := read.Reader()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := buf.ReadFrom(rd); err != nil {
		t.Fatal(err)
	}

	_ = rd.Close()
}
