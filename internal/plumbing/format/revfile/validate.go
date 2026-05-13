// Package revfile provides extensions to go-git's plumbing/format/revfile that
// are intended to be upstreamed. Currently it covers explicit header
// validation (go-git's Decode collapses several distinct categories of
// corruption onto ErrMalformedRevFile) and row-position bounds checking.
package revfile

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	gogitrevfile "github.com/go-git/go-git/v6/plumbing/format/revfile"
)

// Header validation errors. go-git's Decode returns ErrMalformedRevFile for
// magic, ErrUnsupportedVersion for version, and ErrUnsupportedHashFunction
// for hash-id. We keep parallel sentinels so the caller can produce
// upstream-style fsck wording without resorting to string matching.
var (
	ErrUnknownSignature    = errors.New("unknown signature")
	ErrUnsupportedVersion  = errors.New("unsupported version")
	ErrUnsupportedHashFunc = errors.New("unsupported hash function")
	ErrInvalidRowPosition  = errors.New("invalid rev-index position")
)

// Magic and constants from gitformat-pack-rev.
var revMagic = []byte{'R', 'I', 'D', 'X'}

const (
	supportedVersion = 1
	sha1HashID       = 1
	sha256HashID     = 2
)

// ValidateHeader checks the 12-byte header of a .rev file payload (the full
// file bytes as read from disk). It returns one of ErrUnknownSignature,
// ErrUnsupportedVersion, or ErrUnsupportedHashFunc on mismatch, or nil if the
// header is well-formed.
func ValidateHeader(data []byte) error {
	if len(data) < 12 {
		return ErrUnknownSignature
	}

	if !bytes.Equal(data[0:4], revMagic) {
		return ErrUnknownSignature
	}

	version := beUint32(data[4:8])
	if version != supportedVersion {
		return ErrUnsupportedVersion
	}

	hashID := beUint32(data[8:12])
	if hashID != sha1HashID && hashID != sha256HashID {
		return ErrUnsupportedHashFunc
	}

	return nil
}

// ValidateRowPositions consumes the uint32 row-position stream produced by
// gogit-revfile.Decode and returns ErrInvalidRowPosition (wrapped with the
// offending entry) if any value is outside [0, objCount).
func ValidateRowPositions(ch <-chan uint32, objCount int64) error {
	var seen int64

	for v := range ch {
		if int64(v) >= objCount {
			err := fmt.Errorf("%w (entry %d = %d, max %d)",
				ErrInvalidRowPosition, seen, v, objCount-1)

			// Drain the rest so the producer is not blocked on a stalled
			// receiver.
			for range ch {
			}

			return err
		}

		seen++
	}

	return nil
}

// FsckMessage returns the fsck-compatible error string for a .rev file failure.
// Categories supported: ErrUnknownSignature, ErrUnsupportedVersion,
// ErrUnsupportedHashFunc, ErrInvalidRowPosition (from this package), plus
// gogitrevfile.ErrMalformedRevFile (with its wrapped message inspected for
// checksum vs position vs magic context).
//
// The basename is the .rev file name (e.g. "pack-<sha>.rev") and is included in
// the message so it can be grepped by t5325-style tests.
func FsckMessage(basename string, err error) string {
	switch {
	case errors.Is(err, ErrInvalidRowPosition):
		return fmt.Sprintf("reverse-index file %s: invalid rev-index position", basename)
	case errors.Is(err, ErrUnknownSignature):
		return fmt.Sprintf("reverse-index file %s has unknown signature", basename)
	case errors.Is(err, ErrUnsupportedVersion), errors.Is(err, gogitrevfile.ErrUnsupportedVersion):
		return fmt.Sprintf("reverse-index file %s has unsupported version", basename)
	case errors.Is(err, ErrUnsupportedHashFunc), errors.Is(err, gogitrevfile.ErrUnsupportedHashFunction):
		return fmt.Sprintf("reverse-index file %s has unsupported hash id", basename)
	case errors.Is(err, gogitrevfile.ErrMalformedRevFile):
		msg := err.Error()

		switch {
		case strings.Contains(msg, "magic"), strings.Contains(msg, "signature"):
			return fmt.Sprintf("reverse-index file %s has unknown signature", basename)
		case strings.Contains(msg, "wrong checksum"), strings.Contains(msg, "checksum"):
			return fmt.Sprintf("reverse-index file %s: invalid checksum", basename)
		case strings.Contains(msg, "rev-index"), strings.Contains(msg, "position"):
			return fmt.Sprintf("reverse-index file %s: invalid rev-index position", basename)
		}

		return fmt.Sprintf("reverse-index file %s: %s", basename, msg)
	default:
		return fmt.Sprintf("%s: %v", basename, err)
	}
}

func beUint32(b []byte) uint32 {
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}
