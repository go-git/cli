package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/format/index"
	"github.com/spf13/cobra"
)

var (
	lsFilesStage        bool
	lsFilesErrorUnmatch bool
)

func init() {
	lsFilesCmd.Flags().BoolVarP(&lsFilesStage, "stage", "s", false, "Show staged contents' mode, object hash, and stage")
	lsFilesCmd.Flags().BoolVar(&lsFilesErrorUnmatch, "error-unmatch", false, "Exit with an error if a given pathspec does not match any file in the index")
	rootCmd.AddCommand(lsFilesCmd)
}

var lsFilesCmd = &cobra.Command{
	Use:   "ls-files [<options>] [--] [<pathspec>...]",
	Short: "Show information about files in the index and the working tree",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}

		defer r.Close()

		idx, err := r.Storer.Index()
		if err != nil {
			return fmt.Errorf("read index: %w", err)
		}

		specs := make([]string, len(args))
		matched := make([]bool, len(args))

		for i, spec := range args {
			specs[i] = strings.TrimSuffix(spec, "/")
		}

		var selected []*index.Entry

		for _, e := range idx.Entries {
			if len(specs) == 0 {
				selected = append(selected, e)
				continue
			}

			for i, spec := range specs {
				if pathMatchesSpec(e.Name, spec) {
					matched[i] = true

					selected = append(selected, e)

					break
				}
			}
		}

		if lsFilesErrorUnmatch {
			for i, m := range matched {
				if !m {
					return fmt.Errorf("error: pathspec %q did not match any file(s) known to git", args[i])
				}
			}
		}

		sort.Slice(selected, func(i, j int) bool { return selected[i].Name < selected[j].Name })

		for _, e := range selected {
			if lsFilesStage {
				fmt.Fprintf(cmd.OutOrStdout(), "%06o %s %d\t%s\n", uint32(e.Mode), e.Hash, e.Stage, e.Name)
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), e.Name)
			}
		}

		return nil
	},
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}

// pathMatchesSpec reports whether path is selected by spec. A spec matches a
// path that equals it or that is rooted beneath it (treating the spec as a
// directory). Specs are taken as literal strings — no glob interpretation —
// which suits gogit's current pathspec needs.
func pathMatchesSpec(path, spec string) bool {
	return path == spec || strings.HasPrefix(path, spec+"/")
}
