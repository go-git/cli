package main

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/spf13/cobra"
)

// Note: go-git's PlainCloneContext calls checkTargetDirIsEmpty before
// initialising the clone, so the non-empty / non-directory target guard
// is upstream. That check uses osfs.Default (rooted at "/"), so it only
// fires reliably when given an absolute path — the gogit wrapper below
// resolves the destination via filepath.Abs before handing it off.

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

		repoURL := resolveCloneURL(args[0])

		ep, err := url.Parse(repoURL)
		if err != nil {
			return err
		}

		opts := git.CloneOptions{
			URL:           repoURL,
			Depth:         cloneDepth,
			ClientOptions: defaultClientOptions(ep),
			Bare:          cloneBare,
		}

		if cloneTags {
			opts.Tags = git.TagFollowing
		}

		if cloneProgress {
			opts.Progress = cmd.OutOrStdout()
		}

		fmt.Fprintf(cmd.ErrOrStderr(), "Cloning into '%s'...\n", dir)

		absDir, err := filepath.Abs(dir)
		if err != nil {
			return err
		}

		_, err = git.PlainClone(absDir, &opts)

		return err
	},
	DisableFlagsInUseLine: true,
}

// resolveCloneURL accepts a clone target as a user typed it and returns a form
// that go-git's PlainClone can dereference. Bare local paths (relative or
// absolute) are pointed at the directory they name on disk; scp-like refs
// (host:path) and explicit schemes (file://, https://, ssh://, git://) pass
// through unchanged.
func resolveCloneURL(arg string) string {
	if hasURLScheme(arg) || isScpLike(arg) {
		return arg
	}

	abs, err := filepath.Abs(arg)
	if err != nil {
		return arg
	}

	if _, err := os.Stat(abs); err != nil {
		return arg
	}

	return abs
}

// hasURLScheme reports whether arg begins with a recognised URL scheme. We
// match the same set go-git's transport routing recognises.
func hasURLScheme(arg string) bool {
	for _, scheme := range []string{"file://", "http://", "https://", "ssh://", "git://"} {
		if strings.HasPrefix(arg, scheme) {
			return true
		}
	}

	return false
}

// isScpLike reports whether arg looks like `[user@]host:path` — the SSH
// shorthand that has no scheme but is not a local filesystem path. The rule
// matches upstream Git: a `:` must appear before any `/`.
func isScpLike(arg string) bool {
	colon := strings.IndexByte(arg, ':')
	if colon < 0 {
		return false
	}

	slash := strings.IndexByte(arg, '/')

	return slash < 0 || colon < slash
}
