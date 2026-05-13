package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

var (
	diffNoIndex       bool
	diffIgnoreCRAtEol bool
)

func init() {
	diffCmd.Flags().BoolVar(&diffNoIndex, "no-index", false, "Compare two files outside a git repository")
	diffCmd.Flags().BoolVar(&diffIgnoreCRAtEol, "ignore-cr-at-eol", false,
		"Ignore carriage-returns at the end of line when comparing")
	rootCmd.AddCommand(diffCmd)
}

// diff is a minimal stand-in for `git diff` providing only the surface used
// by test-lib.sh's GIT_TEST_CMP override on Windows: --no-index file/file
// comparison with optional CRLF tolerance. Exit code 0 means equal, 1 means
// different. Differences are printed in unified-diff style.
var diffCmd = &cobra.Command{
	Use:   "diff [--no-index] [--ignore-cr-at-eol] -- <file1> <file2>",
	Short: "Show changes between files",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !diffNoIndex {
			return errors.New("only --no-index mode is supported")
		}

		a, err := readDiffInput(args[0], diffIgnoreCRAtEol)
		if err != nil {
			return err
		}

		b, err := readDiffInput(args[1], diffIgnoreCRAtEol)
		if err != nil {
			return err
		}

		if bytes.Equal(a, b) {
			return nil
		}

		fmt.Fprintf(cmd.OutOrStdout(), "--- %s\n+++ %s\n", args[0], args[1])
		emitNaiveDiff(cmd.OutOrStdout(), a, b)

		os.Exit(1)

		return nil
	},
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}

func readDiffInput(path string, ignoreCRAtEol bool) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	if ignoreCRAtEol {
		data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	}

	return data, nil
}

// emitNaiveDiff writes a per-line diff sufficient for test_cmp diagnostics.
// Not a true LCS diff, just a side-by-side dump.
func emitNaiveDiff(w io.Writer, a, b []byte) {
	aLines := splitLines(a)
	bLines := splitLines(b)

	for _, line := range aLines {
		fmt.Fprintf(w, "-%s\n", line)
	}

	for _, line := range bLines {
		fmt.Fprintf(w, "+%s\n", line)
	}
}

func splitLines(b []byte) []string {
	var out []string

	scanner := bufio.NewScanner(bytes.NewReader(b))
	for scanner.Scan() {
		out = append(out, scanner.Text())
	}

	return out
}
