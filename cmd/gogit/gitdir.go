package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// gitDirName is the name of a repository's git directory inside a work tree.
const gitDirName = ".git"

// findGitDir locates the repository's git directory.
func findGitDir() (string, error) {
	path, _, err := discoverGitDir()

	return path, err
}

// discoverGitDir locates the repository's git directory and returns both the
// path to use for I/O and the spelling git would use to name it in a
// diagnostic.
//
// It handles the three shapes git supports: a .git directory in the working
// tree or an ancestor, a .git *file* pointing at a linked worktree's git dir,
// and a bare repository whose working directory is the git dir itself.
//
// The two results differ because git chdirs to the top level of the working
// tree before doing anything, which leaves its GIT_DIR relative; gogit does
// not chdir, so it needs the absolute path to open the file and the relative
// one to report it.
func discoverGitDir() (string, string, error) {
	if d := os.Getenv("GIT_DIR"); d != "" {
		// An explicit GIT_DIR is reported exactly as it was given.
		return d, d, nil
	}

	dir, err := os.Getwd()
	if err != nil {
		return "", "", err
	}

	for {
		gitPath := filepath.Join(dir, gitDirName)

		info, err := os.Stat(gitPath)
		switch {
		case err == nil && info.IsDir():
			return gitPath, gitDirName, nil
		case err == nil && info.Mode().IsRegular():
			// A linked worktree resolves to an absolute path, and git reports
			// it that way.
			resolved, err := readGitFile(gitPath)

			return resolved, resolved, err
		}

		if isGitDir(dir) {
			// A bare repository is its own git dir, which git names ".".
			return dir, ".", nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", errors.New("not a git repository")
		}

		dir = parent
	}
}

// readGitFile resolves a ".git" file, whose contents are "gitdir: <path>".
func readGitFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	target, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:")
	if !ok {
		return "", errors.New("not a git repository")
	}

	target = strings.TrimSpace(target)
	if target == "" {
		return "", errors.New("not a git repository")
	}

	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}

	return filepath.Clean(target), nil
}

// isGitDir reports whether dir is itself a git directory, which is how a bare
// repository presents itself.
func isGitDir(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "HEAD")); err != nil {
		return false
	}

	return isDir(filepath.Join(dir, "objects")) && isDir(filepath.Join(dir, "refs"))
}

func isDir(path string) bool {
	info, err := os.Stat(path)

	return err == nil && info.IsDir()
}
