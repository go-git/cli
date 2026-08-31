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

// UpdateFile applies mutate while holding the config lock and atomically
// replaces path. The lock covers the read as well as the write, preventing two
// writers from successfully committing stale snapshots over one another.
func UpdateFile(path string, mutate func(*File) error) error {
	target, err := resolveWritePath(path)
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}

	lockPath := target + ".lock"

	lock, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		return fmt.Errorf("lock config file %s: %w", path, err)
	}

	committed := false

	defer func() {
		if !committed {
			_ = lock.Close()
			_ = os.Remove(lockPath)
		}
	}()

	f, err := ReadFile(target)
	if err != nil {
		return err
	}

	if err := mutate(f); err != nil {
		return err
	}

	if st, err := os.Stat(target); err == nil {
		if err := lock.Chmod(st.Mode().Perm()); err != nil {
			return fmt.Errorf("chmod config lock: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	if _, err := lock.Write(f.Bytes()); err != nil {
		return fmt.Errorf("write config lock: %w", err)
	}

	if err := lock.Sync(); err != nil {
		return fmt.Errorf("sync config lock: %w", err)
	}

	if err := lock.Close(); err != nil {
		return fmt.Errorf("close config lock: %w", err)
	}

	if err := os.Rename(lockPath, target); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}

	committed = true

	return nil
}

func resolveWritePath(path string) (string, error) {
	current, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	seen := map[string]bool{}

	for {
		current = filepath.Clean(current)
		if seen[current] {
			return "", fmt.Errorf("symbolic link loop at %s", current)
		}

		seen[current] = true

		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				return current, nil
			}

			return "", err
		}

		if info.Mode()&os.ModeSymlink == 0 {
			return current, nil
		}

		target, err := os.Readlink(current)
		if err != nil {
			return "", err
		}

		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(current), target)
		}

		current = target
	}
}
