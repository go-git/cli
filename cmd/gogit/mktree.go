package main

import (
	"fmt"

	internalobject "github.com/go-git/cli/internal/plumbing/object"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(mktreeCmd)
}

var mktreeCmd = &cobra.Command{
	Use:   "mktree",
	Short: "Build a tree-object from ls-tree formatted text",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		r, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}

		defer r.Close()

		entries, err := internalobject.ParseMktreeInput(cmd.InOrStdin())
		if err != nil {
			return err
		}

		obj := r.Storer.NewEncodedObject()
		obj.SetType(plumbing.TreeObject)

		if err := internalobject.WriteTreeRaw(obj, entries); err != nil {
			return err
		}

		hash, err := r.Storer.SetEncodedObject(obj)
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
