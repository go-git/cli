package main

import (
	"errors"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
)

const (
	invalidNoSystemEnv   = "GIT_CONFIG_NOSYSTEM=maybe"
	invalidNoSystemError = "fatal: bad boolean environment value 'maybe' for 'GIT_CONFIG_NOSYSTEM'\n"
	invalidFlag          = "--definitely-invalid"
)

func TestConfigPath(t *testing.T) {
	t.Parallel()

	repo, home := newConfigRepo(t, "[p]\n\tdir = ~/sub\n\tabs = /etc\n\trel = x/y\n\tuser = ~someone/z\n")

	tests := []struct {
		name     string
		args     []string
		want     string
		wantCode int
	}{
		{
			name: "tilde expands",
			args: []string{cmdConfig, subGet, flagPath, keyPathDir},
			want: filepath.Join(home, "sub") + "\n",
		},
		{name: "legacy --path", args: []string{cmdConfig, flagPath, keyPathDir}, want: filepath.Join(home, "sub") + "\n"},
		{name: "without --path the value is literal", args: []string{cmdConfig, subGet, keyPathDir}, want: "~/sub\n"},
		{name: "absolute path is unchanged", args: []string{cmdConfig, subGet, flagPath, "p.abs"}, want: "/etc\n"},
		{name: "relative path is unchanged", args: []string{cmdConfig, subGet, flagPath, "p.rel"}, want: "x/y\n"},
		{name: "~user is rejected", args: []string{cmdConfig, subGet, flagPath, "p.user"}, wantCode: 128},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stdout, stderr, code := runConfig(t, repo, home, tc.args...)
			if code != tc.wantCode {
				t.Fatalf("gogit %v: exit %d, want %d (stderr %q)", tc.args, code, tc.wantCode, stderr)
			}

			if stdout != tc.want {
				t.Fatalf("gogit %v: stdout %q, want %q", tc.args, stdout, tc.want)
			}
		})
	}
}

func TestConfigPathExpandsExistingUser(t *testing.T) {
	t.Parallel()

	account, err := user.Current()
	if err != nil || account.Username == "" || account.HomeDir == "" || strings.Contains(account.Username, "/") {
		t.Skipf("current user is unavailable for ~user expansion: %v", err)
	}

	repo, home := newConfigRepo(t, "[p]\n\tdir = ~"+account.Username+"/sub\n")

	stdout, stderr, code := runConfig(t, repo, home, cmdConfig, subGet, flagPath, keyPathDir)
	if code != 0 {
		t.Fatalf("--path failed: exit %d, stderr %q", code, stderr)
	}

	if want := filepath.Join(account.HomeDir, "sub") + "\n"; stdout != want {
		t.Fatalf("expanded path = %q, want %q", stdout, want)
	}
}

func TestConfigNoSystemFalseKeepsSystemScopeEnabled(t *testing.T) {
	t.Parallel()

	repo, home := newConfigRepo(t, "")
	system := filepath.Join(t.TempDir(), "system.cfg")
	writeConfig(t, system, "[scope]\n\tvalue = SYSTEM\n")

	env := []string{
		"HOME=" + home,
		emptyXDGConfigHome,
		"GIT_CONFIG_NOSYSTEM=false",
		"GIT_CONFIG_SYSTEM=" + system,
	}

	stdout, stderr, err := runGogitEnv(t, repo, env, cmdConfig, subGet, "scope.value")
	if err != nil {
		t.Fatalf("system read failed: %v (stderr %q)", err, stderr)
	}

	if stdout != "SYSTEM\n" {
		t.Fatalf("system value = %q, want SYSTEM", stdout)
	}
}

func TestConfigRejectsInvalidBooleans(t *testing.T) {
	t.Parallel()

	t.Run("environment", func(t *testing.T) {
		t.Parallel()

		repo, home := newConfigRepo(t, "[user]\n\tname = Alice\n")
		env := append(configEnv(home), invalidNoSystemEnv)

		stdout, stderr, err := runGogitEnv(t, repo, env, cmdConfig, subGet, keyUserName)

		var exitErr *exec.ExitError

		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 128 || stdout != "" {
			t.Fatalf("invalid environment bool: stdout %q, stderr %q, err %v", stdout, stderr, err)
		}

		want := invalidNoSystemError
		if stderr != want {
			t.Fatalf("stderr = %q, want %q", stderr, want)
		}
	})

	t.Run("worktree extension", func(t *testing.T) {
		t.Parallel()

		repo, home := newConfigRepo(t, "[extensions]\n\tworktreeConfig = maybe\n")

		stdout, stderr, code := runConfig(t, repo, home, cmdConfig, subGet, keyUserName)
		if code != 128 || stdout != "" {
			t.Fatalf("invalid config bool: exit %d, stdout %q, stderr %q", code, stdout, stderr)
		}

		want := "fatal: bad boolean config value 'maybe' for 'extensions.worktreeconfig'\n"
		if stderr != want {
			t.Fatalf("stderr = %q, want %q", stderr, want)
		}
	})
}

func TestConfigValidatesNoSystemBeforeInvocationErrors(t *testing.T) {
	t.Parallel()

	repo, home := newConfigRepo(t, baseConfig)
	env := []string{"HOME=" + home, emptyXDGConfigHome, invalidNoSystemEnv}
	want := invalidNoSystemError

	for _, args := range [][]string{
		{cmdConfig},
		{cmdConfig, invalidFlag},
		{cmdConfig, subGet, "user"},
		{"-c", "invalid", cmdConfig, subGet, keyUserName},
	} {
		_, stderr, err := runGogitEnv(t, repo, env, args...)

		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 128 || stderr != want {
			t.Errorf("gogit %v: stderr %q, err %v", args, stderr, err)
		}
	}
}

func TestConfigInvalidCommandLineOverrideUsesFatalStatus(t *testing.T) {
	t.Parallel()

	repo, home := newConfigRepo(t, baseConfig)
	_, stderr, err := runGogitEnv(
		t,
		repo,
		configEnv(home),
		"-c",
		"invalid",
		cmdConfig,
		subGet,
		keyUserName,
	)

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 128 {
		t.Fatalf("invalid -c: stderr %q, err %v", stderr, err)
	}

	want := "error: key does not contain a section: invalid\n" +
		"fatal: unable to parse command-line config\n"
	if stderr != want {
		t.Fatalf("stderr = %q, want %q", stderr, want)
	}
}

func TestConfigRejectsInvalidNoSystemBeforeExplicitWrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	mkdirAll(t, home)

	target := filepath.Join(dir, "config")
	env := []string{"HOME=" + home, emptyXDGConfigHome, invalidNoSystemEnv}

	stdout, stderr, err := runGogitEnv(t, dir, env,
		cmdConfig, subSet, flagFile, target, "x.y", "value")

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 128 || stdout != "" {
		t.Fatalf("invalid environment bool: stdout %q, stderr %q, err %v", stdout, stderr, err)
	}

	if want := invalidNoSystemError; stderr != want {
		t.Fatalf("stderr = %q, want %q", stderr, want)
	}

	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("explicit config file was created: %v", err)
	}
}

func TestSystemConfigPathForGitInstallation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		goos       string
		executable string
		want       string
	}{
		{
			name:       "Homebrew",
			goos:       "darwin",
			executable: "/opt/homebrew/bin/git",
			want:       "/opt/homebrew/etc/gitconfig",
		},
		{
			name:       "Unix system",
			goos:       "linux",
			executable: "/usr/bin/git",
			want:       defaultSystemConfigFile,
		},
		{
			name:       "libexec",
			goos:       "darwin",
			executable: "/opt/local/libexec/git-core/git",
			want:       "/opt/local/etc/gitconfig",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := systemConfigPathForExecutable(tc.goos, tc.executable); got != tc.want {
				t.Fatalf("system path = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestConfigInvalidCombinations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{name: "--get with a value", args: []string{cmdConfig, flagGet, keyUserName, "extra"}},
		{name: "--add without a value", args: []string{cmdConfig, flagAdd, keyUserName}},
		{name: "--unset-all with a value", args: []string{cmdConfig, flagUnsetAll, keyUserName, "extra"}},
		{name: "--get and --unset-all together", args: []string{cmdConfig, flagGet, flagUnsetAll, keyUserName}},
		{name: "--local and --global together", args: []string{cmdConfig, subGet, flagLocal, flagGlobal, keyUserName}},
		{name: "--file and --global together", args: []string{cmdConfig, subGet, flagFile, "x", flagGlobal, keyUserName}},
		{name: "get without a key", args: []string{cmdConfig, subGet}},
		{name: "set without a value", args: []string{cmdConfig, subSet, keyUserName}},
		{name: "no arguments at all", args: []string{cmdConfig}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo, home := newConfigRepo(t, baseConfig)

			_, stderr, code := runConfig(t, repo, home, tc.args...)
			if code != 129 {
				t.Fatalf("gogit %v: exit %d, want 129 (stderr %q)", tc.args, code, stderr)
			}

			if got := readFileString(t, filepath.Join(repo, ".git", cmdConfig)); got != baseConfig {
				t.Fatalf("gogit %v modified the config:\n%s", tc.args, got)
			}
		})
	}
}

func TestConfigFlagErrorsUseUsageStatus(t *testing.T) {
	t.Parallel()

	repo, home := newConfigRepo(t, baseConfig)

	for _, args := range [][]string{
		{cmdConfig, invalidFlag},
		{cmdConfig, subGet, invalidFlag, keyUserName},
	} {
		_, stderr, code := runConfig(t, repo, home, args...)
		if code != 129 {
			t.Errorf("gogit %v: exit %d, want 129 (stderr %q)", args, code, stderr)
		}
	}
}

func TestConfigLinkedWorktree(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	home := filepath.Join(base, "home")
	main := filepath.Join(base, "main")
	wt := filepath.Join(base, "wt")
	wtGitDir := filepath.Join(main, ".git", "worktrees", "wt")

	mkdirAll(t, home)
	mkdirAll(t, wt)
	mkdirAll(t, wtGitDir)
	mkdirAll(t, filepath.Join(main, ".git", "objects"))
	mkdirAll(t, filepath.Join(main, ".git", "refs"))

	writeConfig(t, filepath.Join(main, ".git", "HEAD"), "ref: refs/heads/main\n")
	writeConfig(t, filepath.Join(main, ".git", cmdConfig),
		"[extensions]\n\tworktreeConfig = true\n[user]\n\tname = MAIN\n")
	writeConfig(t, filepath.Join(wt, ".git"), "gitdir: "+wtGitDir+"\n")
	writeConfig(t, filepath.Join(wtGitDir, "HEAD"), "ref: refs/heads/other\n")
	writeConfig(t, filepath.Join(wtGitDir, "commondir"), "../..\n")
	writeConfig(t, filepath.Join(wtGitDir, "config.worktree"), "[user]\n\tname = WORKTREE\n")

	stdout, stderr, code := runConfig(t, wt, home, cmdConfig, subGet, keyUserName)
	if code != 0 || stdout != "WORKTREE\n" {
		t.Fatalf("worktree read: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}

	// A write from a linked worktree belongs in the common directory.
	if _, stderr, code := runConfig(t, wt, home, cmdConfig, subSet, keyUserName, "CHANGED"); code != 0 {
		t.Fatalf("worktree write: exit %d, stderr %q", code, stderr)
	}

	if got, want := readFileString(t, filepath.Join(main, ".git", cmdConfig)),
		"[extensions]\n\tworktreeConfig = true\n[user]\n\tname = CHANGED\n"; got != want {
		t.Fatalf("common config = %q, want %q", got, want)
	}

	if _, err := os.Stat(filepath.Join(wtGitDir, cmdConfig)); !os.IsNotExist(err) {
		t.Fatal("a worktree write created a per-worktree config file")
	}
}

func TestConfigBareRepository(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	home := filepath.Join(base, "home")
	bare := filepath.Join(base, "bare.git")

	mkdirAll(t, home)
	mkdirAll(t, filepath.Join(bare, "objects"))
	mkdirAll(t, filepath.Join(bare, "refs"))

	writeConfig(t, filepath.Join(bare, "HEAD"), "ref: refs/heads/main\n")
	writeConfig(t, filepath.Join(bare, cmdConfig), "[user]\n\tname = BARE\n")

	stdout, stderr, code := runConfig(t, bare, home, cmdConfig, subGet, keyUserName)
	if code != 0 || stdout != "BARE\n" {
		t.Fatalf("bare read: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}

	if _, stderr, code := runConfig(t, bare, home, cmdConfig, subSet, keyUserName, "CHANGED"); code != 0 {
		t.Fatalf("bare write: exit %d, stderr %q", code, stderr)
	}

	if got, want := readFileString(t, filepath.Join(bare, cmdConfig)), "[user]\n\tname = CHANGED\n"; got != want {
		t.Fatalf("bare config = %q, want %q", got, want)
	}
}

func TestConfigOutsideRepository(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	home := filepath.Join(base, "home")

	mkdirAll(t, home)

	dir := filepath.Join(base, "plain")
	mkdirAll(t, dir)

	// -c overrides and global values still apply with no repository present.
	stdout, stderr, code := runConfig(t, dir, home, "-c", "only.cmd=C", cmdConfig, subGet, "only.cmd")
	if code != 0 || stdout != "C\n" {
		t.Fatalf("-c outside a repo: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}

	if _, _, code := runConfig(t, dir, home, cmdConfig, subGet, keyUserName); code != 1 {
		t.Fatalf("missing key outside a repo: exit %d, want 1", code)
	}

	// A local write outside a repository must fail rather than invent a file.
	if _, _, code := runConfig(t, dir, home, cmdConfig, subSet, keyUserName, "X"); code == 0 {
		t.Fatal("a local write outside a repository unexpectedly succeeded")
	}
}

func TestConfigRejectsMalformedGitFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		gitfile string
		want    func(string) string
	}{
		{
			name:    "invalid format",
			gitfile: "not a gitfile\n",
			want: func(repo string) string {
				return "fatal: invalid gitfile format: " + filepath.Join(repo, ".git") + "\n"
			},
		},
		{
			name:    "missing target",
			gitfile: "gitdir: missing\n",
			want: func(repo string) string {
				return "fatal: not a git repository: " + filepath.Join(repo, "missing") + "\n"
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			base := t.TempDir()
			repo := filepath.Join(base, "repo")
			home := filepath.Join(base, "home")

			mkdirAll(t, repo)
			mkdirAll(t, home)
			writeConfig(t, filepath.Join(repo, ".git"), tc.gitfile)
			writeConfig(t, filepath.Join(home, ".gitconfig"), "[user]\n\tname = GLOBAL\n")

			resolvedRepo, err := filepath.EvalSymlinks(repo)
			if err != nil {
				t.Fatal(err)
			}

			stdout, stderr, code := runConfig(t, repo, home, cmdConfig, subGet, keyUserName)
			if code != 128 || stdout != "" || stderr != tc.want(resolvedRepo) {
				t.Fatalf("malformed gitfile: exit %d, stdout %q, stderr %q", code, stdout, stderr)
			}
		})
	}
}

func TestConfigRejectsInvalidGitDirForLocalWrites(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	home := filepath.Join(base, "home")
	invalidGitDir := filepath.Join(base, "not-a-repository")

	mkdirAll(t, home)
	mkdirAll(t, invalidGitDir)

	env := append(configEnv(home), "GIT_DIR="+invalidGitDir)
	_, stderr, err := runGogitEnv(t, base, env, cmdConfig, subSet, "x.y", "changed")

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 128 {
		t.Fatalf("invalid GIT_DIR write: stderr %q, err %v", stderr, err)
	}

	if _, err := os.Stat(filepath.Join(invalidGitDir, cmdConfig)); !os.IsNotExist(err) {
		t.Fatalf("invalid GIT_DIR received a config file: %v", err)
	}
}

// TestConfigDiagnosticPaths pins how the config file is named in a
// diagnostic. git reports the path exactly as it resolved it rather than
// absolutising it, so the spelling depends on how the repository was found.
func TestConfigDiagnosticPaths(t *testing.T) {
	t.Parallel()

	const malformed = "[user]\n\tname = x\nbogus line\n"

	t.Run("discovered from the working tree", func(t *testing.T) {
		t.Parallel()

		repo, home := newConfigRepo(t, malformed)

		_, stderr, code := runConfig(t, repo, home, cmdConfig, subGet, keyUserName)
		assertDiagnostic(t, stderr, code, "fatal: bad config line 3 in file .git/config\n")
	})

	t.Run("discovered from a subdirectory", func(t *testing.T) {
		t.Parallel()

		repo, home := newConfigRepo(t, malformed)
		sub := filepath.Join(repo, "deep", "nested")
		mkdirAll(t, sub)

		// git chdirs to the top level before reporting, so the path stays
		// ".git/config" however deep the caller is.
		_, stderr, code := runConfig(t, sub, home, cmdConfig, subGet, keyUserName)
		assertDiagnostic(t, stderr, code, "fatal: bad config line 3 in file .git/config\n")
	})

	t.Run("bare repository", func(t *testing.T) {
		t.Parallel()

		base := t.TempDir()
		home := filepath.Join(base, "home")
		bare := filepath.Join(base, "bare.git")

		mkdirAll(t, home)
		mkdirAll(t, filepath.Join(bare, "objects"))
		mkdirAll(t, filepath.Join(bare, "refs"))
		writeConfig(t, filepath.Join(bare, "HEAD"), "ref: refs/heads/main\n")
		writeConfig(t, filepath.Join(bare, "config"), malformed)

		// A bare repository is its own git dir, which git names ".".
		_, stderr, code := runConfig(t, bare, home, cmdConfig, subGet, keyUserName)
		assertDiagnostic(t, stderr, code, "fatal: bad config line 3 in file ./config\n")
	})

	t.Run("linked worktree reports the common dir", func(t *testing.T) {
		t.Parallel()

		base := t.TempDir()
		home := filepath.Join(base, "home")
		main := filepath.Join(base, "main")
		wt := filepath.Join(base, "wt")
		wtGitDir := filepath.Join(main, ".git", "worktrees", "wt")

		mkdirAll(t, home)
		mkdirAll(t, wt)
		mkdirAll(t, wtGitDir)
		mkdirAll(t, filepath.Join(main, ".git", "objects"))
		mkdirAll(t, filepath.Join(main, ".git", "refs"))
		writeConfig(t, filepath.Join(main, ".git", "HEAD"), "ref: refs/heads/main\n")
		writeConfig(t, filepath.Join(main, ".git", "config"), malformed)
		writeConfig(t, filepath.Join(wt, ".git"), "gitdir: "+wtGitDir+"\n")
		writeConfig(t, filepath.Join(wtGitDir, "commondir"), "../..\n")
		writeConfig(t, filepath.Join(wtGitDir, "HEAD"), "ref: refs/heads/other\n")

		// Resolving through commondir yields an absolute path, and that is
		// what git reports.
		common, err := filepath.EvalSymlinks(filepath.Join(main, ".git"))
		if err != nil {
			t.Fatal(err)
		}

		want := "fatal: bad config line 3 in file " + filepath.Join(common, "config") + "\n"

		_, stderr, code := runConfig(t, wt, home, cmdConfig, subGet, keyUserName)
		assertDiagnostic(t, stderr, code, want)
	})

	t.Run("GIT_DIR is reported as given", func(t *testing.T) {
		t.Parallel()

		repo, home := newConfigRepo(t, malformed)

		stdout, stderr, err := runGogitEnv(t, repo,
			append(configEnv(home), "GIT_DIR=.git"), cmdConfig, subGet, keyUserName)
		if stdout != "" {
			t.Fatalf("stdout = %q, want empty", stdout)
		}

		if err == nil {
			t.Fatal("expected a non-zero exit")
		}

		if want := "fatal: bad config line 3 in file .git/config\n"; stderr != want {
			t.Fatalf("stderr = %q, want %q", stderr, want)
		}
	})

	t.Run("--file is reported as given", func(t *testing.T) {
		t.Parallel()

		repo, home := newConfigRepo(t, baseConfig)
		external := filepath.Join(t.TempDir(), "external.cfg")
		writeConfig(t, external, malformed)

		_, stderr, code := runConfig(t, repo, home, cmdConfig, subGet, flagFile, external, keyUserName)
		assertDiagnostic(t, stderr, code, "fatal: bad config line 3 in file "+external+"\n")
	})

	t.Run("a malformed global file is reported by full path", func(t *testing.T) {
		t.Parallel()

		repo, home := newConfigRepo(t, "[a]\n\tb = c\n")
		global := filepath.Join(home, ".gitconfig")
		writeConfig(t, global, malformed)

		_, stderr, code := runConfig(t, repo, home, cmdConfig, subGet, keyUserName)
		assertDiagnostic(t, stderr, code, "fatal: bad config line 3 in file "+global+"\n")
	})
}

func assertDiagnostic(t *testing.T, stderr string, code int, want string) {
	t.Helper()

	if code != 128 {
		t.Fatalf("exit %d, want 128 (stderr %q)", code, stderr)
	}

	if stderr != want {
		t.Fatalf("stderr = %q, want %q", stderr, want)
	}
}

// TestConfigPreservesKeySpelling covers the command level of git's rule that a
// variable is written with the spelling given on the command line. Upstream's
// t1300-config.sh gates on this at its "mixed case" case.
func TestConfigPreservesKeySpelling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		args []string
		want string
	}{
		{
			name: "new variable in an existing section",
			src:  "[section]\n\tpenguin = little blue\n",
			args: []string{cmdConfig, subSet, keyMixed, "BadPhysics"},
			want: "[section]\n\tpenguin = little blue\n\tMovie = BadPhysics\n",
		},
		{
			name: "legacy implicit set",
			src:  "[section]\n\tpenguin = little blue\n",
			args: []string{cmdConfig, keyMixed, "BadPhysics"},
			want: "[section]\n\tpenguin = little blue\n\tMovie = BadPhysics\n",
		},
		{
			name: "rewriting replaces the old spelling",
			src:  "[section]\n\tMovie = old\n",
			args: []string{cmdConfig, subSet, "Section.MOVIE", "new"},
			want: "[section]\n\tMOVIE = new\n",
		},
		{
			name: "a new section takes the command-line spelling",
			src:  "",
			args: []string{cmdConfig, subSet, "Core.MyVar", "V"},
			want: "[Core]\n\tMyVar = V\n",
		},
		{
			name: "--add keeps the command-line spelling",
			src:  "[core]\n\tx = 1\n",
			args: []string{cmdConfig, flagAdd, "Core.MyVar", "V"},
			want: "[core]\n\tx = 1\n\tMyVar = V\n",
		},
		{
			name: "a new subsection keeps every part's spelling",
			src:  "",
			args: []string{cmdConfig, subSet, "Remote.Origin.URL", "u"},
			want: "[Remote \"Origin\"]\n\tURL = u\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo, home := newConfigRepo(t, tc.src)
			path := filepath.Join(repo, ".git", "config")

			if _, stderr, code := runConfig(t, repo, home, tc.args...); code != 0 {
				t.Fatalf("gogit %v: exit %d, stderr %q", tc.args, code, stderr)
			}

			if got := readFileString(t, path); got != tc.want {
				t.Fatalf("gogit %v:\n--- got ---\n%s\n--- want ---\n%s", tc.args, got, tc.want)
			}
		})
	}
}

// TestConfigLookupIgnoresCase confirms reads still fold section and variable
// names while keeping subsection names case-sensitive.
func TestConfigLookupIgnoresCase(t *testing.T) {
	t.Parallel()

	repo, home := newConfigRepo(t, "[Section]\n\tMovie = BadPhysics\n[remote \"Origin\"]\n\tURL = u\n")

	for _, key := range []string{"section.movie", "SECTION.MOVIE", keyMixed} {
		stdout, _, code := runConfig(t, repo, home, cmdConfig, subGet, key)
		if code != 0 || stdout != "BadPhysics\n" {
			t.Errorf("get %s: exit %d, stdout %q", key, code, stdout)
		}
	}

	if stdout, _, code := runConfig(t, repo, home, cmdConfig, subGet, "REMOTE.Origin.URL"); code != 0 || stdout != "u\n" {
		t.Errorf("get REMOTE.Origin.URL: exit %d, stdout %q", code, stdout)
	}

	// The subsection is the one part that stays case-sensitive.
	if _, _, code := runConfig(t, repo, home, cmdConfig, subGet, "remote.origin.url"); code != 1 {
		t.Errorf("subsection lookup should be case-sensitive: exit %d, want 1", code)
	}
}
