package main

import (
	"crypto"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	internalrevfile "github.com/go-git/cli/internal/plumbing/format/revfile"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/format/idxfile"
	"github.com/go-git/go-git/v6/plumbing/format/revfile"
	"github.com/go-git/go-git/v6/plumbing/hash"
	"github.com/spf13/cobra"
)

var fsckFull bool

func init() {
	fsckCmd.Flags().BoolVar(&fsckFull, "full", false, "Check all object directories, not just the local one")
	rootCmd.AddCommand(fsckCmd)
}

var fsckCmd = &cobra.Command{
	Use:   "fsck",
	Short: "Verify the connectivity and validity of the objects in the database",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		r, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}

		defer r.Close()

		problems := fsckObjects(r)
		problems = append(problems, fsckRevFiles(repoGitDir(r))...)

		for _, p := range problems {
			fmt.Fprintln(cmd.ErrOrStderr(), p)
		}

		if len(problems) > 0 {
			return errors.New("fsck found problems")
		}

		return nil
	},
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}

func fsckObjects(r *git.Repository) []string {
	iter, err := r.Storer.IterEncodedObjects(plumbing.AnyObject)
	if err != nil {
		return []string{fmt.Sprintf("iter objects: %v", err)}
	}

	defer iter.Close()

	var problems []string

	for {
		o, err := iter.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			problems = append(problems, fmt.Sprintf("object: %v", err))

			continue
		}

		rd, err := o.Reader()
		if err != nil {
			problems = append(problems, fmt.Sprintf("object %s: %v", o.Hash(), err))

			continue
		}

		if _, err := io.Copy(io.Discard, rd); err != nil {
			problems = append(problems, fmt.Sprintf("object %s: %v", o.Hash(), err))
		}

		_ = rd.Close()
	}

	return problems
}

// fsckRevFiles validates each `.git/objects/pack/pack-*.rev` via the extended
// revfile validator. Error messages are formatted to match upstream fsck so
// the t5325 corruption tests can grep them.
func fsckRevFiles(gitDir string) []string {
	matches, err := filepath.Glob(filepath.Join(gitDir, "objects", "pack", "pack-*.rev"))
	if err != nil {
		return []string{fmt.Sprintf("glob rev files: %v", err)}
	}

	var problems []string

	for _, m := range matches {
		if err := fsckSingleRevFile(m); err != nil {
			problems = append(problems, internalrevfile.FsckMessage(filepath.Base(m), err))
		}
	}

	return problems
}

// revFileInfo holds the metadata parsed from a .idx file needed for
// validating the corresponding .rev file.
type revFileInfo struct {
	objCount     int64
	packChecksum plumbing.ObjectID
}

// readIdxInfo parses the .idx counterpart of a .rev file and returns
// the object count and pack checksum required by revfile.Decode.
func readIdxInfo(revPath string) (revFileInfo, error) {
	idxPath := strings.TrimSuffix(revPath, ".rev") + ".idx"

	f, err := os.Open(idxPath)
	if err != nil {
		return revFileInfo{}, fmt.Errorf("open idx: %w", err)
	}

	defer f.Close()

	idx := idxfile.NewMemoryIndex(crypto.SHA1.Size())
	dec := idxfile.NewDecoder(f, hash.New(crypto.SHA1))

	if err := dec.Decode(idx); err != nil {
		return revFileInfo{}, fmt.Errorf("decode idx: %w", err)
	}

	count, err := idx.Count()
	if err != nil {
		return revFileInfo{}, fmt.Errorf("idx count: %w", err)
	}

	return revFileInfo{
		objCount:     count,
		packChecksum: idx.PackfileChecksum,
	}, nil
}

func fsckSingleRevFile(path string) error {
	info, err := readIdxInfo(path)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if err := internalrevfile.ValidateHeader(data); err != nil {
		return err
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}

	defer f.Close()

	ch := make(chan uint32, 1024)
	positionErrCh := make(chan error, 1)

	go func() {
		positionErrCh <- internalrevfile.ValidateRowPositions(ch, info.objCount)
	}()

	decodeErr := revfile.Decode(f, info.objCount, info.packChecksum, ch)

	if posErr := <-positionErrCh; posErr != nil {
		// Prioritise the row-position error over any downstream checksum
		// mismatch caused by the same byte flip.
		return posErr
	}

	return decodeErr
}
