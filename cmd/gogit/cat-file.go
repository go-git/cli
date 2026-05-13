package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/spf13/cobra"
)

var (
	catFileExists     bool
	catFileBatchCheck bool
)

func init() {
	catFileCmd.Flags().BoolVarP(&catFileExists, "exists", "e", false,
		"Check whether object exists; exit 0 if so, 1 otherwise")
	catFileCmd.Flags().BoolVar(&catFileBatchCheck, "batch-check", false,
		"Read object IDs from stdin and print <oid> <type> <size> per line (or '<oid> missing')")
	rootCmd.AddCommand(catFileCmd)
}

var catFileCmd = &cobra.Command{
	Use:   "cat-file (-e <oid> | --batch-check)",
	Short: "Provide content or check existence of repository objects",
	RunE: func(cmd *cobra.Command, args []string) error {
		if catFileExists && catFileBatchCheck {
			return errors.New("-e and --batch-check are mutually exclusive")
		}

		if !catFileExists && !catFileBatchCheck {
			return errors.New("one of -e or --batch-check is required")
		}

		r, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}

		if catFileExists {
			if len(args) != 1 {
				return errors.New("-e requires exactly one <oid> argument")
			}

			return catFileExistsCheck(r, args[0])
		}

		return catFileBatchCheckRun(cmd, r, os.Stdin)
	},
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}

func catFileExistsCheck(r *git.Repository, oid string) error {
	h := plumbing.NewHash(oid)
	if _, err := r.Storer.EncodedObject(plumbing.AnyObject, h); err != nil {
		os.Exit(1)
	}

	return nil
}

func catFileBatchCheckRun(cmd *cobra.Command, r *git.Repository, stdin io.Reader) error {
	w := bufio.NewWriter(cmd.OutOrStdout())
	defer w.Flush()

	scanner := bufio.NewScanner(stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		h := plumbing.NewHash(line)

		obj, err := r.Storer.EncodedObject(plumbing.AnyObject, h)
		if err != nil {
			fmt.Fprintf(w, "%s missing\n", line)

			continue
		}

		fmt.Fprintf(w, "%s %s %d\n", line, obj.Type(), obj.Size())
	}

	return scanner.Err()
}
