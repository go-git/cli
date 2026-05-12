package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/format/packfile"
	"github.com/go-git/go-git/v6/plumbing/storer"
	"github.com/spf13/cobra"
)

var (
	indexPackStdin  bool
	indexPackStrict bool
)

func init() {
	indexPackCmd.Flags().BoolVar(&indexPackStdin, "stdin", false, "Read the pack from standard input")
	indexPackCmd.Flags().BoolVar(&indexPackStrict, "strict", false, "Reject packs containing duplicate object IDs")
	rootCmd.AddCommand(indexPackCmd)
}

var indexPackCmd = &cobra.Command{
	Use:   "index-pack --stdin [--strict]",
	Short: "Build a pack index for an existing packed archive",
	RunE: func(cmd *cobra.Command, _ []string) error {
		if !indexPackStdin {
			return errors.New("--stdin is required")
		}

		r, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}

		return indexPackRun(r, cmd.InOrStdin(), indexPackStrict)
	},
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}

func indexPackRun(repo *git.Repository, in io.Reader, strict bool) error {
	pw, ok := repo.Storer.(storer.PackfileWriter)
	if !ok {
		return errors.New("repository storer does not support packfile writes")
	}

	if !strict {
		return packfile.WritePackfileToObjectStorage(pw, in)
	}

	buf, err := io.ReadAll(in)
	if err != nil {
		return fmt.Errorf("read pack: %w", err)
	}

	if err := checkPackForDuplicates(buf); err != nil {
		return err
	}

	return packfile.WritePackfileToObjectStorage(pw, bytes.NewReader(buf))
}

func checkPackForDuplicates(pack []byte) error {
	obs := &dupObserver{seen: make(map[plumbing.Hash]struct{})}
	parser := packfile.NewParser(bytes.NewReader(pack), packfile.WithScannerObservers(obs))

	if _, err := parser.Parse(); err != nil {
		return fmt.Errorf("parse pack: %w", err)
	}

	if obs.dup != plumbing.ZeroHash {
		return fmt.Errorf("duplicate object %s in pack (--strict)", obs.dup)
	}

	return nil
}

type dupObserver struct {
	seen map[plumbing.Hash]struct{}
	dup  plumbing.Hash
}

func (o *dupObserver) OnHeader(_ uint32) error        { return nil }
func (o *dupObserver) OnFooter(_ plumbing.Hash) error { return nil }

func (o *dupObserver) OnInflatedObjectHeader(_ plumbing.ObjectType, _, _ int64) error {
	return nil
}

func (o *dupObserver) OnInflatedObjectContent(h plumbing.Hash, _ int64, _ uint32, _ []byte) error {
	if _, ok := o.seen[h]; ok && o.dup == plumbing.ZeroHash {
		o.dup = h
	}

	o.seen[h] = struct{}{}

	return nil
}
