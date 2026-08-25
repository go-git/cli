package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// ReadFile parses the configuration file at path. A missing file parses as an
// empty configuration, which is what Git does; any other read error is
// reported so a transient failure can never be mistaken for an empty file and
// then written back over the original.
func ReadFile(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Parse(nil)
		}

		return nil, err
	}

	return Parse(data)
}

// WriteFile replaces path with f's contents atomically, so an interrupted or
// failing write leaves the original file untouched rather than truncated.
func WriteFile(path string, f *File, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".gogit-*")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}

	tmpName := tmp.Name()
	renamed := false

	// Any path out of this function other than a completed rename leaves the
	// original file untouched and removes the partial temp file.
	defer func() {
		if !renamed {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmp.Chmod(perm); err != nil {
		return fmt.Errorf("chmod temp config: %w", err)
	}

	if _, err := tmp.Write(f.Bytes()); err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}

	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temp config: %w", err)
	}

	// Close before rename, and report the error: a deferred Close would hide
	// write failures that only surface on flush.
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp config: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}

	renamed = true

	return nil
}

// FileMode returns the permissions to give a rewritten config file: the
// existing file's mode, or 0o666 for a new one, so an existing file's
// permissions survive the rename.
func FileMode(path string) os.FileMode {
	if st, err := os.Stat(path); err == nil {
		return st.Mode().Perm()
	}

	return 0o666
}
