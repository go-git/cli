package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// resolvePathspec converts a user-supplied pathspec, interpreted relative to
// cwd, into a path relative to the worktree root. Returns an error if the
// pathspec resolves outside the worktree.
func resolvePathspec(worktreeRoot, cwd, spec string) (string, error) {
	abs := spec
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(cwd, spec)
	}

	abs = filepath.Clean(abs)

	root := filepath.Clean(worktreeRoot)

	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", fmt.Errorf("pathspec %q outside worktree: %w", spec, err)
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("pathspec %q is outside worktree %q", spec, worktreeRoot)
	}

	return rel, nil
}
