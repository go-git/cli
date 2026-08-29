// Package submodule holds submodule operations that aren't yet exposed by
// go-git itself. The logic here mirrors what would eventually become
// Worktree.AddSubmodule and related helpers in
// github.com/go-git/go-git/v6; the package layout keeps that intent
// explicit so callers can find the upstreaming target at a glance.
package submodule

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/format/index"
)

// Add registers a new submodule at `path` (relative to repo's worktree)
// pointing at `url`. It clones the URL into the worktree at `path`, writes
// a corresponding entry to `.gitmodules`, stages `.gitmodules`, and stages
// a gitlink (mode 160000, hash = clone HEAD) at `path` in the parent's
// index. Returns the cloned submodule repository.
//
// Mirrors `git submodule add <url> <path>` on its happy path. Gaps vs
// upstream: the submodule lives at `<path>/.git`, not under
// `.git/modules/<path>/` with a gitfile pointer — implementing that
// indirection is a follow-up that should land alongside upstreaming.
func Add(repo *git.Repository, url, path string) (*git.Repository, error) {
	if repo == nil {
		return nil, fmt.Errorf("submodule.Add: nil parent repository")
	}

	clean := filepath.ToSlash(filepath.Clean(path))
	if filepath.IsAbs(clean) || strings.HasPrefix(clean, "../") || clean == ".." {
		return nil, fmt.Errorf("submodule.Add: path %q must be inside the repository", path)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("submodule.Add: open parent worktree: %w", err)
	}

	target := filepath.Join(worktree.Filesystem().Root(), clean)

	sub, err := git.PlainClone(target, &git.CloneOptions{URL: url})
	if err != nil {
		return nil, fmt.Errorf("submodule.Add: clone %q: %w", url, err)
	}

	head, err := sub.Head()
	if err != nil {
		return nil, fmt.Errorf("submodule.Add: read clone HEAD: %w", err)
	}

	if err := writeGitmodulesEntry(worktree.Filesystem().Root(), clean, url); err != nil {
		return nil, err
	}

	if _, err := worktree.Add(".gitmodules"); err != nil {
		return nil, fmt.Errorf("submodule.Add: stage .gitmodules: %w", err)
	}

	if err := stageGitlink(repo, clean, head.Hash()); err != nil {
		return nil, err
	}

	return sub, nil
}

// writeGitmodulesEntry creates or replaces the `[submodule "<path>"]`
// section in <repoRoot>/.gitmodules. The submodule's name matches its
// path — upstream's default and what go-git's Submodules() expects.
func writeGitmodulesEntry(repoRoot, path, url string) error {
	gitmodulesPath := filepath.Join(repoRoot, ".gitmodules")

	modules := config.NewModules()

	if data, err := os.ReadFile(gitmodulesPath); err == nil {
		if err := modules.Unmarshal(data); err != nil {
			return fmt.Errorf("submodule.Add: parse existing .gitmodules: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("submodule.Add: read .gitmodules: %w", err)
	}

	modules.Submodules[path] = &config.Submodule{Name: path, Path: path, URL: url}

	out, err := modules.Marshal()
	if err != nil {
		return fmt.Errorf("submodule.Add: marshal .gitmodules: %w", err)
	}

	return os.WriteFile(gitmodulesPath, out, 0o644)
}

// stageGitlink inserts (or replaces) an index entry at `path` with mode
// 160000 (gitlink) pointing at `hash`. go-git's Worktree.Add reads from
// the filesystem and never produces a gitlink, so this is open-coded
// against Storer.Index / SetIndex.
func stageGitlink(repo *git.Repository, path string, hash plumbing.Hash) error {
	idx, err := repo.Storer.Index()
	if err != nil {
		return fmt.Errorf("submodule.Add: read index: %w", err)
	}

	entry := &index.Entry{
		Hash: hash,
		Name: path,
		Mode: filemode.Submodule,
	}

	replaced := false

	for i, e := range idx.Entries {
		if e.Name == path && e.Stage == index.Merged {
			idx.Entries[i] = entry
			replaced = true

			break
		}
	}

	if !replaced {
		idx.Entries = append(idx.Entries, entry)
	}

	sort.Slice(idx.Entries, func(i, j int) bool {
		return idx.Entries[i].Name < idx.Entries[j].Name
	})

	return repo.Storer.SetIndex(idx)
}
