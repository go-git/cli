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

		if err := ensureCloneTargetAvailable(dir); err != nil {
			return err
		}

		_, err = git.PlainClone(dir, &opts)

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

// ensureCloneTargetAvailable matches upstream's pre-clone check: the target
// must not already be a non-empty directory, and must not be a non-directory
// path (e.g. an existing file). go-git's PlainClone will happily merge a
// clone into a populated directory, which lets clones silently overwrite
// unrelated content.
func ensureCloneTargetAvailable(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return err
	}

	if !info.IsDir() {
		return fmt.Errorf("fatal: destination path %q already exists and is not an empty directory", dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	if len(entries) > 0 {
		return fmt.Errorf("fatal: destination path %q already exists and is not an empty directory", dir)
	}

	return nil
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
