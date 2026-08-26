package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gitconfig "github.com/go-git/cli/internal/plumbing/format/config"
)

// gitExitError carries a git-compatible exit status out of a command. msg, when
// non-empty, is the single stderr line to print; main must not print the
// error itself, because git stays silent for some non-zero statuses (a
// missing key exits 1, an unset of a missing key exits 5, both without
// diagnostics).
type gitExitError struct {
	code int
	msg  string
}

func (e *gitExitError) Error() string {
	if e.msg != "" {
		return e.msg
	}

	return fmt.Sprintf("exit status %d", e.code)
}

// configFile names a configuration file twice: the path used for all I/O, and
// the spelling git would use for it in a diagnostic. They differ for a
// repository discovered from the working tree, which git reports relative to
// the top level.
type configFile struct {
	path    string
	display string
}

// configSource is one configuration file consulted for a read, or the set of
// -c command-line overrides.
type configSource struct {
	file      *gitconfig.File
	overrides []configOverride
}

func (s configSource) values(key gitconfig.Key) []string {
	if s.file != nil {
		return s.file.Values(key)
	}

	var out []string

	for _, o := range s.overrides {
		if o.key.Matches(key) {
			out = append(out, o.value)
		}
	}

	return out
}

// readSources returns the files to consult, lowest precedence first.
//
// A location flag selects exactly one source. Otherwise git's default order
// applies: system, then the XDG and per-user global files, then the
// repository, then -c overrides.
func readSources(o *configOpts) ([]configSource, error) {
	if file, ok, err := explicitLocation(o); err != nil {
		return nil, err
	} else if ok {
		src, err := loadSource(file)
		if err != nil {
			return nil, err
		}

		return []configSource{src}, nil
	}

	sources := make([]configSource, 0)

	if p, ok := systemConfigPath(); ok {
		src, loaded, err := loadOptionalSource(absoluteFile(p))
		if err != nil {
			return nil, err
		}

		if loaded {
			sources = append(sources, src)
		}
	}

	for _, p := range globalConfigPaths() {
		src, loaded, err := loadOptionalSource(absoluteFile(p))
		if err != nil {
			return nil, err
		}

		if loaded {
			sources = append(sources, src)
		}
	}

	// Being outside a repository is not an error for a default read: -c
	// overrides and the global files still apply, as they do in git.
	if f, err := localConfigFile(); err == nil {
		src, err := loadSource(f)
		if err != nil {
			return nil, err
		}

		sources = append(sources, src)
	}

	return append(sources, configSource{overrides: configOverrideList}), nil
}

// absoluteFile names a file that git reports by its full path, which is how
// it reports every file it did not discover by walking up from the cwd.
func absoluteFile(path string) configFile {
	return configFile{path: path, display: path}
}

// writeTarget returns the single file a mutation applies to. Writes default
// to the repository config, never to the merged view.
func writeTarget(o *configOpts) (configFile, error) {
	if file, ok, err := explicitLocation(o); err != nil {
		return configFile{}, err
	} else if ok {
		return file, nil
	}

	return localConfigFile()
}

// explicitLocation resolves --file/--local/--global/--system. For --global it
// picks the file git would write to: the XDG file when it already exists,
// otherwise ~/.gitconfig.
func explicitLocation(o *configOpts) (configFile, bool, error) {
	switch {
	case o.file != "":
		// git reports --file exactly as it was spelled on the command line.
		return absoluteFile(o.file), true, nil

	case o.local:
		f, err := localConfigFile()

		return f, true, err

	case o.global:
		paths := globalConfigPaths()
		for _, p := range paths {
			if _, err := os.Stat(p); err == nil {
				return absoluteFile(p), true, nil
			}
		}

		if len(paths) == 0 {
			return configFile{}, true, errors.New("no global config file available")
		}

		return absoluteFile(paths[len(paths)-1]), true, nil

	case o.system:
		p, ok := systemConfigPath()
		if !ok {
			return configFile{}, true, errors.New("system config is disabled by GIT_CONFIG_NOSYSTEM")
		}

		return absoluteFile(p), true, nil
	}

	return configFile{}, false, nil
}

func loadSource(cf configFile) (configSource, error) {
	f, err := gitconfig.ReadFile(cf.path)
	if err != nil {
		return configSource{}, configReadError(cf, err)
	}

	return configSource{file: f}, nil
}

// loadOptionalSource ignores missing and unreadable system/global files, as
// git does, but still reports malformed files instead of hiding corruption.
func loadOptionalSource(cf configFile) (configSource, bool, error) {
	f, err := gitconfig.ReadFile(cf.path)
	if err != nil {
		var perr *gitconfig.ParseError
		if errors.As(err, &perr) {
			return configSource{}, false, configReadError(cf, err)
		}

		return configSource{}, false, nil
	}

	return configSource{file: f}, true, nil
}

// globalConfigPaths returns the per-user config files in ascending precedence
// order, so ~/.gitconfig wins over the XDG file as it does in git.
func globalConfigPaths() []string {
	if p, ok := os.LookupEnv("GIT_CONFIG_GLOBAL"); ok {
		if p == "" || p == os.DevNull {
			return nil
		}

		return []string{p}
	}

	var paths []string

	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		paths = append(paths, filepath.Join(xdg, "git", "config"))
	} else if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".config", "git", "config"))
	}

	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".gitconfig"))
	}

	return paths
}

// systemConfigPath reports the system config file, and whether the system
// scope is enabled at all.
func systemConfigPath() (string, bool) {
	if v := os.Getenv("GIT_CONFIG_NOSYSTEM"); v != "" && v != "0" {
		return "", false
	}

	if p, ok := os.LookupEnv("GIT_CONFIG_SYSTEM"); ok {
		if p == "" || p == os.DevNull {
			return "", false
		}

		return p, true
	}

	return "/etc/gitconfig", true
}

// localConfigFile returns the repository's config file. For a linked worktree
// this is the common directory's config, not the worktree's own git dir.
func localConfigFile() (configFile, error) {
	gitDir, display, err := discoverGitDir()
	if err != nil {
		return configFile{}, err
	}

	common := commonGitDir(gitDir)
	if common != gitDir {
		// Resolving through commondir yields an absolute path, and that is
		// what git reports for a linked worktree.
		display = common
	}

	return configFile{
		path: filepath.Join(common, "config"),
		// Concatenated rather than joined: git names a bare repository's
		// config "./config", which filepath.Join would clean to "config".
		display: display + "/config",
	}, nil
}

// commonGitDir follows a linked worktree's `commondir` pointer back to the
// main git directory, which is where shared state such as config lives.
func commonGitDir(gitDir string) string {
	data, err := os.ReadFile(filepath.Join(gitDir, "commondir"))
	if err != nil {
		return gitDir
	}

	common := strings.TrimSpace(string(data))
	if common == "" {
		return gitDir
	}

	if !filepath.IsAbs(common) {
		common = filepath.Join(gitDir, common)
	}

	return filepath.Clean(common)
}
