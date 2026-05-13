package main

import (
	"fmt"

	"github.com/go-git/go-git/v6"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(addCmd)
}

var addCmd = &cobra.Command{
	Use:   "add <pathspec>...",
	Short: "Add file contents to the index",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}

		w, err := r.Worktree()
		if err != nil {
			return fmt.Errorf("failed to open worktree: %w", err)
		}

		for _, path := range args {
			if _, err := w.Add(path); err != nil {
				return fmt.Errorf("failed to add %s: %w", path, err)
			}
		}

		return nil
	},
	DisableFlagsInUseLine: true,
}
