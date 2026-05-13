package main

import (
	"bufio"
	"crypto"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	internalidxfile "github.com/go-git/cli/internal/plumbing/format/idxfile"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/format/idxfile"
	"github.com/go-git/go-git/v6/plumbing/format/packfile"
	"github.com/go-git/go-git/v6/plumbing/hash"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/spf13/cobra"
)

var (
	packObjectsAll          bool
	packObjectsIndexVersion string
	packObjectsNoReuse      bool
	packObjectsRevIndex     bool
)

func init() {
	packObjectsCmd.Flags().BoolVar(&packObjectsAll, "all", false, "Pack all reachable objects from refs")
	packObjectsCmd.Flags().StringVar(&packObjectsIndexVersion, "index-version", "2", "Idx version (1 or 2[,offset])")
	packObjectsCmd.Flags().BoolVar(&packObjectsNoReuse, "no-reuse-object", false,
		"Do not reuse delta encodings (accepted, no-op)")
	packObjectsCmd.Flags().BoolVar(&packObjectsRevIndex, "rev-index", false, "Also write a .rev file alongside the .idx")
	rootCmd.AddCommand(packObjectsCmd)
}

var packObjectsCmd = &cobra.Command{
	Use:   "pack-objects <basename>",
	Short: "Create a packed archive of objects",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		basename := args[0]

		r, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}

		defer r.Close()

		hashes, err := packObjectsCollectHashes(r, cmd.InOrStdin(), packObjectsAll)
		if err != nil {
			return err
		}

		idxVer, err := parseIndexVersion(packObjectsIndexVersion)
		if err != nil {
			return err
		}

		repoCfg, _ := r.Config()
		writeRev := packObjectsRevIndex || configBool("pack.writeReverseIndex", repoCfg, false)

		packHash, err := packObjectsInto(r, basename, hashes, idxVer, writeRev)
		if err != nil {
			return err
		}

		fmt.Fprintln(cmd.OutOrStdout(), packHash)

		return nil
	},
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}

// packObjectsInto encodes the given object hashes into a pack at
// <basename>-<sha>.pack with a matching .idx, optionally writing a .rev.
// Returns the pack's content hash. Reusable by Task 7's repack.
func packObjectsInto(
	r *git.Repository, basename string, hashes []plumbing.Hash, idxVer int, writeRev bool,
) (plumbing.Hash, error) {
	// CreateTemp in the destination directory so the final os.Rename stays on
	// the same filesystem (avoids "invalid cross-device link" when /tmp lives
	// on a different mount).
	if dir := filepath.Dir(basename); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return plumbing.ZeroHash, err
		}
	}

	tmpPack, err := os.CreateTemp(filepath.Dir(basename), "gogit-pack-*.pack")
	if err != nil {
		return plumbing.ZeroHash, err
	}
	defer os.Remove(tmpPack.Name())

	enc := packfile.NewEncoder(tmpPack, r.Storer, false)

	packHash, err := enc.Encode(hashes, 10)
	if err != nil {
		_ = tmpPack.Close()

		return plumbing.ZeroHash, err
	}

	if err := tmpPack.Close(); err != nil {
		return plumbing.ZeroHash, err
	}

	packPath := basename + "-" + packHash.String() + ".pack"
	idxPath := basename + "-" + packHash.String() + ".idx"

	if err := os.Rename(tmpPack.Name(), packPath); err != nil {
		return plumbing.ZeroHash, err
	}

	idx, err := buildIdxFromPack(packPath)
	if err != nil {
		return plumbing.ZeroHash, err
	}

	if err := writeIdxFile(idxPath, idx, idxVer, packHash); err != nil {
		return plumbing.ZeroHash, err
	}

	if writeRev {
		revPath := basename + "-" + packHash.String() + ".rev"
		if err := writeRevIndex(idx, revPath); err != nil {
			return plumbing.ZeroHash, err
		}
	}

	return packHash, nil
}

func packObjectsCollectHashes(r *git.Repository, in io.Reader, all bool) ([]plumbing.Hash, error) {
	if all {
		return collectAllReachable(r)
	}

	var hashes []plumbing.Hash

	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		hashes = append(hashes, plumbing.NewHash(line))
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if len(hashes) == 0 {
		return nil, errors.New("no object hashes on stdin")
	}

	return hashes, nil
}

// collectAllReachable walks every ref, then for each commit walks parents and
// tree contents (subtrees + blobs) into a set of object hashes.
func collectAllReachable(r *git.Repository) ([]plumbing.Hash, error) {
	refs, err := r.References()
	if err != nil {
		return nil, err
	}

	seen := map[plumbing.Hash]struct{}{}

	visit := func(h plumbing.Hash) error {
		if _, ok := seen[h]; ok {
			return nil
		}

		seen[h] = struct{}{}

		return nil
	}

	var frontier []plumbing.Hash

	if err := refs.ForEach(func(ref *plumbing.Reference) error {
		h := ref.Hash()
		if h.IsZero() {
			return nil
		}

		frontier = append(frontier, h)

		return nil
	}); err != nil {
		return nil, err
	}

	if err := walkCommits(r, frontier, visit); err != nil {
		return nil, err
	}

	if len(seen) == 0 {
		return nil, errors.New("nothing to pack: no reachable objects")
	}

	out := make([]plumbing.Hash, 0, len(seen))
	for h := range seen {
		out = append(out, h)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })

	return out, nil
}

func walkCommits(r *git.Repository, frontier []plumbing.Hash, visit func(plumbing.Hash) error) error {
	queued := map[plumbing.Hash]struct{}{}
	queue := make([]plumbing.Hash, 0, len(frontier))

	for _, h := range frontier {
		if _, ok := queued[h]; ok {
			continue
		}

		queued[h] = struct{}{}
		queue = append(queue, h)
	}

	for len(queue) > 0 {
		h := queue[0]
		queue = queue[1:]

		commit, err := r.CommitObject(h)
		if err != nil {
			queue = enqueueTagTarget(r, h, queue, queued, visit)

			continue
		}

		if err := visit(commit.Hash); err != nil {
			return err
		}

		tree, err := commit.Tree()
		if err == nil {
			if err := visit(tree.Hash); err != nil {
				return err
			}

			if err := walkTree(tree, visit); err != nil {
				return err
			}
		}

		for _, p := range commit.ParentHashes {
			if _, ok := queued[p]; ok {
				continue
			}

			queued[p] = struct{}{}
			queue = append(queue, p)
		}
	}

	return nil
}

// enqueueTagTarget checks whether h is an annotated tag. If so it records the
// tag object itself and enqueues the tag's target for further traversal.
// Otherwise h is recorded as-is (blob, tree, etc.). Returns the updated queue.
func enqueueTagTarget(
	r *git.Repository,
	h plumbing.Hash,
	queue []plumbing.Hash,
	queued map[plumbing.Hash]struct{},
	visit func(plumbing.Hash) error,
) []plumbing.Hash {
	tag, err := r.TagObject(h)
	if err != nil {
		// Not a tag — blob, tree, or other object; just record it.
		_ = visit(h)

		return queue
	}

	_ = visit(tag.Hash)

	if _, ok := queued[tag.Target]; !ok {
		queued[tag.Target] = struct{}{}
		queue = append(queue, tag.Target)
	}

	return queue
}

// walkTree walks all subtrees and blob entries of the given tree, recording
// every encountered hash via visit.
func walkTree(tree *object.Tree, visit func(plumbing.Hash) error) error {
	for _, e := range tree.Entries {
		if err := visit(e.Hash); err != nil {
			return err
		}

		if e.Mode.IsFile() {
			continue
		}

		sub, err := tree.Tree(e.Name)
		if err != nil {
			continue
		}

		if err := walkTree(sub, visit); err != nil {
			return err
		}
	}

	return nil
}

func parseIndexVersion(s string) (int, error) {
	parts := strings.SplitN(s, ",", 2)
	switch parts[0] {
	case "1":
		if len(parts) > 1 {
			return 0, errors.New("--index-version=1 does not support offset")
		}

		return 1, nil
	case "2":
		return 2, nil
	default:
		return 0, fmt.Errorf("unsupported idx version %q", parts[0])
	}
}

// buildIdxFromPack parses the pack file at packPath and returns a populated
// MemoryIndex.
func buildIdxFromPack(packPath string) (*idxfile.MemoryIndex, error) {
	f, err := os.Open(packPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	w := &idxfile.Writer{}

	parser := packfile.NewParser(f, packfile.WithScannerObservers(w))
	if _, err := parser.Parse(); err != nil {
		return nil, err
	}

	idx, err := w.Index()
	if err != nil {
		return nil, err
	}

	return idx, nil
}

// writeIdxFile encodes idx as either v1 or v2 to path.
func writeIdxFile(path string, idx *idxfile.MemoryIndex, version int, packHash plumbing.Hash) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if version == 2 {
		return idxfile.Encode(f, hash.New(crypto.SHA1), idx)
	}

	return internalidxfile.EncodeV1(f, idx, packHash)
}
