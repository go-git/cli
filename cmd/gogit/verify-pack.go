package main

import (
	"crypto"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"

	"github.com/go-git/go-billy/v6"
	fixtures "github.com/go-git/go-git-fixtures/v5"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/format/idxfile"
	"github.com/go-git/go-git/v6/plumbing/format/packfile"
	"github.com/spf13/cobra"
)

var (
	verifyPackVerbose    bool
	verifyPackFixtureUrl bool
	verifyPackFixtureTag bool
	verifyPackSHA256     bool
)

func init() {
	verifyPackCmd.Flags().BoolVarP(&verifyPackVerbose, "verbose", "v", false, "Show detailed object information")
	verifyPackCmd.Flags().BoolVarP(&verifyPackFixtureUrl, "fixture-url", "", false, "Use <file> as go-git-fixture url")
	verifyPackCmd.Flags().BoolVarP(&verifyPackFixtureTag, "fixture-tag", "", false, "Use <file> as go-git-fixture tag")
	verifyPackCmd.Flags().BoolVarP(&verifyPackSHA256, "sha256", "", false, "Treat the pack file as sha256")
	rootCmd.AddCommand(verifyPackCmd)
}

var verifyPackCmd = &cobra.Command{
	Use:   "verify-pack [-v] <file>",
	Short: "Validate packed Git archive files",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return verifyPack(args[0], verifyPackVerbose)
	},
	DisableFlagsInUseLine: true,
}

type objectInfo struct {
	hash       plumbing.Hash
	typ        plumbing.ObjectType
	diskType   plumbing.ObjectType
	size       int64
	packedSize int64
	offset     int64
	depth      int
	base       plumbing.Hash
}

func verifyPack(path string, verbose bool) error {
	idxFile, packFile, err := openPack(path)
	if err != nil {
		return err
	}

	defer func() {
		err = idxFile.Close()
		if err != nil {
			slog.Debug("failed to close idx file", "error", err)
		}
	}()

	defer func() {
		err = packFile.Close()
		if err != nil {
			slog.Debug("failed to close pack file", "error", err)
		}
	}()

	ch := crypto.SHA1
	if verifyPackSHA256 {
		ch = crypto.SHA256
	}

	idx := idxfile.NewMemoryIndex(ch.Size())

	dec := idxfile.NewDecoder(idxFile)
	if err := dec.Decode(idx); err != nil {
		return fmt.Errorf("failed to decode index file: %w", err)
	}

	pf := packfile.NewPackfile(
		packFile,
		packfile.WithIdx(idx),
		packfile.WithObjectIDSize(ch.Size()),
	)

	defer func() {
		err := pf.Close()
		if err != nil {
			slog.Debug("failed to close Packfile object", "error", err)
		}
	}()

	scanner, err := pf.Scanner() //nolint:staticcheck
	if err != nil {
		return fmt.Errorf("failed to get scanner: %w", err)
	}

	entries, err := idx.EntriesByOffset()
	if err != nil {
		return fmt.Errorf("failed to get entries: %w", err)
	}

	var objects []objectInfo

	for {
		entry, err := entries.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return fmt.Errorf("failed to read entry: %w", err)
		}

		// Read raw object header to get delta information.
		err = scanner.SeekFromStart(int64(entry.Offset))
		if err != nil {
			return fmt.Errorf("failed to seek to offset %d: %w", entry.Offset, err)
		}

		if !scanner.Scan() {
			return fmt.Errorf("failed to scan object at offset %d", entry.Offset)
		}

		header, ok := scanner.Data().Value().(packfile.ObjectHeader)
		if !ok {
			return errors.New("failed to scan pack header")
		}

		// For delta objects, Size is the delta size.
		// For regular objects, Size is the inflated size.
		info := objectInfo{
			hash:     entry.Hash,
			diskType: header.Type,
			size:     header.Size,
			offset:   int64(entry.Offset),
		}

		// Calculate packed size (distance to next header or end of file).
		if len(objects) > 0 {
			objects[len(objects)-1].packedSize = info.offset - objects[len(objects)-1].offset
		}

		objects = append(objects, info)
	}

	// Calculate the packed size of the last object.
	if len(objects) > 0 {
		stat, err := packFile.Stat()
		if err != nil {
			return fmt.Errorf("failed to stat pack file: %w", err)
		}
		// Pack file ends with a checksum (20-byte SHA-1 or 32-byte SHA-256).
		objects[len(objects)-1].packedSize = stat.Size() - objects[len(objects)-1].offset - int64(ch.Size())
	}

	// Resolve actual types for all objects (after delta application).
	for i := range objects {
		obj, err := pf.GetByOffset(objects[i].offset)
		if err != nil {
			return fmt.Errorf("failed to get object at offset %d: %w", objects[i].offset, err)
		}

		objects[i].typ = obj.Type()
	}

	// Build delta chain information.
	deltaChains := make(map[plumbing.Hash]int)
	objectByHash := make(map[plumbing.Hash]*objectInfo)
	objectByOffset := make(map[int64]*objectInfo)

	for i := range objects {
		objectByHash[objects[i].hash] = &objects[i]
		objectByOffset[objects[i].offset] = &objects[i]
	}

	// Calculate delta chains by reading headers again.
	for i := range objects {
		if !objects[i].diskType.IsDelta() {
			continue
		}

		err := scanner.SeekFromStart(objects[i].offset)
		if err != nil {
			return fmt.Errorf("failed to seek to offset %d: %w", objects[i].offset, err)
		}

		if !scanner.Scan() {
			return fmt.Errorf("failed to scan object at offset %d", objects[i].offset)
		}

		header, ok := scanner.Data().Value().(packfile.ObjectHeader)
		if !ok {
			return errors.New("failed to scan pack header")
		}

		// Calculate delta chain depth.
		depth := 1

		var baseHash plumbing.Hash

		//exhaustive:ignore only delta types needs handling.
		switch header.Type {
		case plumbing.REFDeltaObject:
			baseHash = header.Reference
		case plumbing.OFSDeltaObject:
			// OffsetReference is the absolute offset of the base object.
			if baseObj, ok := objectByOffset[header.OffsetReference]; ok {
				baseHash = baseObj.hash
			}
		}

		// Follow the chain to calculate total depth.
		if !baseHash.IsZero() {
			current := baseHash
			for {
				baseObj, ok := objectByHash[current]
				if !ok {
					break
				}

				if !baseObj.diskType.IsDelta() {
					// Reached a non-delta base.
					break
				}

				// Get the base object's header.
				err := scanner.SeekFromStart(baseObj.offset)
				if err != nil {
					break
				}

				if !scanner.Scan() {
					break
				}

				baseHeader, ok := scanner.Data().Value().(packfile.ObjectHeader)
				if !ok {
					break
				}

				depth++

				if baseHeader.Type == plumbing.REFDeltaObject {
					current = baseHeader.Reference
				} else if baseHeader.Type == plumbing.OFSDeltaObject {
					// OffsetReference is the absolute offset.
					if nextBase, ok := objectByOffset[baseHeader.OffsetReference]; ok {
						current = nextBase.hash
					} else {
						break
					}
				} else {
					break
				}
			}
		}

		objects[i].depth = depth
		objects[i].base = baseHash
		deltaChains[objects[i].hash] = depth
	}

	if verbose {
		for _, obj := range objects {
			// Format type with padding to match git's output.
			typeStr := obj.typ.String()
			if len(typeStr) == 4 {
				typeStr = typeStr + "   "
			} else {
				typeStr = typeStr + " "
			}

			fmt.Printf("%s %s%d %d %d",
				obj.hash.String(),
				typeStr,
				obj.size,
				obj.packedSize,
				obj.offset,
			)

			if obj.diskType.IsDelta() && !obj.base.IsZero() {
				fmt.Printf(" %d %s", obj.depth, obj.base.String())
			}

			fmt.Println()
		}

		// Print statistics.
		nonDelta := len(objects) - len(deltaChains)
		fmt.Printf("non delta: %d objects\n", nonDelta)

		// Count chain lengths.
		chainLengths := make(map[int]int)
		for _, depth := range deltaChains {
			chainLengths[depth]++
		}

		// Sort chain lengths for consistent output.
		var lengths []int
		for length := range chainLengths {
			lengths = append(lengths, length)
		}

		sort.Ints(lengths)

		for _, length := range lengths {
			count := chainLengths[length]

			objWord := "objects"
			if count == 1 {
				objWord = "object"
			}

			fmt.Printf("chain length = %d: %d %s\n", length, count, objWord)
		}
	}

	fmt.Printf("%s: ok\n", path)

	return nil
}

func openPack(path string) (billy.File, billy.File, error) {
	if verifyPackFixtureUrl || verifyPackFixtureTag {
		var f fixtures.Fixtures
		if verifyPackFixtureUrl {
			f = fixtures.ByURL(path)
		}

		if verifyPackFixtureTag {
			f = fixtures.ByTag(path)
		}

		if len(f) == 0 {
			return nil, nil, fmt.Errorf("no fixture found for %q", path)
		}

		fixture := f.One()

		return fixture.Idx(), fixture.Packfile(), nil
	}

	idxPath := path
	packPath := path

	if before, ok := strings.CutSuffix(path, ".idx"); ok {
		packPath = before + ".pack"
	} else if before, ok := strings.CutSuffix(path, ".pack"); ok {
		idxPath = before + ".idx"
	} else {
		return nil, nil, errors.New("file must have .idx or .pack extension")
	}

	idxFile, err := os.Open(idxPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open index file: %w", err)
	}

	packFile, err := os.Open(packPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open pack file: %w", err)
	}

	return idxFile, packFile, nil
}
