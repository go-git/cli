package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/storage/filesystem"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(countObjectsCmd)
}

var countObjectsCmd = &cobra.Command{
	Use:   "count-objects",
	Short: "Count unpacked number of objects and their disk consumption",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		r, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}

		defer r.Close()

		gitDir := repoGitDir(r)

		count, bytes, err := walkLooseObjects(gitDir)
		if err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "%d objects, %d kilobytes\n", count, bytes/1024)

		return nil
	},
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}

// repoGitDir returns the absolute path to the .git directory (or the bare repo
// root). Uses the filesystem.Storage's root when available so it works in
// worktrees and subdirectories. Falls back to a best-effort cwd-based guess.
func repoGitDir(r *git.Repository) string {
	if s, ok := r.Storer.(*filesystem.Storage); ok {
		return s.Filesystem().Root()
	}

	wd, err := os.Getwd()
	if err != nil {
		return ".git"
	}

	candidate := filepath.Join(wd, ".git")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}

	return ".git"
}

// walkLooseObjects sums loose object count and bytes under <gitDir>/objects.
func walkLooseObjects(gitDir string) (int, int64, error) {
	root := filepath.Join(gitDir, "objects")

	entries, err := os.ReadDir(root)
	if err != nil {
		return 0, 0, err
	}

	var count int

	var totalBytes int64

	for _, e := range entries {
		if !e.IsDir() || len(e.Name()) != 2 {
			continue
		}

		inner, err := os.ReadDir(filepath.Join(root, e.Name()))
		if err != nil {
			return 0, 0, err
		}

		for _, f := range inner {
			info, err := f.Info()
			if err != nil {
				return 0, 0, err
			}

			count++

			totalBytes += info.Size()
		}
	}

	return count, totalBytes, nil
}
