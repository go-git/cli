package main

import (
	"errors"
	"sort"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/format/index"
	"github.com/go-git/go-git/v6/plumbing/object"
)

// buildAndWriteTree groups idx.Entries by directory, recursively builds tree
// objects, writes them all to the object store, and returns the root tree's
// hash. Refuses (with error) if any entry has a null hash.
func buildAndWriteTree(r *git.Repository, idx *index.Index) (plumbing.Hash, error) {
	for _, e := range idx.Entries {
		if e.Hash == plumbing.ZeroHash {
			return plumbing.ZeroHash, errors.New("write-tree: index contains entry with null sha")
		}
	}

	return writeSubtree(r, idx.Entries, "")
}

type treeChild struct {
	name       string
	isDir      bool
	fileMode   filemode.FileMode
	fileHash   plumbing.Hash
	dirEntries []*index.Entry
}

// writeSubtree builds the tree for entries under dirPrefix and returns its hash.
// dirPrefix is "" for the root, "a/", "a/b/" etc. for nested subtrees.
func writeSubtree(r *git.Repository, entries []*index.Entry, dirPrefix string) (plumbing.Hash, error) {
	byName := map[string]*treeChild{}

	var order []string

	for _, e := range entries {
		if !strings.HasPrefix(e.Name, dirPrefix) {
			continue
		}

		rest := e.Name[len(dirPrefix):]

		dirName, _, isDir := strings.Cut(rest, "/")
		if !isDir {
			c, exists := byName[rest]
			if !exists {
				c = &treeChild{name: rest}
				byName[rest] = c
				order = append(order, rest)
			}

			c.fileMode = e.Mode
			c.fileHash = e.Hash

			continue
		}

		c, exists := byName[dirName]
		if !exists {
			c = &treeChild{name: dirName, isDir: true}
			byName[dirName] = c
			order = append(order, dirName)
		}

		c.isDir = true
		c.dirEntries = append(c.dirEntries, e)
	}

	sort.Slice(order, func(i, j int) bool {
		return treeEntryLess(byName[order[i]], byName[order[j]])
	})

	var treeEntries []object.TreeEntry

	for _, n := range order {
		c := byName[n]

		if c.isDir {
			subHash, err := writeSubtree(r, c.dirEntries, dirPrefix+n+"/")
			if err != nil {
				return plumbing.ZeroHash, err
			}

			treeEntries = append(treeEntries, object.TreeEntry{
				Name: n,
				Mode: filemode.Dir,
				Hash: subHash,
			})

			continue
		}

		treeEntries = append(treeEntries, object.TreeEntry{
			Name: n,
			Mode: c.fileMode,
			Hash: c.fileHash,
		})
	}

	tree := &object.Tree{Entries: treeEntries}
	obj := r.Storer.NewEncodedObject()

	if err := tree.Encode(obj); err != nil {
		return plumbing.ZeroHash, err
	}

	return r.Storer.SetEncodedObject(obj)
}

// treeEntryLess implements upstream's base_name_compare: tree entries are
// compared by name with directory entries getting an implicit trailing '/'.
func treeEntryLess(a, b *treeChild) bool {
	an := a.name

	if a.isDir {
		an += "/"
	}

	bn := b.name

	if b.isDir {
		bn += "/"
	}

	return an < bn
}
