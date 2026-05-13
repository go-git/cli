package main

import (
	"fmt"

	"github.com/go-git/go-git/v6"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(writeTreeCmd)
}

var writeTreeCmd = &cobra.Command{
	Use:   "write-tree",
	Short: "Create a tree object from the current index",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		r, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}

		defer r.Close()

		idx, err := r.Storer.Index()
		if err != nil {
			return err
		}

		hash, err := buildAndWriteTree(r, idx)
		if err != nil {
			return err
		}

		fmt.Fprintln(cmd.OutOrStdout(), hash)

		return nil
	},
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}
