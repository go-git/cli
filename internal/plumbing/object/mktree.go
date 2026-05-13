package object

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/object"
)

// ParseMktreeInput reads the `ls-tree` text format used as input by
// `git mktree` — one entry per line: `<mode> SP <type> SP <oid> TAB <name>`.
// Returns the corresponding tree entries. The type token is required for
// format parity with `ls-tree` but is not consulted: mode alone determines
// the entry semantics.
//
// Null (all-zero) hashes are accepted; upstream `git mktree` allows them
// and downstream validation is the consumer's responsibility.
func ParseMktreeInput(r io.Reader) ([]object.TreeEntry, error) {
	var entries []object.TreeEntry

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		head, name, ok := strings.Cut(line, "\t")
		if !ok {
			return nil, fmt.Errorf("mktree: missing TAB in line %q", line)
		}

		parts := strings.SplitN(head, " ", 3)
		if len(parts) != 3 {
			return nil, fmt.Errorf("mktree: malformed line %q", line)
		}

		modeStr, _, oid := parts[0], parts[1], parts[2]

		mode, err := filemode.New(modeStr)
		if err != nil {
			return nil, fmt.Errorf("mktree: invalid mode %q: %w", modeStr, err)
		}

		entries = append(entries, object.TreeEntry{
			Name: name,
			Mode: mode,
			Hash: plumbing.NewHash(oid),
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return nil, errors.New("mktree: no entries on stdin")
	}

	return entries, nil
}

// WriteTreeRaw serialises the given entries into obj as a Git tree object.
// Unlike object.Tree.Encode, this does NOT run Tree.Validate — so entries
// with null hashes (which `git mktree` accepts) round-trip cleanly.
//
// obj must already be configured with type plumbing.TreeObject before this
// is called.
func WriteTreeRaw(obj plumbing.EncodedObject, entries []object.TreeEntry) error {
	w, err := obj.Writer()
	if err != nil {
		return err
	}

	for _, e := range entries {
		if _, err := fmt.Fprintf(w, "%o %s\x00", e.Mode, e.Name); err != nil {
			_ = w.Close()

			return err
		}

		if _, err := e.Hash.WriteTo(w); err != nil {
			_ = w.Close()

			return err
		}
	}

	return w.Close()
}
