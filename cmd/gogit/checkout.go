package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(checkoutCmd)
}

var checkoutCmd = &cobra.Command{
	Use:   "checkout <tree-ish> -- <pathspec>...",
	Short: "Restore working tree files from a tree",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		treeish, paths, err := splitCheckoutArgs(cmd, args)
		if err != nil {
			return err
		}

		if treeish != "HEAD" {
			return fmt.Errorf("only HEAD is supported as tree-ish, got %q", treeish)
		}

		r, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}

		w, err := r.Worktree()
		if err != nil {
			return fmt.Errorf("failed to open worktree: %w", err)
		}

		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		root := w.Filesystem().Root()

		var resolved []string

		for _, spec := range paths {
			rel, rerr := resolvePathspec(root, cwd, spec)
			if rerr != nil {
				return rerr
			}

			resolved = append(resolved, rel)
		}

		expanded, err := expandDirectoryPaths(r, resolved)
		if err != nil {
			return err
		}

		if len(expanded) == 0 {
			return errors.New("no matching paths in HEAD")
		}

		return w.Restore(&git.RestoreOptions{
			Staged:   true,
			Worktree: true,
			Files:    expanded,
		})
	},
	DisableFlagsInUseLine: true,
}

// splitCheckoutArgs splits cobra-parsed args into tree-ish and pathspecs.
// Cobra strips the "--" separator but records its position via ArgsLenAtDash.
func splitCheckoutArgs(cmd *cobra.Command, args []string) (string, []string, error) {
	dashAt := cmd.ArgsLenAtDash()
	if dashAt < 0 {
		return "", nil, errors.New("missing -- separator between tree-ish and pathspecs")
	}

	if dashAt == 0 {
		return "", nil, errors.New("missing tree-ish before --")
	}

	if dashAt == len(args) {
		return "", nil, errors.New("missing pathspec after --")
	}

	return args[0], args[dashAt:], nil
}

// expandDirectoryPaths replaces directory entries in paths with all file
// paths from HEAD's tree that live under them. File entries are kept as-is.
func expandDirectoryPaths(r *git.Repository, paths []string) ([]string, error) {
	ref, err := r.Head()
	if err != nil {
		return nil, fmt.Errorf("resolving HEAD: %w", err)
	}

	commit, err := r.CommitObject(ref.Hash())
	if err != nil {
		return nil, err
	}

	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}

	var out []string

	for _, p := range paths {
		if _, err := tree.File(p); err == nil {
			out = append(out, p)

			continue
		}

		// Treat as directory: collect every tree entry whose path starts with p+"/".
		prefix := p + "/"
		if p == "." || p == "" {
			prefix = ""
		}

		walker := object.NewTreeWalker(tree, true, nil)

		for {
			name, entry, werr := walker.Next()
			if werr != nil {
				break
			}

			if !entry.Mode.IsFile() {
				continue
			}

			if prefix == "" || strings.HasPrefix(name, prefix) {
				out = append(out, name)
			}
		}

		walker.Close()
	}

	return out, nil
}
