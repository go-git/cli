package main

import (
	"crypto"
	"errors"
	"fmt"
	"io"

	"github.com/go-git/go-git/v6/plumbing/format/idxfile"
	"github.com/go-git/go-git/v6/plumbing/hash"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(showIndexCmd)
}

var showIndexCmd = &cobra.Command{
	Use:                   "show-index",
	Short:                 "Show packed archive index",
	Args:                  cobra.NoArgs,
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return showIndexRun(cmd.InOrStdin(), cmd.OutOrStdout())
	},
}

func showIndexRun(in io.Reader, out io.Writer) error {
	idx := idxfile.NewMemoryIndex(crypto.SHA1.Size())
	dec := idxfile.NewDecoder(in, hash.New(crypto.SHA1))

	if err := dec.Decode(idx); err != nil {
		return fmt.Errorf("decode idx: %w", err)
	}

	iter, err := idx.EntriesByOffset()
	if err != nil {
		return err
	}

	for {
		e, err := iter.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return err
		}

		crc, _ := idx.FindCRC32(e.Hash)
		fmt.Fprintf(out, "%d %s %08x\n", e.Offset, e.Hash, crc)
	}

	return nil
}
