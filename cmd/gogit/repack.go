package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/spf13/cobra"
)

var (
	repackAll    bool
	repackDelete bool
)

func init() {
	repackCmd.Flags().BoolVarP(&repackAll, "all", "a", false, "Pack all reachable objects")
	repackCmd.Flags().BoolVarP(&repackDelete, "delete-redundant", "d", false, "Remove redundant objects after packing")
	rootCmd.AddCommand(repackCmd)
}

var repackCmd = &cobra.Command{
	Use:   "repack [-a] [-d]",
	Short: "Pack unpacked objects in a repository",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if !repackAll {
			return errors.New("repack without -a is not supported in v1")
		}

		r, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}

		defer r.Close()

		gitDir := repoGitDir(r)

		hashes, err := collectAllReachable(r)
		if err != nil {
			return err
		}

		repoCfg, _ := r.Config()
		writeRev := configBool("pack.writeReverseIndex", repoCfg, false)

		packBase := filepath.Join(gitDir, "objects", "pack", "pack")

		packHash, err := packObjectsInto(r, packBase, hashes, 2, writeRev)
		if err != nil {
			return err
		}

		fmt.Fprintln(cmd.OutOrStdout(), packHash)

		if repackDelete {
			if err := removeLooseObjects(gitDir, hashes); err != nil {
				return err
			}
		}

		return nil
	},
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}

// removeLooseObjects deletes loose object files whose SHA appears in packed.
func removeLooseObjects(gitDir string, packed []plumbing.Hash) error {
	packedSet := make(map[string]struct{}, len(packed))
	for _, h := range packed {
		packedSet[h.String()] = struct{}{}
	}

	root := filepath.Join(gitDir, "objects")

	dirs, err := os.ReadDir(root)
	if err != nil {
		return err
	}

	for _, d := range dirs {
		if !d.IsDir() || len(d.Name()) != 2 {
			continue
		}

		entries, err := os.ReadDir(filepath.Join(root, d.Name()))
		if err != nil {
			return err
		}

		for _, e := range entries {
			full := d.Name() + e.Name()
			if _, ok := packedSet[full]; ok {
				if err := os.Remove(filepath.Join(root, d.Name(), e.Name())); err != nil {
					return err
				}
			}
		}
	}

	return nil
}
