package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	gitconfig "github.com/go-git/cli/internal/plumbing/format/config"
)

const defaultSystemConfigFile = "/etc/gitconfig"

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
	location configFile
	file     *gitconfig.File

	overrides []configOverride
	includes  bool
}

// readSources returns the files to consult, lowest precedence first.
//
// A location flag limits the sources to one scope. Otherwise git's default
// precedence applies.
func readSources(o *configOpts) ([]configSource, error) {
	selected, err := selectConfigLocation(o)
	if err != nil {
		return nil, err
	}

	o = &selected

	if err := validateNoSystem(); err != nil {
		return nil, err
	}

	if files, ok, err := explicitReadFiles(o); err != nil {
		return nil, configRepositoryError(err)
	} else if ok {
		sources := make([]configSource, 0, len(files))
		for _, file := range files {
			src, err := loadSource(file)
			if err != nil {
				return nil, err
			}

			sources = append(sources, src)
		}

		return sources, nil
	}

	sources := make([]configSource, 0)

	if p, ok, err := systemConfigPath(); err != nil {
		return nil, err
	} else if ok {
		src, loaded, err := loadOptionalSource(absoluteFile(p))
		if err != nil {
			return nil, err
		}

		if loaded {
			src.includes = true
			sources = append(sources, src)
		}
	}

	for _, p := range globalConfigPaths() {
		src, loaded, err := loadOptionalSource(absoluteFile(p))
		if err != nil {
			return nil, err
		}

		if loaded {
			src.includes = true
			sources = append(sources, src)
		}
	}

	sources, err = appendLocalConfigSources(sources)
	if err != nil {
		return nil, err
	}

	if len(configOverrideList) > 0 {
		sources = append(sources, configSource{
			overrides: configOverrideList,
			includes:  true,
		})
	}

	return sources, nil
}

func appendLocalConfigSources(sources []configSource) ([]configSource, error) {
	// Being outside a repository is not an error for a default read: -c
	// overrides and the global files still apply, as they do in git.
	file, err := localConfigFile()
	if errors.Is(err, errNoRepository) {
		return sources, nil
	}

	if err != nil {
		return nil, configRepositoryError(err)
	}

	source, err := loadSource(file)
	if err != nil {
		return nil, err
	}

	source.includes = true
	sources = append(sources, source)

	worktree, enabled, err := worktreeConfigFile(source.file)
	if err != nil || !enabled {
		return sources, err
	}

	worktreeSource, err := loadSource(worktree)
	if err != nil {
		return nil, err
	}

	worktreeSource.includes = true

	return append(sources, worktreeSource), nil
}

// absoluteFile names a file that git reports by its full path, which is how
// it reports every file it did not discover by walking up from the cwd.
func absoluteFile(path string) configFile {
	return configFile{path: path, display: path}
}

// writeTarget returns the single file a mutation applies to. Writes default
// to the repository config, never to the merged view.
func writeTarget(o *configOpts) (configFile, error) {
	selected, err := selectConfigLocation(o)
	if err != nil {
		return configFile{}, err
	}

	o = &selected

	if err := validateNoSystem(); err != nil {
		return configFile{}, err
	}

	if file, ok, err := explicitWriteLocation(o); err != nil {
		return configFile{}, configRepositoryError(err)
	} else if ok {
		return file, nil
	}

	file, err := localConfigFile()

	return file, configRepositoryError(err)
}

func configRepositoryError(err error) error {
	if err == nil {
		return nil
	}

	var exitErr *gitExitError
	if errors.As(err, &exitErr) {
		return err
	}

	return &gitExitError{code: exitFatal, msg: "fatal: " + err.Error()}
}

// selectConfigLocation shares file-selector presence and conflicts between reads and writes.
func selectConfigLocation(o *configOpts) (configOpts, error) {
	selected := *o
	if path, present := os.LookupEnv("GIT_CONFIG"); present {
		if o.local || o.global || o.system {
			return configOpts{}, usageError("only one config file at a time")
		}

		if !o.fileSet {
			selected.file, selected.fileSet = path, true
		}
	}

	return selected, nil
}

func explicitReadFiles(o *configOpts) ([]configFile, bool, error) {
	switch {
	case o.fileSet:
		if o.file == "-" {
			return []configFile{{path: "-", display: "standard input"}}, true, nil
		}

		return []configFile{absoluteFile(o.file)}, true, nil

	case o.local:
		f, err := localConfigFile()

		return []configFile{f}, true, err

	case o.global:
		paths := globalConfigPaths()

		files := make([]configFile, 0, len(paths))
		for _, path := range paths {
			files = append(files, absoluteFile(path))
		}

		return files, true, nil

	case o.system:
		return []configFile{absoluteFile(systemConfigFilePath())}, true, nil
	}

	return nil, false, nil
}

// explicitWriteLocation resolves the single file changed by an explicit scope.
func explicitWriteLocation(o *configOpts) (configFile, bool, error) {
	switch {
	case o.fileSet:
		if o.file == "" {
			return configFile{}, true, &gitExitError{
				code: exitCannotWrite,
				msg:  "error: could not write config file : Is a directory",
			}
		}

		if o.file == "-" {
			return configFile{}, true, &gitExitError{
				code: exitFatal,
				msg:  "fatal: writing to stdin is not supported",
			}
		}

		return absoluteFile(o.file), true, nil

	case o.local:
		f, err := localConfigFile()

		return f, true, err

	case o.global:
		paths := globalConfigPaths()
		for _, path := range slices.Backward(paths) {
			if _, err := os.Stat(path); err == nil {
				return absoluteFile(path), true, nil
			}
		}

		if len(paths) == 0 {
			return configFile{}, true, errors.New("no global config file available")
		}

		return absoluteFile(paths[len(paths)-1]), true, nil

	case o.system:
		return absoluteFile(systemConfigFilePath()), true, nil
	}

	return configFile{}, false, nil
}

func loadSource(cf configFile) (configSource, error) {
	if cf.path == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return configSource{}, err
		}

		f, err := gitconfig.Parse(data)
		if err != nil {
			return configSource{}, configReadError(cf, err)
		}

		return configSource{location: cf, file: f}, nil
	}

	f, err := gitconfig.ReadFile(cf.path)
	if err != nil {
		return configSource{}, configReadError(cf, err)
	}

	return configSource{location: cf, file: f}, nil
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

	return configSource{location: cf, file: f}, true, nil
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
func systemConfigPath() (string, bool, error) {
	noSystem, err := noSystemConfig()
	if err != nil {
		return "", false, err
	}

	if noSystem {
		return "", false, nil
	}

	path := systemConfigFilePath()

	return path, path != "" && path != os.DevNull, nil
}

func systemConfigFilePath() string {
	if path, ok := os.LookupEnv("GIT_CONFIG_SYSTEM"); ok {
		return path
	}

	return defaultSystemConfigPath()
}

func validateNoSystem() error {
	_, err := noSystemConfig()

	return err
}

func noSystemConfig() (bool, error) {
	value := os.Getenv("GIT_CONFIG_NOSYSTEM")
	noSystem, valid := gitBool(value, false)

	if !valid {
		return false, &gitExitError{
			code: exitFatal,
			msg: fmt.Sprintf(
				"fatal: bad boolean environment value '%s' for 'GIT_CONFIG_NOSYSTEM'",
				value,
			),
		}
	}

	return noSystem, nil
}

func defaultSystemConfigPath() string {
	gitPath, err := exec.LookPath("git")
	if err == nil && !sameExecutable(gitPath) {
		if path := systemConfigPathForExecutable(runtime.GOOS, gitPath); path != "" {
			return path
		}
	}

	if runtime.GOOS == "windows" {
		if programFiles := os.Getenv("PROGRAMFILES"); programFiles != "" {
			return filepath.Join(programFiles, "Git", "etc", "gitconfig")
		}
	}

	return defaultSystemConfigFile
}

func sameExecutable(path string) bool {
	executable, err := os.Executable()
	if err != nil {
		return false
	}

	want, err := os.Stat(executable)
	if err != nil {
		return false
	}

	got, err := os.Stat(path)

	return err == nil && os.SameFile(want, got)
}

func systemConfigPathForExecutable(goos, executable string) string {
	dir := filepath.Dir(executable)

	var prefix string

	switch filepath.Base(dir) {
	case "bin", "cmd":
		prefix = filepath.Dir(dir)
	case "git-core":
		prefix = filepath.Dir(filepath.Dir(dir))
	default:
		return ""
	}

	if goos != "windows" && prefix == "/usr" {
		return defaultSystemConfigFile
	}

	return filepath.Join(prefix, "etc", "gitconfig")
}

// localConfigFile returns the repository's config file. For a linked worktree
// this is the common directory's config, not the worktree's own git dir.
func localConfigFile() (configFile, error) {
	gitDir, display, err := discoverGitDir()
	if err != nil {
		return configFile{}, err
	}

	common, err := commonGitDir(gitDir)
	if err != nil {
		return configFile{}, err
	}

	if common != gitDir {
		// Resolving through commondir yields an absolute path, and that is
		// what git reports for a linked worktree.
		display = common
	}

	return configFile{
		path: common + "/config",
		// Concatenated rather than joined: git names a bare repository's
		// config "./config", which filepath.Join would clean to "config".
		display: display + "/config",
	}, nil
}

func worktreeConfigFile(local *gitconfig.File) (configFile, bool, error) {
	key := gitconfig.Key{Section: "extensions", Name: "worktreeConfig"}

	var (
		value           string
		implicit, found bool
	)

	for _, entry := range local.Entries() {
		if entry.Key.Matches(key) {
			value, implicit, found = entry.Value, entry.Implicit, true
		}
	}

	enabled := false

	if found {
		var valid bool

		enabled, valid = gitBool(value, implicit)
		if !valid {
			return configFile{}, false, &gitExitError{
				code: exitFatal,
				msg: fmt.Sprintf(
					"fatal: bad boolean config value '%s' for 'extensions.worktreeconfig'",
					value,
				),
			}
		}
	}

	if !enabled {
		return configFile{}, false, nil
	}

	gitDir, display, err := discoverGitDir()
	if err != nil {
		return configFile{}, false, err
	}

	return configFile{
		path:    gitDir + "/config.worktree",
		display: display + "/config.worktree",
	}, true, nil
}

func gitBool(value string, implicit bool) (bool, bool) {
	if implicit {
		return true, true
	}

	switch strings.ToLower(value) {
	case "true", "yes", "on", "1":
		return true, true
	case "", "false", "no", "off", "0":
		return false, true
	}

	n, err := strconv.ParseInt(value, 10, 64)

	return n != 0, err == nil
}

// commonGitDir follows a linked worktree's `commondir` pointer back to the
// main git directory, which is where shared state such as config lives.
func commonGitDir(gitDir string) (string, error) {
	data, err := os.ReadFile(gitDir + "/commondir")
	if os.IsNotExist(err) {
		return gitDir, nil
	}

	if err != nil {
		return "", err
	}

	common := strings.TrimRight(string(data), "\r\n")
	if common == "" {
		return "", errors.New("empty commondir")
	}

	if !filepath.IsAbs(common) {
		common = gitDir + "/" + common
	}

	return filepath.EvalSymlinks(common)
}
