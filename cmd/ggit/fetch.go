package main

import (
	"errors"
	"math"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/spf13/cobra"
)

var (
	fetchProgress  bool
	fetchDepth     int
	fetchUnshallow bool
)

func init() {
	fetchCmd.Flags().BoolVarP(&fetchProgress, "progress", "", true, "Show fetch progress")
	fetchCmd.Flags().IntVarP(&fetchDepth, "depth", "", 0, "Create a shallow fetch of that depth")
	fetchCmd.Flags().BoolVarP(&fetchUnshallow, "unshallow", "", false, "Convert a shallow repository to a complete one")

	rootCmd.AddCommand(fetchCmd)
	rootCmd.CompletionOptions.HiddenDefaultCmd = true
}

var fetchCmd = &cobra.Command{
	Use:   "fetch [<options>] [--] [<repository> [<refspec>...]]",
	Short: "Download objects and refs from another repository",
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := git.PlainOpen(".")
		if err != nil {
			return err
		}

		remote, err := r.Remote("origin")
		if err != nil {
			return err
		}

		ep, err := transport.NewEndpoint(remote.Config().URLs[0])
		if err != nil {
			return err
		}

		if fetchUnshallow {
			fetchDepth = math.MaxInt32
		}

		opts := git.FetchOptions{
			Depth: fetchDepth,
			Auth:  defaultAuth(ep),
		}

		if fetchProgress {
			opts.Progress = cmd.OutOrStdout()
		}

		err = remote.Fetch(&opts)
		if errors.Is(err, git.NoErrAlreadyUpToDate) {
			return nil
		}

		return err
	},
}
