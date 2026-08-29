package main

import (
	"errors"
	"fmt"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/spf13/cobra"
)

var mergeFFOnly bool

func init() {
	mergeCmd.Flags().BoolVar(&mergeFFOnly, "ff-only", false, "Refuse to merge unless the merge can be resolved as a fast-forward")
	rootCmd.AddCommand(mergeCmd)
}

var mergeCmd = &cobra.Command{
	Use:   "merge <branch>",
	Short: "Join two development histories together",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}

		defer r.Close()

		target, err := resolveMergeTarget(r, args[0])
		if err != nil {
			return err
		}

		err = r.Merge(*target, git.MergeOptions{Strategy: git.FastForwardMerge})

		switch {
		case errors.Is(err, git.ErrFastForwardMergeNotPossible):
			if mergeFFOnly {
				return fmt.Errorf("fatal: Not possible to fast-forward, aborting")
			}

			return fmt.Errorf("non-fast-forward merge not supported (use --ff-only)")
		case err != nil:
			return fmt.Errorf("merge: %w", err)
		}

		// r.Merge advances the HEAD ref but does not touch the worktree.
		// Sync the worktree by checking out HEAD's new tip, matching the
		// behaviour `git merge --ff-only` has on a non-bare repository.
		head, err := r.Head()
		if err != nil {
			return fmt.Errorf("read HEAD after merge: %w", err)
		}

		w, err := r.Worktree()
		if err != nil {
			// Bare repos have no worktree; the ref update above is the
			// whole merge.
			if errors.Is(err, git.ErrIsBareRepository) {
				return nil
			}

			return fmt.Errorf("open worktree: %w", err)
		}

		return w.Checkout(&git.CheckoutOptions{Branch: head.Name(), Force: true})
	},
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}

// resolveMergeTarget interprets a branch name (or local ref short name) and
// returns the corresponding plumbing.Reference. Falls back through a few
// common forms: full reference name, "refs/heads/<name>" branch, then bare
// short-name lookup.
func resolveMergeTarget(r *git.Repository, name string) (*plumbing.Reference, error) {
	for _, candidate := range []plumbing.ReferenceName{
		plumbing.ReferenceName(name),
		plumbing.NewBranchReferenceName(name),
	} {
		ref, err := r.Reference(candidate, true)
		if err == nil {
			return ref, nil
		}
	}

	return nil, fmt.Errorf("merge: %q does not name a known reference", name)
}
