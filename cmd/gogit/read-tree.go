package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/format/index"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/plumbing/storer"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(readTreeCmd)
}

var readTreeCmd = &cobra.Command{
	Use:   "read-tree <tree>",
	Short: "Read tree information into the index",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		r, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}

		defer r.Close()

		h, err := resolveTree(r, args[0])
		if err != nil {
			return fmt.Errorf("resolve %s: %w", args[0], err)
		}

		// object.Tree.Decode is permissive and does not validate null hashes,
		// so the normal load path works even for trees with null-SHA entries.
		tree, err := r.TreeObject(h)
		if err != nil {
			return fmt.Errorf("load tree %s: %w", h, err)
		}

		allowNull := os.Getenv("GIT_ALLOW_NULL_SHA1") != ""

		idx := &index.Index{Version: 2}

		if err = walkTreeIntoIndex(r, tree, "", allowNull, idx); err != nil {
			return err
		}

		return r.Storer.SetIndex(idx)
	},
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}

// resolveTree resolves a tree-ish argument to a tree hash.
// It accepts a 40-hex SHA directly, or a ref that points to a commit
// (in which case the commit's tree is used).
func resolveTree(r *git.Repository, arg string) (plumbing.Hash, error) {
	h := plumbing.NewHash(arg)
	if !h.IsZero() {
		// It looks like a full SHA1; check what object type it is.
		obj, err := r.Storer.EncodedObject(plumbing.AnyObject, h)
		if err != nil {
			return plumbing.ZeroHash, err
		}

		if obj.Type() == plumbing.TreeObject {
			return h, nil
		}

		if obj.Type() == plumbing.CommitObject {
			commit, err := object.GetCommit(r.Storer, h)
			if err != nil {
				return plumbing.ZeroHash, err
			}

			return commit.TreeHash, nil
		}

		return plumbing.ZeroHash, fmt.Errorf("%s is a %s, not a tree", arg, obj.Type())
	}

	// Try as a ref name pointing to a commit.
	ref, err := storer.ResolveReference(r.Storer, plumbing.NewBranchReferenceName(arg))
	if err != nil {
		return plumbing.ZeroHash, plumbing.ErrReferenceNotFound
	}

	commit, err := object.GetCommit(r.Storer, ref.Hash())
	if err != nil {
		return plumbing.ZeroHash, err
	}

	return commit.TreeHash, nil
}

func walkTreeIntoIndex(r *git.Repository, tree *object.Tree, prefix string, allowNull bool, idx *index.Index) error {
	for _, e := range tree.Entries {
		name := e.Name
		if prefix != "" {
			name = prefix + "/" + e.Name
		}

		if e.Mode == filemode.Dir {
			sub, err := r.TreeObject(e.Hash)
			if err != nil {
				return fmt.Errorf("load subtree %s: %w", e.Hash, err)
			}

			if err := walkTreeIntoIndex(r, sub, name, allowNull, idx); err != nil {
				return err
			}

			continue
		}

		if e.Hash == plumbing.ZeroHash && !allowNull {
			return errors.New("read-tree: tree contains entry with null sha (set GIT_ALLOW_NULL_SHA1=1 to bypass)")
		}

		idx.Entries = append(idx.Entries, &index.Entry{
			Name: name,
			Mode: e.Mode,
			Hash: e.Hash,
		})
	}

	return nil
}
