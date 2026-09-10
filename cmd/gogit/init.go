package main

import (
	"fmt"
	"os"

	"github.com/go-git/go-git/v6"
	formatcfg "github.com/go-git/go-git/v6/plumbing/format/config"
	"github.com/spf13/cobra"
)

var (
	initTemplate     string
	initObjectFormat string
	initQuiet        bool
)

func init() {
	initCmd.Flags().StringVar(&initTemplate, "template", "", "Template directory (accepted for compatibility, ignored)")
	initCmd.Flags().StringVar(&initObjectFormat, "object-format", "", "Object hash algorithm: sha1 or sha256")
	initCmd.Flags().BoolVarP(&initQuiet, "quiet", "q", false, "Suppress all output except errors")
	rootCmd.AddCommand(initCmd)
}

var initCmd = &cobra.Command{
	Use:   "init [<directory>]",
	Short: "Create an empty Git repository",
	Args:  cobra.RangeArgs(0, 1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "."
		if len(args) == 1 {
			dir = args[0]
		}

		format, err := resolveInitObjectFormat(initObjectFormat, os.Getenv("GIT_DEFAULT_HASH"))
		if err != nil {
			return err
		}

		if _, err := git.PlainInit(dir, false, git.WithObjectFormat(format)); err != nil {
			return fmt.Errorf("failed to init repository: %w", err)
		}

		if !initQuiet {
			fmt.Fprintf(cmd.OutOrStdout(), "Initialized empty Git repository in %s\n", dir)
		}

		return nil
	},
	DisableFlagsInUseLine: true,
}

// resolveInitObjectFormat picks the hash algorithm for `gogit init`, applying
// upstream's resolution order: --object-format flag wins, then GIT_DEFAULT_HASH,
// then the sha1 default. An unrecognised value is an error.
func resolveInitObjectFormat(flag, env string) (formatcfg.ObjectFormat, error) {
	for _, v := range []string{flag, env} {
		switch v {
		case "":
			continue
		case "sha1":
			return formatcfg.SHA1, nil
		case "sha256":
			return formatcfg.SHA256, nil
		default:
			return formatcfg.SHA1, fmt.Errorf("unknown hash algorithm %q", v)
		}
	}

	return formatcfg.SHA1, nil
}
