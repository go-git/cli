package main

import (
	"crypto"
	"io"
	"os"

	"github.com/go-git/go-git/v6/plumbing/format/idxfile"
	"github.com/go-git/go-git/v6/plumbing/format/revfile"
	"github.com/go-git/go-git/v6/plumbing/hash"
)

// encodeRevIndex writes the reverse-index encoding of idx to w.
func encodeRevIndex(idx *idxfile.MemoryIndex, w io.Writer) error {
	return revfile.Encode(w, hash.New(crypto.SHA1), idx)
}

// writeRevIndex emits a .rev file derived from the given idx at outPath.
func writeRevIndex(idx *idxfile.MemoryIndex, outPath string) error {
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return encodeRevIndex(idx, f)
}
