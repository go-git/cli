package main

import (
	"path"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/spf13/cobra"
)

var (
	cloneBare     bool
	cloneProgress bool
	cloneDepth    int
)

func init() {
	cloneCmd.Flags().BoolVarP(&cloneBare, "bare", "", false, "Create a bare repository")
	cloneCmd.Flags().BoolVarP(&cloneProgress, "progress", "", true, "Show clone progress")
	cloneCmd.Flags().IntVarP(&cloneDepth, "depth", "", 0, "Create a shallow clone of that depth")
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
		}

		ep, err := transport.NewEndpoint(args[0])
		if err != nil {
			return err
		}

		opts := git.CloneOptions{
			URL:   args[0],
			Depth: cloneDepth,
			Auth:  defaultAuth(ep),
		}

		if cloneProgress {
			opts.Progress = cmd.OutOrStdout()
		}

		_, err = git.PlainClone(dir, cloneBare, &opts)
		return err
	},
	DisableFlagsInUseLine: true,
}
