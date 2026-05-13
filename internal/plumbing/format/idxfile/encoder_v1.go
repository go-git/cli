// Package idxfile provides extensions to go-git's plumbing/format/idxfile that
// are intended to be upstreamed. Currently it covers the v1 pack-index encoder
// (go-git only ships a v2 encoder via idxfile.Encode).
package idxfile

import (
	"bytes"
	"crypto"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/format/idxfile"
	"github.com/go-git/go-git/v6/plumbing/hash"
)

// ErrOffsetTooLargeForV1 is returned by EncodeV1 when any object's pack offset
// exceeds the 4 GiB range representable by the v1 idx format.
var ErrOffsetTooLargeForV1 = errors.New("v1 idx cannot represent offsets > 4 GiB")

// EncodeV1 writes idx as a legacy version-1 pack index file. The format is:
//
//   - 256 fanout entries, each a 4-byte big-endian unsigned integer giving the
//     cumulative count of objects whose first hash byte is ≤ index.
//   - Per object: a 4-byte big-endian offset followed by a 20-byte SHA-1 hash,
//     in ascending hash order.
//   - 20-byte SHA-1 of the pack file (provided via packHash).
//   - 20-byte SHA-1 of all preceding idx bytes.
//
// V1 cannot represent pack offsets ≥ 2^32; EncodeV1 returns ErrOffsetTooLargeForV1
// when any entry's offset exceeds that range. Currently SHA-1 hashes only.
func EncodeV1(w io.Writer, idx *idxfile.MemoryIndex, packHash plumbing.Hash) error {
	entries, err := collectAndSortEntries(idx)
	if err != nil {
		return err
	}

	hashFn := hash.New(crypto.SHA1)
	mw := io.MultiWriter(w, hashFn)

	if err := writeV1Fanout(mw, entries); err != nil {
		return err
	}

	if err := writeV1Entries(mw, entries); err != nil {
		return err
	}

	if _, err := mw.Write(packHash.Bytes()); err != nil {
		return err
	}

	if _, err := w.Write(hashFn.Sum(nil)); err != nil {
		return err
	}

	return nil
}

type v1Entry struct {
	hash   plumbing.Hash
	offset uint32
}

func collectAndSortEntries(idx *idxfile.MemoryIndex) ([]v1Entry, error) {
	iter, err := idx.Entries()
	if err != nil {
		return nil, err
	}

	var entries []v1Entry

	for {
		e, err := iter.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return nil, err
		}

		if e.Offset > 0xFFFFFFFF {
			return nil, fmt.Errorf("%w: object %s at offset %d", ErrOffsetTooLargeForV1, e.Hash, e.Offset)
		}

		entries = append(entries, v1Entry{hash: e.Hash, offset: uint32(e.Offset)})
	}

	sort.Slice(entries, func(i, j int) bool {
		return bytes.Compare(entries[i].hash.Bytes(), entries[j].hash.Bytes()) < 0
	})

	return entries, nil
}

func writeV1Fanout(w io.Writer, entries []v1Entry) error {
	var fanout [256]uint32

	for _, e := range entries {
		b := e.hash.Bytes()
		fanout[b[0]]++
	}

	var running uint32

	for i := range fanout {
		running += fanout[i]
		fanout[i] = running
	}

	for _, v := range fanout {
		if err := binary.Write(w, binary.BigEndian, v); err != nil {
			return err
		}
	}

	return nil
}

func writeV1Entries(w io.Writer, entries []v1Entry) error {
	for _, e := range entries {
		if err := binary.Write(w, binary.BigEndian, e.offset); err != nil {
			return err
		}

		if _, err := w.Write(e.hash.Bytes()); err != nil {
			return err
		}
	}

	return nil
}
