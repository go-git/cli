package main

import (
	"errors"
	"fmt"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/spf13/cobra"
)

var cherryPickStrategy string

func init() {
	cherryPickCmd.Flags().StringVar(&cherryPickStrategy, "strategy-option", "theirs", "Conflict resolution preference: `theirs` (keep cherry-picked changes) or `ours` (keep current changes)")
	rootCmd.AddCommand(cherryPickCmd)
}

var cherryPickCmd = &cobra.Command{
	Use:   "cherry-pick <commit>...",
	Short: "Apply the changes introduced by some existing commits",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		strategy, err := parseCherryPickStrategy(cherryPickStrategy)
		if err != nil {
			return err
		}

		r, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}

		defer r.Close()

		commits, err := resolveCherryPickCommits(r, args)
		if err != nil {
			return err
		}

		w, err := r.Worktree()
		if err != nil {
			return fmt.Errorf("failed to open worktree: %w", err)
		}

		opts := &git.CommitOptions{}

		committer, err := signatureFromEnv("GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL", "GIT_COMMITTER_DATE")
		if err != nil && !errors.Is(err, errNoIdentityEnv) {
			return err
		}

		opts.Committer = committer

		return w.CherryPick(opts, strategy, commits...)
	},
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}

// parseCherryPickStrategy maps the user-facing --strategy-option value onto
// go-git's OrtMergeStrategyOption. go-git's CherryPick auto-resolves
// conflicting changes by picking one side; the upstream `-X theirs` / `-X
// ours` strategy options are the closest analogues, and `theirs` matches
// the default behaviour upstream `git cherry-pick` users intuit
// (incoming changes win).
func parseCherryPickStrategy(s string) (git.OrtMergeStrategyOption, error) {
	switch s {
	case "theirs":
		return git.TheirsMergeStrategy, nil
	case "ours":
		return git.OursMergeStrategy, nil
	default:
		return 0, fmt.Errorf("cherry-pick: unknown --strategy-option %q (expected `theirs` or `ours`)", s)
	}
}

// resolveCherryPickCommits turns each positional argument into a commit
// object. Any single bad reference fails the whole call before any commits
// are applied, matching upstream's "pre-flight validation" behaviour.
//
// Falls back to FromHex when ResolveRevision can't parse the input — go-git's
// resolver gates the full-hash branch on the sha1 hex length, so 64-char
// sha256 hex strings miss it and would otherwise fail.
func resolveCherryPickCommits(r *git.Repository, args []string) ([]*object.Commit, error) {
	out := make([]*object.Commit, 0, len(args))

	for _, arg := range args {
		var (
			hash plumbing.Hash
			ok   bool
		)

		if resolved, err := r.ResolveRevision(plumbing.Revision(arg)); err == nil {
			hash, ok = *resolved, true
		} else if h, fromHex := plumbing.FromHex(arg); fromHex {
			hash, ok = h, true
		}

		if !ok {
			return nil, fmt.Errorf("cherry-pick: %q does not name a known commit", arg)
		}

		commit, err := r.CommitObject(hash)
		if err != nil {
			return nil, fmt.Errorf("cherry-pick: not a commit object: %s", arg)
		}

		out = append(out, commit)
	}

	return out, nil
}
