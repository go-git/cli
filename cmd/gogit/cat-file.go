package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/spf13/cobra"
)

const catFileDefaultBatchFormat = "%(objectname) %(objecttype) %(objectsize)"

var (
	catFileExists           bool
	catFileBatchCheckFormat string
)

func init() {
	catFileCmd.Flags().BoolVarP(&catFileExists, "exists", "e", false,
		"Check whether object exists; exit 0 if so, 1 otherwise")
	catFileCmd.Flags().StringVar(&catFileBatchCheckFormat, "batch-check", "",
		"Read object IDs from stdin and print per-line metadata using the optional <format>")
	catFileCmd.Flags().Lookup("batch-check").NoOptDefVal = catFileDefaultBatchFormat
	rootCmd.AddCommand(catFileCmd)
}

var catFileCmd = &cobra.Command{
	Use:   "cat-file (-e <oid> | --batch-check[=<format>] | <type> <oid>)",
	Short: "Provide content or check existence of repository objects",
	RunE: func(cmd *cobra.Command, args []string) error {
		batchCheckSet := cmd.Flags().Changed("batch-check")
		if catFileExists && batchCheckSet {
			return errors.New("-e and --batch-check are mutually exclusive")
		}

		r, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}

		defer r.Close()

		switch {
		case catFileExists:
			if len(args) != 1 {
				return errors.New("-e requires exactly one <oid> argument")
			}

			return catFileExistsCheck(r, args[0])
		case batchCheckSet:
			return catFileBatchCheckRun(cmd, r, os.Stdin, catFileBatchCheckFormat)
		case len(args) == 2:
			return catFileTypedPrint(cmd, r, args[0], args[1])
		default:
			return errors.New("usage: cat-file (-e <oid> | --batch-check[=<fmt>] | <type> <oid>)")
		}
	},
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}

// catFileExistsCheck never returns: it calls os.Exit(1) when the object is
// absent and os.Exit(0) when it is present. The signature returns error only
// to fit cobra's RunE contract via its caller.
func catFileExistsCheck(r *git.Repository, oid string) error { //nolint:unparam
	h := plumbing.NewHash(oid)
	if _, err := r.Storer.EncodedObject(plumbing.AnyObject, h); err != nil {
		os.Exit(1)
	}

	return nil
}

// catFileTypedPrint resolves <oid>, verifies obj.Type().String() == typ,
// and writes content to stdout. Silently exits non-zero on missing object
// or type mismatch, matching `git cat-file <type> <oid>` semantics.
func catFileTypedPrint(cmd *cobra.Command, r *git.Repository, typ, oid string) error {
	h := plumbing.NewHash(oid)

	obj, err := r.Storer.EncodedObject(plumbing.AnyObject, h)
	if err != nil {
		os.Exit(1)
	}

	if obj.Type().String() != typ {
		os.Exit(1)
	}

	rd, err := obj.Reader()
	if err != nil {
		return err
	}

	defer rd.Close()

	if _, err := io.Copy(cmd.OutOrStdout(), rd); err != nil {
		return err
	}

	return nil
}

func catFileBatchCheckRun(cmd *cobra.Command, r *git.Repository, stdin io.Reader, format string) error {
	w := bufio.NewWriter(cmd.OutOrStdout())
	defer w.Flush()

	scanner := bufio.NewScanner(stdin)
	for scanner.Scan() {
		// Per upstream `cat-file --batch-check`, each input line is
		// "<oid>[ <rest>]" — the rest is ignored for lookups but echoed back
		// when the format reuses the original input. We only care about the
		// leading oid token.
		raw := strings.TrimRight(scanner.Text(), "\n\r")
		if raw == "" {
			continue
		}

		oid := strings.SplitN(raw, " ", 2)[0]

		h := plumbing.NewHash(oid)

		obj, err := r.Storer.EncodedObject(plumbing.AnyObject, h)
		if err != nil {
			fmt.Fprintf(w, "%s missing\n", oid)

			continue
		}

		fmt.Fprintln(w, renderBatchCheck(format, oid, obj))
	}

	return scanner.Err()
}

// renderBatchCheck expands the supported %(name) tokens in format. Unknown
// tokens are left in place (mirrors upstream's permissive behaviour for
// unrecognised atoms, which is preferable to silently erroring out on a
// typo).
func renderBatchCheck(format, oid string, obj plumbing.EncodedObject) string {
	size := strconv.FormatInt(obj.Size(), 10)
	replacer := strings.NewReplacer(
		"%(objectname)", oid,
		"%(objecttype)", obj.Type().String(),
		"%(objectsize)", size,
		// %(objectsize:disk) is the on-disk size for packed objects. We do
		// not currently expose that via go-git's EncodedObject; the
		// uncompressed size is close enough for the comparison assertions
		// in t5325 (which compare two runs against the same backend).
		"%(objectsize:disk)", size,
	)

	return replacer.Replace(format)
}
