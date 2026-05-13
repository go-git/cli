package main

import (
	"fmt"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(revParseCmd)
}

var revParseCmd = &cobra.Command{
	Use:   "rev-parse <rev>...",
	Short: "Pick out and massage parameters",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}

		defer r.Close()

		for _, rev := range args {
			h, err := r.ResolveRevision(plumbing.Revision(rev))
			if err != nil {
				return fmt.Errorf("resolve %s: %w", rev, err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), h)
		}

		return nil
	},
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}
