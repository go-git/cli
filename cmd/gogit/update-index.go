package main

import (
	"errors"
	"fmt"

	"github.com/go-git/go-git/v6"
	"github.com/spf13/cobra"
)

var updateIndexShowVersion bool

func init() {
	updateIndexCmd.Flags().BoolVar(&updateIndexShowVersion, "show-index-version", false,
		"Print the index format version")
	rootCmd.AddCommand(updateIndexCmd)
}

var updateIndexCmd = &cobra.Command{
	Use:   "update-index --show-index-version",
	Short: "Register file contents in the working tree to the index",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if !updateIndexShowVersion {
			return errors.New("only --show-index-version is supported in v1")
		}

		r, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}

		defer r.Close()

		idx, err := r.Storer.Index()
		if err != nil {
			return err
		}

		fmt.Fprintln(cmd.OutOrStdout(), idx.Version)

		return nil
	},
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}
