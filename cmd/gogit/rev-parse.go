package main

import (
	"fmt"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	formatcfg "github.com/go-git/go-git/v6/plumbing/format/config"
	"github.com/spf13/cobra"
)

var revParseShowObjectFormat bool

func init() {
	revParseCmd.Flags().BoolVar(&revParseShowObjectFormat, "show-object-format", false, "Show the object format (hash algorithm) in use for the repository")
	rootCmd.AddCommand(revParseCmd)
}

var revParseCmd = &cobra.Command{
	Use:   "rev-parse <rev>...",
	Short: "Pick out and massage parameters",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}

		defer r.Close()

		if revParseShowObjectFormat {
			cfg, err := r.Config()
			if err != nil {
				return fmt.Errorf("read config: %w", err)
			}

			// formatcfg.UnsetObjectFormat ("") is the zero value when
			// extensions.objectformat is absent, which means SHA1.
			of := cfg.Extensions.ObjectFormat
			if of == formatcfg.UnsetObjectFormat {
				of = formatcfg.SHA1
			}

			fmt.Fprintln(cmd.OutOrStdout(), of)

			return nil
		}

		if len(args) == 0 {
			return fmt.Errorf("rev-parse: no revisions and no --show-* flag")
		}

		for _, rev := range args {
			h, err := r.ResolveRevision(plumbing.Revision(rev))
			if err != nil {
				return fmt.Errorf("resolve %s: %w", rev, err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), h)
		}

		return nil
	},
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}
