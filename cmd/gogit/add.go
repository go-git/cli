package main

import (
	"fmt"
	"os"
	"path/filepath"

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

		defer r.Close()

		gitDir := repoGitDir(r)
		_, statErr := os.Stat(filepath.Join(gitDir, "index"))
		hadExistingIndex := statErr == nil

		repoCfg, _ := r.Config()
		version := pickIndexVersion(repoCfg, os.LookupEnv, hadExistingIndex, cmd.ErrOrStderr())

		w, err := r.Worktree()
		if err != nil {
			return fmt.Errorf("failed to open worktree: %w", err)
		}

		for _, path := range args {
			if _, err := w.Add(path); err != nil {
				return fmt.Errorf("failed to add %s: %w", path, err)
			}
		}

		idx, err := r.Storer.Index()
		if err != nil {
			return err
		}

		if idx.Version != version {
			idx.Version = version

			if err := r.Storer.SetIndex(idx); err != nil {
				return err
			}
		}

		return nil
	},
	DisableFlagsInUseLine: true,
}
