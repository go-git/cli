package main

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	formatcfg "github.com/go-git/go-git/v6/plumbing/format/config"
	"github.com/go-git/go-git/v6/plumbing/format/idxfile"
	"github.com/go-git/go-git/v6/plumbing/format/packfile"
	"github.com/go-git/go-git/v6/plumbing/hash"
	"github.com/go-git/go-git/v6/plumbing/storer"
	"github.com/spf13/cobra"
)

var (
	indexPackStdin        bool
	indexPackStrict       bool
	indexPackOutput       string
	indexPackFsckObjects  bool
	indexPackIndexVersion string
	indexPackVerify       bool
	indexPackRevIndex     bool
	indexPackNoRevIndex   bool
)

func init() {
	indexPackCmd.Flags().BoolVar(&indexPackStdin, "stdin", false, "Read the pack from standard input")
	indexPackCmd.Flags().BoolVar(&indexPackStrict, "strict", false, "Reject packs containing duplicate object IDs")
	indexPackCmd.Flags().StringVarP(&indexPackOutput, "output", "o", "", "Explicit idx output path")
	indexPackCmd.Flags().BoolVar(&indexPackFsckObjects, "fsck-objects", false, "Validate object structure during indexing")
	indexPackCmd.Flags().StringVar(&indexPackIndexVersion, "index-version", "2", "Idx version to write (1 or 2)")
	indexPackCmd.Flags().BoolVar(&indexPackVerify, "verify", false, "Verify the matching .idx for an existing pack file")
	indexPackCmd.Flags().BoolVar(&indexPackRevIndex, "rev-index", false, "Also write a .rev file")
	// --no-rev-index is the explicit negation. We can't use cobra's --no-* auto-
	// inversion (it's not enabled), so wire a separate flag that resets the same
	// underlying bool to false. Last-flag-wins for combined invocations.
	indexPackCmd.Flags().BoolVar(&indexPackNoRevIndex, "no-rev-index", false,
		"Do not write a .rev file (overrides --rev-index and pack.writeReverseIndex)")
	rootCmd.AddCommand(indexPackCmd)
}

var indexPackCmd = &cobra.Command{
	Use:   "index-pack --stdin [--strict] [-o <path>] [--rev-index] [--verify <pack>]",
	Short: "Build a pack index for an existing packed archive",
	RunE: func(cmd *cobra.Command, args []string) error {
		// --verify mode: take a positional pack path, no --stdin.
		if indexPackVerify {
			if len(args) != 1 {
				return errors.New("--verify requires exactly one pack file argument")
			}

			return indexPackVerifyRun(args[0])
		}

		// Positional-pack-file mode: index an existing .pack on disk.
		if !indexPackStdin && len(args) == 1 {
			return indexPackFromFile(args[0])
		}

		if !indexPackStdin {
			return errors.New("--stdin is required")
		}

		r, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}

		defer r.Close()

		repoCfg, _ := r.Config()
		writeRev := (indexPackRevIndex || configBool("pack.writeReverseIndex", repoCfg, false)) &&
			!indexPackNoRevIndex

		opts := indexPackOpts{
			strict:       indexPackStrict,
			output:       indexPackOutput,
			fsckObjects:  indexPackFsckObjects,
			indexVersion: 2,
			revIndex:     writeRev,
		}

		if indexPackIndexVersion != "" && indexPackIndexVersion != "2" {
			v, err := parseIndexVersion(indexPackIndexVersion)
			if err != nil {
				return err
			}

			opts.indexVersion = v
		}

		needsPostProcess := opts.output != "" || opts.fsckObjects || opts.indexVersion != 2 || opts.revIndex || opts.strict
		if !needsPostProcess {
			return indexPackRun(r, cmd.InOrStdin(), false)
		}

		buf, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return fmt.Errorf("read pack: %w", err)
		}

		return indexPackProcess(r, buf, opts)
	},
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}

type indexPackOpts struct {
	strict       bool
	output       string
	fsckObjects  bool
	indexVersion int
	revIndex     bool
}

// indexPackRun handles the simple streaming non-strict path.
func indexPackRun(repo *git.Repository, in io.Reader, strict bool) error {
	pw, ok := repo.Storer.(storer.PackfileWriter)
	if !ok {
		return errors.New("repository storer does not support packfile writes")
	}

	if !strict {
		return packfile.WritePackfileToObjectStorage(pw, in)
	}

	return indexPackStrictStream(repo, pw, in)
}

// indexPackStrictStream streams the pack from in to a temp file while
// concurrently scanning it for duplicate object IDs. Memory is bounded by the
// parser's working set rather than buffering the entire pack. Only after a
// clean parse is the temp file handed to the storer for commit.
func indexPackStrictStream(repo *git.Repository, sw storer.PackfileWriter, in io.Reader) error {
	tmp, err := os.CreateTemp(strictTempDir(repo), "gogit-strict-*.pack")
	if err != nil {
		return err
	}

	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	pr, pwPipe := io.Pipe()

	parseErrCh := make(chan error, 1)

	go func() {
		perr := checkPackForDuplicates(pr)
		if perr != nil {
			// Drain remaining bytes so the producer's MultiWriter Write does
			// not block on a stalled reader.
			_, _ = io.Copy(io.Discard, pr)
		}

		parseErrCh <- perr
	}()

	if _, err := io.Copy(io.MultiWriter(tmp, pwPipe), in); err != nil {
		_ = pwPipe.Close()
		_ = tmp.Close()

		<-parseErrCh

		return fmt.Errorf("read pack: %w", err)
	}

	_ = pwPipe.Close()

	if err := <-parseErrCh; err != nil {
		_ = tmp.Close()

		return err
	}

	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		_ = tmp.Close()

		return err
	}

	defer tmp.Close()

	return packfile.WritePackfileToObjectStorage(sw, tmp)
}

// strictTempDir picks a directory for the strict-mode tempfile. The repo's
// pack dir is preferred (same filesystem as the eventual commit destination)
// with the OS temp dir as a fallback.
func strictTempDir(repo *git.Repository) string {
	packDir := filepath.Join(repoGitDir(repo), "objects", "pack")
	if err := os.MkdirAll(packDir, 0o755); err == nil {
		return packDir
	}

	return ""
}

// indexPackProcess handles the post-processing paths: -o, --fsck-objects,
// --index-version, --rev-index, --strict.
func indexPackProcess(r *git.Repository, pack []byte, opts indexPackOpts) error {
	if opts.strict {
		if err := checkPackForDuplicates(bytes.NewReader(pack)); err != nil {
			return err
		}
	}

	if opts.fsckObjects {
		if err := fsckPackObjects(pack); err != nil {
			return err
		}
	}

	// The pack SHA is the last 20 bytes of the pack data.
	if len(pack) < 20 {
		return errors.New("pack data too short")
	}

	packHash, ok := plumbing.FromBytes(pack[len(pack)-20:])
	if !ok {
		return errors.New("invalid pack SHA trailer")
	}

	gitDir := repoGitDir(r)
	packDir := filepath.Join(gitDir, "objects", "pack")

	if err := os.MkdirAll(packDir, 0o755); err != nil {
		return fmt.Errorf("create pack dir: %w", err)
	}

	packPath := filepath.Join(packDir, "pack-"+packHash.String()+".pack")

	if err := os.WriteFile(packPath, pack, 0o444); err != nil {
		return fmt.Errorf("write pack: %w", err)
	}

	idx, err := buildIdxFromPack(packPath)
	if err != nil {
		_ = os.Remove(packPath)

		return fmt.Errorf("build idx: %w", err)
	}

	idxPath := opts.output
	if idxPath == "" {
		idxPath = filepath.Join(packDir, "pack-"+packHash.String()+".idx")
	}

	if err := writeIdxFile(idxPath, idx, opts.indexVersion, packHash); err != nil {
		_ = os.Remove(packPath)

		return fmt.Errorf("write idx: %w", err)
	}

	if opts.revIndex {
		var revPath string

		if opts.output != "" {
			if base, ok := strings.CutSuffix(opts.output, ".idx"); ok {
				revPath = base + ".rev"
			} else {
				revPath = opts.output + ".rev"
			}
		} else {
			revPath = filepath.Join(packDir, "pack-"+packHash.String()+".rev")
		}

		if err := writeRevIndex(idx, revPath); err != nil {
			_ = os.Remove(packPath)
			_ = os.Remove(idxPath)

			return fmt.Errorf("write rev index: %w", err)
		}
	}

	return nil
}

// indexPackFromFile indexes an existing pack file on disk. The .idx is written
// adjacent to the .pack (or at the -o path). Respects --rev-index, --no-rev-index,
// and pack.writeReverseIndex config. Does not require a git repository in cwd.
func indexPackFromFile(packPath string) error {
	if !strings.HasSuffix(packPath, ".pack") {
		return fmt.Errorf("expected a .pack file, got %q", packPath)
	}

	idx, err := buildIdxFromPack(packPath)
	if err != nil {
		return fmt.Errorf("build idx: %w", err)
	}

	// Determine pack hash from the path name (pack-<hash>.pack).
	base := filepath.Base(packPath)
	packHashStr := strings.TrimPrefix(strings.TrimSuffix(base, ".pack"), "pack-")
	packHash := plumbing.NewHash(packHashStr)

	idxPath := indexPackOutput
	if idxPath == "" {
		idxPath = packPath[:len(packPath)-len(".pack")] + ".idx"
	}

	if err := writeIdxFile(idxPath, idx, 2, packHash); err != nil {
		return fmt.Errorf("write idx: %w", err)
	}

	// Determine rev-index write:
	//   1. --no-rev-index is a hard override → never write
	//   2. --rev-index flag → always write
	//   3. pack.writeReverseIndex from -c override (key present) → use it
	//   4. pack.writeReverseIndex from .git/config → read raw file
	var writeRev bool

	switch {
	case indexPackNoRevIndex:
		writeRev = false
	case indexPackRevIndex:
		writeRev = true
	default:
		if hasConfigOverride("pack.writeReverseIndex") {
			writeRev = configBool("pack.writeReverseIndex", nil, false)
		} else if gitDir, gerr := findGitDir(); gerr == nil {
			cfgPath := filepath.Join(gitDir, "config")
			writeRev = readConfigBool(cfgPath, "pack", "writeReverseIndex", false)
		}
	}

	if writeRev {
		var revPath string

		if indexPackOutput != "" {
			if base2, ok := strings.CutSuffix(indexPackOutput, ".idx"); ok {
				revPath = base2 + ".rev"
			} else {
				revPath = indexPackOutput + ".rev"
			}
		} else {
			revPath = packPath[:len(packPath)-len(".pack")] + ".rev"
		}

		if err := writeRevIndex(idx, revPath); err != nil {
			return fmt.Errorf("write rev index: %w", err)
		}
	}

	return nil
}

// readConfigBool reads a boolean value from a raw git config file without
// requiring a full repository open.
func readConfigBool(cfgPath, section, key string, defaultVal bool) bool {
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return defaultVal
	}

	raw := formatcfg.New()
	if err := formatcfg.NewDecoder(strings.NewReader(string(data))).Decode(raw); err != nil {
		return defaultVal
	}

	val := raw.Section(section).Option(key)
	if val == "" {
		return defaultVal
	}

	return strings.EqualFold(val, "true")
}

// indexPackVerifyRun rebuilds an idx in-memory from packPath and compares it
// byte-for-byte against the on-disk <packPath-without-.pack>.idx.
// When indexPackRevIndex is set it additionally verifies the on-disk .rev.
func indexPackVerifyRun(packPath string) error {
	if !strings.HasSuffix(packPath, ".pack") {
		return fmt.Errorf("expected a .pack file, got %q", packPath)
	}

	idx, err := buildIdxFromPack(packPath)
	if err != nil {
		return fmt.Errorf("build idx from pack: %w", err)
	}

	var rebuilt bytes.Buffer
	if err := idxfile.Encode(&rebuilt, hash.New(crypto.SHA1), idx); err != nil {
		return fmt.Errorf("encode rebuilt idx: %w", err)
	}

	prefix := packPath[:len(packPath)-len(".pack")]
	idxPath := prefix + ".idx"

	onDisk, err := os.ReadFile(idxPath)
	if err != nil {
		return fmt.Errorf("read on-disk idx: %w", err)
	}

	if !bytes.Equal(rebuilt.Bytes(), onDisk) {
		return errors.New("idx mismatch: on-disk idx does not match pack contents")
	}

	if indexPackRevIndex {
		var rebuiltRev bytes.Buffer
		if err := encodeRevIndex(idx, &rebuiltRev); err != nil {
			return fmt.Errorf("encode rebuilt rev index: %w", err)
		}

		revPath := prefix + ".rev"

		onDiskRev, err := os.ReadFile(revPath)
		if err != nil {
			return fmt.Errorf("read on-disk rev: %w", err)
		}

		if !bytes.Equal(rebuiltRev.Bytes(), onDiskRev) {
			return errors.New("rev validation error: on-disk .rev does not match pack contents")
		}
	}

	return nil
}

// checkPackForDuplicates parses the pack stream and returns an error if any
// object ID appears more than once. Streams from r; does not buffer the whole
// pack.
func checkPackForDuplicates(r io.Reader) error {
	obs := &dupObserver{seen: make(map[plumbing.Hash]struct{})}
	parser := packfile.NewParser(r, packfile.WithScannerObservers(obs))

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

// fsckPackObjects parses the pack and validates that each object's content can
// be read without error.
func fsckPackObjects(pack []byte) error {
	obs := &fsckObserver{}
	parser := packfile.NewParser(bytes.NewReader(pack), packfile.WithScannerObservers(obs))

	if _, err := parser.Parse(); err != nil {
		return fmt.Errorf("fsck parse pack: %w", err)
	}

	return obs.err
}

type fsckObserver struct {
	err error
}

func (o *fsckObserver) OnHeader(_ uint32) error        { return nil }
func (o *fsckObserver) OnFooter(_ plumbing.Hash) error { return nil }

func (o *fsckObserver) OnInflatedObjectHeader(_ plumbing.ObjectType, _, _ int64) error {
	return nil
}

func (o *fsckObserver) OnInflatedObjectContent(_ plumbing.Hash, _ int64, _ uint32, content []byte) error {
	if content == nil {
		o.err = errors.New("nil object content")

		return o.err
	}

	return nil
}
