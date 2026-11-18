package main

import (
	"fmt"
	"path/filepath"
	"strings"

	internalsubmodule "github.com/go-git/cli/internal/submodule"
	"github.com/go-git/go-git/v6"
	"github.com/spf13/cobra"
)

var submoduleUpdateInit bool

func init() {
	submoduleUpdateCmd.Flags().BoolVar(&submoduleUpdateInit, "init", false, "Initialise uninitialised submodules before updating")

	submoduleCmd.AddCommand(submoduleStatusCmd)
	submoduleCmd.AddCommand(submoduleInitCmd)
	submoduleCmd.AddCommand(submoduleUpdateCmd)
	submoduleCmd.AddCommand(submoduleAddCmd)
	rootCmd.AddCommand(submoduleCmd)
}

var submoduleCmd = &cobra.Command{
	Use:   "submodule <command>",
	Short: "Initialise, update, or inspect submodules",
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Usage()
	},
	DisableFlagsInUseLine: true,
}

var submoduleStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the status of submodules",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		w, err := openWorktree()
		if err != nil {
			return err
		}

		statuses, err := w.Submodules()
		if err != nil {
			return fmt.Errorf("read submodules: %w", err)
		}

		s, err := statuses.Status()
		if err != nil {
			return fmt.Errorf("submodule status: %w", err)
		}

		// Status' String() already emits "<flag><sha1> <path>" lines, the
		// upstream-shape format.
		fmt.Fprint(cmd.OutOrStdout(), s.String())

		return nil
	},
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}

var submoduleInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialise submodules recorded in .gitmodules into the local config",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		w, err := openWorktree()
		if err != nil {
			return err
		}

		subs, err := w.Submodules()
		if err != nil {
			return fmt.Errorf("read submodules: %w", err)
		}

		return subs.Init()
	},
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}

var submoduleUpdateCmd = &cobra.Command{
	Use:   "update [--init]",
	Short: "Update submodules to the recorded commit",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		w, err := openWorktree()
		if err != nil {
			return err
		}

		subs, err := w.Submodules()
		if err != nil {
			return fmt.Errorf("read submodules: %w", err)
		}

		return subs.Update(&git.SubmoduleUpdateOptions{Init: submoduleUpdateInit})
	},
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}

var submoduleAddCmd = &cobra.Command{
	Use:   "add <repository> [<path>]",
	Short: "Add the given repository as a submodule",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		parent, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
		if err != nil {
			return fmt.Errorf("open parent repository: %w", err)
		}

		defer parent.Close()

		relPath := args[0]
		if len(args) == 2 {
			relPath = args[1]
		} else {
			relPath = filepath.Base(strings.TrimSuffix(relPath, ".git"))
		}

		cloneURL := resolveCloneURL(args[0])

		if _, err := internalsubmodule.Add(parent, cloneURL, relPath); err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Adding existing repo at '%s' to the index\n", relPath)

		return nil
	},
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}

// openWorktree opens the repository found relative to the cwd and returns
// its worktree. It's the shape several submodule subcommands need; using a
// helper here keeps each RunE one line shorter than open-then-Worktree.
func openWorktree() (*git.Worktree, error) {
	r, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil, fmt.Errorf("open repository: %w", err)
	}

	w, err := r.Worktree()
	if err != nil {
		return nil, fmt.Errorf("open worktree: %w", err)
	}

	return w, nil
}
