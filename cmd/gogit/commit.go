package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/spf13/cobra"
)

// errNoIdentityEnv is returned by signatureFromEnv when none of the identity
// environment variables are set, so the caller can fall back to git config.
var errNoIdentityEnv = errors.New("no identity environment variables set")

var commitMessage string

func init() {
	commitCmd.Flags().StringVarP(&commitMessage, "message", "m", "", "Commit message")
	_ = commitCmd.MarkFlagRequired("message")
	rootCmd.AddCommand(commitCmd)
}

var commitCmd = &cobra.Command{
	Use:   "commit -m <message>",
	Short: "Record changes to the repository",
	RunE: func(cmd *cobra.Command, _ []string) error {
		r, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}

		defer r.Close()

		w, err := r.Worktree()
		if err != nil {
			return fmt.Errorf("failed to open worktree: %w", err)
		}

		opts := &git.CommitOptions{}

		author, err := signatureFromEnv("GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_AUTHOR_DATE")
		if err != nil && !errors.Is(err, errNoIdentityEnv) {
			return err
		}

		opts.Author = author

		committer, err := signatureFromEnv("GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL", "GIT_COMMITTER_DATE")
		if err != nil && !errors.Is(err, errNoIdentityEnv) {
			return err
		}

		opts.Committer = committer

		if _, err := w.Commit(commitMessage, opts); err != nil {
			return fmt.Errorf("commit failed: %w", err)
		}

		return nil
	},
	DisableFlagsInUseLine: true,
}

// signatureFromEnv builds an object.Signature from the given environment
// variable names. Returns errNoIdentityEnv if none of the variables are set.
func signatureFromEnv(nameVar, emailVar, dateVar string) (*object.Signature, error) {
	name := os.Getenv(nameVar)
	email := os.Getenv(emailVar)
	date := os.Getenv(dateVar)

	if name == "" && email == "" && date == "" {
		return nil, errNoIdentityEnv
	}

	sig := &object.Signature{Name: name, Email: email, When: time.Now()}

	if date != "" {
		t, err := parseGitDate(date)
		if err != nil {
			return nil, fmt.Errorf("invalid %s=%q: %w", dateVar, date, err)
		}

		sig.When = t
	}

	return sig, nil
}

// parseGitDate parses the "<unix-seconds> <±HHMM>" format used by GIT_*_DATE.
func parseGitDate(s string) (time.Time, error) {
	var secs int64

	var zone string

	if _, err := fmt.Sscanf(s, "%d %s", &secs, &zone); err != nil {
		return time.Time{}, err
	}

	// Defer the sign/range parsing to time.Parse with the canonical "-0700"
	// layout: it validates length and digits and handles both positive and
	// negative offsets correctly. The reference value's date components are
	// irrelevant — we only consume the resulting zone offset.
	zt, err := time.Parse("-0700", zone)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid zone %q: %w", zone, err)
	}

	_, offset := zt.Zone()

	return time.Unix(secs, 0).In(time.FixedZone(zone, offset)), nil
}
