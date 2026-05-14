package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/format/index"
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
		return runSubmoduleAdd(cmd, args)
	},
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}

// runSubmoduleAdd implements `gogit submodule add <repo> [<path>]`:
// resolve the URL (local paths get expanded to absolute), clone the
// submodule into <path>, write a `.gitmodules` entry, then stage the
// submodule path in the parent's index as a gitlink (mode 160000 with
// the submodule's HEAD as the hash). go-git's Worktree.Add doesn't
// gitlink-stage, so the index entry is written directly via
// Storer.Index / SetIndex.
func runSubmoduleAdd(cmd *cobra.Command, args []string) error {
	repoArg := args[0]

	parent, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return fmt.Errorf("open parent repository: %w", err)
	}

	defer parent.Close()

	parentWT, err := parent.Worktree()
	if err != nil {
		return fmt.Errorf("open parent worktree: %w", err)
	}

	parentRoot := parentWT.Filesystem().Root()

	relPath := args[0]
	if len(args) == 2 {
		relPath = args[1]
	} else {
		relPath = filepath.Base(strings.TrimSuffix(relPath, ".git"))
	}

	relPath = filepath.ToSlash(filepath.Clean(relPath))

	if filepath.IsAbs(relPath) || strings.HasPrefix(relPath, "../") || relPath == ".." {
		return fmt.Errorf("submodule add: path %q must be inside the repository", relPath)
	}

	cloneURL := resolveCloneURL(repoArg)
	cloneTarget := filepath.Join(parentRoot, relPath)

	if err := ensureCloneTargetAvailable(cloneTarget); err != nil {
		return err
	}

	sub, err := git.PlainClone(cloneTarget, &git.CloneOptions{URL: cloneURL})
	if err != nil {
		return fmt.Errorf("submodule add: clone %q: %w", repoArg, err)
	}

	head, err := sub.Head()
	if err != nil {
		return fmt.Errorf("submodule add: read clone HEAD: %w", err)
	}

	if err := upsertGitmodulesEntry(parentRoot, relPath, cloneURL); err != nil {
		return err
	}

	if _, err := parentWT.Add(".gitmodules"); err != nil {
		return fmt.Errorf("submodule add: stage .gitmodules: %w", err)
	}

	if err := stageGitlinkEntry(parent, relPath, head.Hash()); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Adding existing repo at '%s' to the index\n", relPath)

	return nil
}

// upsertGitmodulesEntry creates or updates a single `[submodule "<path>"]`
// section in <repoRoot>/.gitmodules. The submodule's name matches its path
// — upstream's default and what go-git's Submodules() expects.
func upsertGitmodulesEntry(repoRoot, path, url string) error {
	gitmodulesPath := filepath.Join(repoRoot, ".gitmodules")

	modules := config.NewModules()

	if data, err := os.ReadFile(gitmodulesPath); err == nil {
		if err := modules.Unmarshal(data); err != nil {
			return fmt.Errorf("parse existing .gitmodules: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read .gitmodules: %w", err)
	}

	modules.Submodules[path] = &config.Submodule{Name: path, Path: path, URL: url}

	out, err := modules.Marshal()
	if err != nil {
		return fmt.Errorf("marshal .gitmodules: %w", err)
	}

	return os.WriteFile(gitmodulesPath, out, 0o644)
}

// stageGitlinkEntry inserts (or replaces) an index entry at `path` with
// mode 160000 (gitlink) pointing at `hash`. The replace-or-append logic
// keeps the index ordered enough for go-git's encoder, which sorts on
// write.
func stageGitlinkEntry(repo *git.Repository, path string, hash plumbing.Hash) error {
	idx, err := repo.Storer.Index()
	if err != nil {
		return fmt.Errorf("read index: %w", err)
	}

	replaced := false

	for i, e := range idx.Entries {
		if e.Name == path && e.Stage == index.Merged {
			idx.Entries[i] = &index.Entry{
				Hash: hash,
				Name: path,
				Mode: filemode.Submodule,
			}
			replaced = true

			break
		}
	}

	if !replaced {
		idx.Entries = append(idx.Entries, &index.Entry{
			Hash: hash,
			Name: path,
			Mode: filemode.Submodule,
		})
	}

	sort.Slice(idx.Entries, func(i, j int) bool {
		return idx.Entries[i].Name < idx.Entries[j].Name
	})

	return repo.Storer.SetIndex(idx)
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
