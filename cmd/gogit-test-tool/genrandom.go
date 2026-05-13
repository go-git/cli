package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
)

// genRandom emits length deterministic bytes derived from seed, byte-for-byte
// equivalent to upstream's t/helper/test-genrandom.c. The seed loop in C is
// `do { next = next*11 + *c; } while (*c++);` — the body runs once per byte
// INCLUDING the terminating NUL. The PRNG is the POSIX.1-2001 rand() LCG:
// next = next*1103515245 + 12345; output byte = (next>>16) & 0xff. All
// arithmetic is uint64 to match `unsigned long` on 64-bit hosts.
func genRandom(w io.Writer, seed string, length uint64) error {
	var next uint64

	for _, c := range []byte(seed) {
		next = next*11 + uint64(c)
	}
	// Fold the implicit NUL terminator (matching C do/while semantics).
	next *= 11

	bw := bufio.NewWriter(w)
	defer bw.Flush()

	for range length {
		next = next*1103515245 + 12345

		if err := bw.WriteByte(byte((next >> 16) & 0xff)); err != nil {
			return err
		}
	}

	return nil
}

func runGenRandom(args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return errors.New("usage: genrandom <seed_string> [<size>]")
	}

	seed := args[0]
	length := ^uint64(0) // ULONG_MAX equivalent

	if len(args) == 2 {
		v, err := strconv.ParseUint(args[1], 10, 64)
		if err != nil {
			return fmt.Errorf("cannot parse size %q: %w", args[1], err)
		}

		length = v
	}

	return genRandom(stdoutWriter(), seed, length)
}
