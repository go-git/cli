package main

import (
	"fmt"

	"github.com/go-git/go-git/v6"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(tagCmd)
}

var tagCmd = &cobra.Command{
	Use:   "tag <name>",
	Short: "Create a tag",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		name := args[0]

		r, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}

		defer r.Close()

		head, err := r.Head()
		if err != nil {
			return err
		}

		if _, err := r.CreateTag(name, head.Hash(), nil); err != nil {
			return err
		}

		return nil
	},
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}
