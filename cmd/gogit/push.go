package main

import (
	"errors"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing/transport"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	pushProgress bool
	pushPrune    bool
	pushQuiet    bool
	pushForce    bool
)

func init() {
	pushCmd.Flags().BoolVarP(&pushQuiet, "quiet", "q", false, "Suppress all output unless an error occurs")
	pushCmd.Flags().BoolVarP(&pushProgress, "progress", "", true, "Force show push progress")
	pushCmd.Flags().BoolVarP(&pushPrune, "prune", "", false, "Prune remote branches")
	pushCmd.Flags().BoolVarP(&pushForce, "force", "f", false, "Force push")

	rootCmd.AddCommand(pushCmd)
	rootCmd.CompletionOptions.HiddenDefaultCmd = true
}

var pushCmd = &cobra.Command{
	Use:   "push [<options>] [--] [<repository> [<refspec>...]]",
	Short: "Update remote refs along with associated objects",
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := git.PlainOpen(".")
		if err != nil {
			return err
		}

		var ep *transport.Endpoint
		remoteName := git.DefaultRemoteName
		if len(args) > 0 {
			ep, err = transport.NewEndpoint(args[0])
			if err != nil {
				// We have a remote name
				remoteName = args[0]
			}
		}

		var refspecs []config.RefSpec
		if len(args) > 1 {
			for _, arg := range args[1:] {
				refspecs = append(refspecs, config.RefSpec(arg))
			}
		}

		remote, err := r.Remote(remoteName)
		if err != nil {
			return err
		}

		if ep == nil {
			// Use the default remote URL
			urln := len(remote.Config().URLs) - 1
			if urln < 0 {
				return errors.New("no remote URLs")
			}

			ep, err = transport.NewEndpoint(remote.Config().URLs[urln])
			if err != nil {
				return err
			}
		}

		opts := git.PushOptions{
			Auth:       defaultAuth(ep),
			RemoteName: remoteName,
			RefSpecs:   refspecs,
			Prune:      pushPrune,
			Force:      pushForce,
		}

		var isatty bool
		stderr := cmd.ErrOrStderr()
		if f, ok := stderr.(interface {
			Fd() uintptr
		}); ok {
			isatty = term.IsTerminal(int(f.Fd()))
		}

		if !pushQuiet && (isatty || pushProgress) {
			opts.Progress = cmd.ErrOrStderr()
		}

		err = remote.Push(&opts)
		if errors.Is(err, git.NoErrAlreadyUpToDate) {
			cmd.PrintErr("Everything up-to-date")
			return nil
		}

		return err
	},
}
