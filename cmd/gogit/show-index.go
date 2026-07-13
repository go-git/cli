package main

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"time"

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

	idxIn, err := idxInput(in)
	if err != nil {
		return err
	}

	dec := idxfile.NewDecoder(idxIn, hash.New(crypto.SHA1))

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

func idxInput(in io.Reader) (idxfile.Input, error) {
	b, err := io.ReadAll(in)
	if err != nil {
		return nil, fmt.Errorf("read idx input: %w", err)
	}

	return &memoryIdxInput{
		Reader: bytes.NewReader(b),
		size:   int64(len(b)),
	}, nil
}

type memoryIdxInput struct {
	*bytes.Reader

	size int64
}

func (in *memoryIdxInput) Stat() (fs.FileInfo, error) {
	return memoryIdxFileInfo{size: in.size}, nil
}

type memoryIdxFileInfo struct {
	size int64
}

func (fi memoryIdxFileInfo) Name() string       { return "" }
func (fi memoryIdxFileInfo) Size() int64        { return fi.size }
func (fi memoryIdxFileInfo) Mode() fs.FileMode  { return 0 }
func (fi memoryIdxFileInfo) ModTime() time.Time { return time.Time{} }
func (fi memoryIdxFileInfo) IsDir() bool        { return false }
func (fi memoryIdxFileInfo) Sys() any           { return nil }
