package main

import (
	"crypto"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-git/go-billy/v6/osfs"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/format/idxfile"
	"github.com/go-git/go-git/v6/plumbing/format/packfile"
	"github.com/spf13/cobra"
)

var verifyPackVerbose bool

func init() {
	verifyPackCmd.Flags().BoolVarP(&verifyPackVerbose, "verbose", "v", false, "Show detailed object information")
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
	idxPath := path
	packPath := path

	if strings.HasSuffix(path, ".idx") {
		packPath = strings.TrimSuffix(path, ".idx") + ".pack"
	} else if strings.HasSuffix(path, ".pack") {
		idxPath = strings.TrimSuffix(path, ".pack") + ".idx"
	} else {
		return fmt.Errorf("file must have .idx or .pack extension")
	}

	idxFile, err := os.Open(idxPath)
	if err != nil {
		return fmt.Errorf("failed to open index file: %w", err)
	}
	defer func() {
		err = idxFile.Close()
		if err != nil {
			slog.Debug("failed to close idx file", "error", err)
		}
	}()

	idx := idxfile.NewMemoryIndex(crypto.SHA1.Size())
	dec := idxfile.NewDecoder(idxFile)
	if err := dec.Decode(idx); err != nil {
		return fmt.Errorf("failed to decode index file: %w", err)
	}

	fs := osfs.New(filepath.Dir(packPath))
	packFile, err := fs.Open(filepath.Base(packPath))
	if err != nil {
		return fmt.Errorf("failed to open pack file: %w", err)
	}
	defer func() {
		err = packFile.Close()
		if err != nil {
			slog.Debug("failed to close pack file", "error", err)
		}
	}()

	pf := packfile.NewPackfile(
		packFile,
		packfile.WithIdx(idx),
		packfile.WithFs(fs),
	)
	defer func() {
		err = pf.Close()
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
		if err == io.EOF {
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

		header := scanner.Data().Value().(packfile.ObjectHeader)

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
		// Pack file ends with a 20-byte SHA-1 checksum.
		objects[len(objects)-1].packedSize = stat.Size() - objects[len(objects)-1].offset - int64(crypto.SHA1.Size())
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

		header := scanner.Data().Value().(packfile.ObjectHeader)

		// Calculate delta chain depth.
		depth := 1
		var baseHash plumbing.Hash

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
				baseHeader := scanner.Data().Value().(packfile.ObjectHeader)

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

	fmt.Printf("%s: ok\n", packPath)

	return nil
}
