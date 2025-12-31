package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-git/go-billy/v6/memfs"
	"github.com/go-git/go-billy/v6/osfs"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/x/plumbing/worktree"
	xstorage "github.com/go-git/go-git/v6/x/storage"
	"github.com/spf13/cobra"
)

var (
	worktreeAddCommit string
	worktreeAddDetach bool
)

func init() {
	worktreeAddCmd.Flags().StringVarP(&worktreeAddCommit, "commit", "c", "", "Commit hash to checkout in the new worktree")
	worktreeAddCmd.Flags().BoolVarP(&worktreeAddDetach, "detach", "d", false, "Create a detached HEAD worktree")
	worktreeCmd.AddCommand(worktreeAddCmd)
	worktreeCmd.AddCommand(worktreeListCmd)
	worktreeCmd.AddCommand(worktreeRemoveCmd)
	rootCmd.AddCommand(worktreeCmd)
}

var worktreeCmd = &cobra.Command{
	Use:   "worktree <command>",
	Short: "Manage repository worktrees",
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Usage()
	},
	DisableFlagsInUseLine: true,
}

var worktreeAddCmd = &cobra.Command{
	Use:   "add <path>",
	Short: "Add a new linked worktree",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := args[0]
		name := filepath.Base(path)

		r, err := git.PlainOpen(".")
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}

		w, err := worktree.New(r.Storer)
		if err != nil {
			return fmt.Errorf("failed to create worktree manager: %w", err)
		}

		wt := osfs.New(path)

		var opts []worktree.Option
		if worktreeAddDetach {
			opts = append(opts, worktree.WithDetachedHead())
		}
		if worktreeAddCommit != "" {
			hash := plumbing.NewHash(worktreeAddCommit)
			if !hash.IsZero() {
				opts = append(opts, worktree.WithCommit(hash))
			}
		}

		err = w.Add(wt, name, opts...)
		if err != nil {
			return fmt.Errorf("failed to add worktree: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Worktree '%s' created at '%s'\n", name, path)

		return nil
	},
	DisableFlagsInUseLine: true,
}

var worktreeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all linked worktrees",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		r, err := git.PlainOpen(".")
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}

		w, err := worktree.New(r.Storer)
		if err != nil {
			return fmt.Errorf("failed to create worktree manager: %w", err)
		}

		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}

		ref, err := r.Head()
		if err != nil {
			return fmt.Errorf("failed to get HEAD: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "%-30s %s %s\n", cwd, ref.Hash().String()[:7], refName(ref))

		worktrees, err := w.List()
		if err != nil {
			return fmt.Errorf("failed to list worktrees: %w", err)
		}

		wts, ok := r.Storer.(xstorage.WorktreeStorer)
		if !ok {
			return errors.New("storer does not implement WorktreeStorer")
		}

		commonDir := wts.Filesystem()
		for _, name := range worktrees {
			gitdirPath := filepath.Join(commonDir.Root(), "worktrees", name, "gitdir")
			gitdirData, err := os.ReadFile(gitdirPath)
			if err != nil || len(gitdirData) == 0 {
				continue
			}

			wtPath := filepath.Dir(string(gitdirData[:len(gitdirData)-1]))
			wt := memfs.New()
			err = w.Init(wt, name)
			if err != nil {
				continue
			}

			wtRepo, err := w.Open(wt)
			if err != nil {
				continue
			}

			wtRef, err := wtRepo.Head()
			if err != nil {
				return fmt.Errorf("failed to get HEAD: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "%-30s %s %s\n", wtPath, wtRef.Hash().String()[:7], refName(wtRef))
		}

		return nil
	},
	DisableFlagsInUseLine: true,
}

func refName(ref *plumbing.Reference) string {
	name := ref.Name()
	if name.IsBranch() {
		return fmt.Sprintf("[%s]", name.Short())
	}

	return fmt.Sprintf("(detached %s)", string(ref.Name()))
}

var worktreeRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a linked worktree",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		r, err := git.PlainOpen(".")
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}

		wt, err := worktree.New(r.Storer)
		if err != nil {
			return fmt.Errorf("failed to create worktree manager: %w", err)
		}

		err = wt.Remove(name)
		if err != nil {
			return fmt.Errorf("failed to remove worktree: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Worktree '%s' removed\n", name)

		return nil
	},
	DisableFlagsInUseLine: true,
}
