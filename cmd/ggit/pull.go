package main

import (
	"errors"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/spf13/cobra"
)

var pullProgress bool

var pullCmd = &cobra.Command{
	Use:   "pull [<options>] [<repo> [<refspec>...]]",
	Short: "Fetch from and integrate with another repository or a local branch",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}

		repo, err := git.PlainOpen(dir)
		if err != nil {
			return err
		}

		w, err := repo.Worktree()
		if err != nil {
			return err
		}

		cfg, err := repo.Config()
		if err != nil {
			return err
		}

		ep, err := transport.NewEndpoint(cfg.Remotes["origin"].URLs[0])
		if err != nil {
			return err
		}

		opts := git.PullOptions{
			Auth: defaultAuth(ep),
		}
		if pullProgress {
			opts.Progress = cmd.OutOrStdout()
		}

		err = w.Pull(&opts)
		switch {
		case errors.Is(err, git.NoErrAlreadyUpToDate):
			cmd.Println("Already up-to-date.")
			return nil
		default:
			return err
		}
	},
	DisableFlagsInUseLine: true,
}

func init() {
	pullCmd.Flags().BoolVarP(&pullProgress, "progress", "", true, "Show pull progress")
	rootCmd.AddCommand(pullCmd)
}
