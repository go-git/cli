package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// gitDirName is the name of a repository's git directory inside a work tree.
const gitDirName = ".git"

var errNoRepository = errors.New("not a git repository")

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
	if d, present := os.LookupEnv("GIT_DIR"); present {
		info, err := os.Stat(d)
		if err != nil {
			if os.IsNotExist(err) {
				return "", "", fmt.Errorf("%w: %s", errNoRepository, d)
			}

			return "", "", err
		}

		if info.IsDir() {
			if isGitDir(d) {
				return d, d, nil
			}

			return "", "", fmt.Errorf("%w: %s", errNoRepository, d)
		}

		if info.Mode().IsRegular() {
			resolved, err := readGitFile(d)

			return resolved, d, err
		}

		return "", "", fmt.Errorf("%w: %s", errNoRepository, d)
	}

	dir, err := os.Getwd()
	if err != nil {
		return "", "", err
	}

	for {
		gitPath := filepath.Join(dir, gitDirName)

		info, err := os.Stat(gitPath)
		switch {
		case err == nil && info.IsDir() && isGitDir(gitPath):
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
			return "", "", errNoRepository
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
		return "", fmt.Errorf("invalid gitfile format: %s", path)
	}

	target = strings.TrimSpace(target)
	if target == "" {
		return "", fmt.Errorf("invalid gitfile format: %s", path)
	}

	if !filepath.IsAbs(target) {
		parent, _ := filepath.Split(path)
		target = parent + target
	}

	if !isGitDir(target) {
		return "", fmt.Errorf("not a git repository: %s", target)
	}

	return target, nil
}

// isGitDir validates the worktree HEAD and the shared repository directories.
func isGitDir(dir string) bool {
	if !validRepositoryHEAD(dir + "/HEAD") {
		return false
	}

	common, err := commonGitDir(dir)
	if err != nil {
		return false
	}

	return isDir(common+"/objects") && isDir(common+"/refs")
}

func validRepositoryHEAD(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}

	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)

		return err == nil && strings.HasPrefix(target, "refs/")
	}

	if !info.Mode().IsRegular() {
		return false
	}

	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, 255))
	if err != nil {
		return false
	}

	if ref, ok := strings.CutPrefix(string(data), "ref:"); ok {
		return strings.HasPrefix(strings.TrimLeft(ref, " \t\n\r\v\f"), "refs/")
	}
	// Repository discovery accepts detached HEADs before checking object existence.
	if len(data) < 40 {
		return false
	}

	_, err = hex.DecodeString(string(data[:40]))

	return err == nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)

	return err == nil && info.IsDir()
}
