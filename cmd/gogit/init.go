package main

import (
	"fmt"

	"github.com/go-git/go-git/v6"
	"github.com/spf13/cobra"
)

var initTemplate string

func init() {
	initCmd.Flags().StringVar(&initTemplate, "template", "", "Template directory (accepted for compatibility, ignored)")
	rootCmd.AddCommand(initCmd)
}

var initCmd = &cobra.Command{
	Use:   "init [<directory>]",
	Short: "Create an empty Git repository",
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "."
		if len(args) == 1 {
			dir = args[0]
		}

		if _, err := git.PlainInit(dir, false); err != nil {
			return fmt.Errorf("failed to init repository: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Initialized empty Git repository in %s\n", dir)

		return nil
	},
	DisableFlagsInUseLine: true,
}
