package main

import (
	"crypto"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/format/idxfile"
	"github.com/go-git/go-git/v6/plumbing/hash"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(prunePackedCmd)
}

var prunePackedCmd = &cobra.Command{
	Use:   "prune-packed",
	Short: "Remove extra objects that are already in pack files",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		r, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}

		defer r.Close()

		gitDir := repoGitDir(r)

		packed, err := collectPackedHashes(gitDir)
		if err != nil {
			return err
		}

		return removeLooseIfPacked(gitDir, packed)
	},
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}

func collectPackedHashes(gitDir string) (map[plumbing.Hash]struct{}, error) {
	out := map[plumbing.Hash]struct{}{}

	matches, err := filepath.Glob(filepath.Join(gitDir, "objects", "pack", "pack-*.idx"))
	if err != nil {
		return nil, err
	}

	for _, m := range matches {
		f, err := os.Open(m)
		if err != nil {
			return nil, err
		}

		idx := idxfile.NewMemoryIndex(crypto.SHA1.Size())
		dec := idxfile.NewDecoder(f, hash.New(crypto.SHA1))

		if err := dec.Decode(idx); err != nil {
			_ = f.Close()

			return nil, fmt.Errorf("decode %s: %w", m, err)
		}

		_ = f.Close()

		iter, err := idx.Entries()
		if err != nil {
			return nil, err
		}

		for {
			e, err := iter.Next()
			if err != nil {
				break
			}

			out[e.Hash] = struct{}{}
		}
	}

	return out, nil
}

func removeLooseIfPacked(gitDir string, packed map[plumbing.Hash]struct{}) error {
	root := filepath.Join(gitDir, "objects")

	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}

	for _, e := range entries {
		if !e.IsDir() || len(e.Name()) != 2 {
			continue
		}

		if err := pruneOneObjectDir(filepath.Join(root, e.Name()), e.Name(), packed); err != nil {
			return err
		}
	}

	return nil
}

func pruneOneObjectDir(dir, prefix string, packed map[plumbing.Hash]struct{}) error {
	files, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, f := range files {
		full := prefix + f.Name()

		if len(full) != 40 {
			continue
		}

		h := plumbing.NewHash(full)

		if _, ok := packed[h]; !ok {
			continue
		}

		if err := os.Remove(filepath.Join(dir, f.Name())); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}

	return nil
}
