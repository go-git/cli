package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/go-git/go-git/v6/plumbing/format/packfile"
)

// runDelta implements `test-tool delta {-d|-p} <base> <delta> <out>`. We only
// need -p (apply); upstream's -d (compute) is not used by the curated tests.
func runDelta(args []string) error {
	if len(args) != 4 || args[0] != "-p" {
		return errors.New("usage: delta -p <base> <delta> <output>")
	}

	base, err := os.ReadFile(args[1])
	if err != nil {
		return fmt.Errorf("read base %s: %w", args[1], err)
	}

	delta, err := os.ReadFile(args[2])
	if err != nil {
		return fmt.Errorf("read delta %s: %w", args[2], err)
	}

	result, err := packfile.PatchDelta(base, delta)
	if err != nil {
		return fmt.Errorf("apply delta: %w", err)
	}

	if err := os.WriteFile(args[3], result, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", args[3], err)
	}

	return nil
}
