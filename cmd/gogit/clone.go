package main

import (
	"fmt"
	"path"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/transport"
	"github.com/spf13/cobra"
)

var (
	cloneBare     bool
	cloneProgress bool
	cloneDepth    int
	cloneTags     bool
)

func init() {
	cloneCmd.Flags().BoolVarP(&cloneBare, "bare", "", false, "Create a bare repository")
	cloneCmd.Flags().BoolVarP(&cloneProgress, "progress", "", true, "Show clone progress")
	cloneCmd.Flags().IntVarP(&cloneDepth, "depth", "", 0, "Create a shallow clone of that depth")
	cloneCmd.Flags().BoolVarP(&cloneTags, "tags", "", false, "Clone tags")
	rootCmd.AddCommand(cloneCmd)
	rootCmd.CompletionOptions.HiddenDefaultCmd = true
}

var cloneCmd = &cobra.Command{
	Use:   "clone [<options>] [--] <repo> [<dir>]",
	Short: "Clone a repository into a new directory",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := path.Base(args[0])
		if len(args) > 1 {
			dir = args[1]
		} else {
			dir = strings.TrimSuffix(dir, ".git")
			if cloneBare {
				dir = dir + ".git"
			}
		}

		ep, err := transport.NewEndpoint(args[0])
		if err != nil {
			return err
		}

		opts := git.CloneOptions{
			URL:   args[0],
			Depth: cloneDepth,
			Auth:  defaultAuth(ep),
			Bare:  cloneBare,
		}

		if cloneTags {
			opts.Tags = git.TagFollowing
		}
		if cloneProgress {
			opts.Progress = cmd.OutOrStdout()
		}

		fmt.Fprintf(cmd.ErrOrStderr(), "Cloning into '%s'...\n", dir)

		_, err = git.PlainClone(dir, &opts)

		return err
	},
	DisableFlagsInUseLine: true,
}
