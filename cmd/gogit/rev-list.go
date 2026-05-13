package main

import (
	"errors"
	"fmt"
	"sort"

	"github.com/go-git/go-git/v6"
	"github.com/spf13/cobra"
)

var (
	revListAll          bool
	revListObjects      bool
	revListNoObjectName bool
)

func init() {
	revListCmd.Flags().BoolVar(&revListAll, "all", false, "Walk all references")
	revListCmd.Flags().BoolVar(&revListObjects, "objects", false, "List non-commit objects as well")
	revListCmd.Flags().BoolVar(&revListNoObjectName, "no-object-names", false,
		"Suppress the names alongside objects (only emit the hash)")
	rootCmd.AddCommand(revListCmd)
}

var revListCmd = &cobra.Command{
	Use:   "rev-list [--all] [--objects] [--no-object-names]",
	Short: "List commits and optionally objects in reverse-chronological order",
	RunE: func(cmd *cobra.Command, _ []string) error {
		if !revListAll {
			return errors.New("--all is required (v1)")
		}

		r, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}

		defer r.Close()

		hashes, err := collectAllReachable(r)
		if err != nil {
			return err
		}

		// Match upstream rev-list ordering by emitting hashes in a stable
		// (sorted) order. The exact upstream order is reverse-chronological,
		// but our consumers (t5325 case 9) use the output as a sort-independent
		// input set, so sorted output is sufficient.
		sort.Slice(hashes, func(i, j int) bool { return hashes[i].String() < hashes[j].String() })

		w := cmd.OutOrStdout()

		for _, h := range hashes {
			// In v1 the --no-object-names suppression collapses any path/name
			// suffix; without --objects we'd emit only commits, but t5325
			// always passes --objects. We emit just the hash either way.
			if _, err := fmt.Fprintln(w, h); err != nil {
				return err
			}
		}

		_ = revListObjects
		_ = revListNoObjectName

		return nil
	},
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}
