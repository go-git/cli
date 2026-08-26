package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// configEnv isolates a test from the developer's real configuration: HOME
// points at a scratch directory and the system config is switched off, the
// same way upstream's test-lib.sh does it.
func configEnv(home string) []string {
	return []string{"HOME=" + home, "GIT_CONFIG_NOSYSTEM=1", "XDG_CONFIG_HOME="}
}

// runConfig runs gogit in dir and returns stdout, stderr and the exit status.
func runConfig(t *testing.T, dir, home string, args ...string) (string, string, int) {
	t.Helper()

	stdout, stderr, err := runGogitEnv(t, dir, configEnv(home), args...)

	code := 0

	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("gogit %v: %v", args, err)
		}

		code = ee.ExitCode()
	}

	return stdout, stderr, code
}

// newConfigRepo creates a repository with the given extra config content and
// returns the work tree and the isolated HOME.
func newConfigRepo(t *testing.T, extra string) (string, string) {
	t.Helper()

	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	home := filepath.Join(base, "home")

	mkdirAll(t, filepath.Join(repo, ".git"))
	mkdirAll(t, home)

	// A config file plus HEAD/objects/refs is enough for the config command,
	// and avoids depending on another gogit subcommand to set the test up.
	mkdirAll(t, filepath.Join(repo, ".git", "objects"))
	mkdirAll(t, filepath.Join(repo, ".git", "refs"))

	if err := os.WriteFile(filepath.Join(repo, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	writeConfig(t, filepath.Join(repo, ".git", "config"), extra)

	return repo, home
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeConfig(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFileString(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return string(data)
}

// Argument and key literals shared by the config tables.
const (
	cmdConfig = "config"
	subGet    = "get"
	subSet    = "set"
	subUnset  = "unset"

	flagGet      = "--get"
	flagAdd      = "--add"
	flagAll      = "--all"
	flagUnsetAll = "--unset-all"
	flagPath     = "--path"
	flagFile     = "--file"
	flagLocal    = "--local"
	flagGlobal   = "--global"

	keyUserName  = "user.name"
	keyFooBar    = "foo.bar"
	keyMissing   = "no.such"
	keyOriginURL = "remote.origin.url"
	keyPr        = "pr.k"

	valAuthor  = "A U Thor\n"
	valThree   = "three"
	keyPathDir = "p.dir"
	keyMixed   = "Section.Movie"
	valNewName = "New Name"
	overridePr = "pr.k=CMD"
)

const baseConfig = `[core]
	repositoryformatversion = 0
[user]
	name = A U Thor
[remote "origin"]
	url = https://example.com/x.git
[remote "team.one"]
	url = https://t.example/o.git
[foo]
	bar = one
	bar = two
[e]
	empty =
`

func TestConfigGet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		want     string
		wantCode int
	}{
		{name: "implicit get", args: []string{cmdConfig, keyUserName}, want: valAuthor},
		{name: "legacy --get", args: []string{cmdConfig, flagGet, keyUserName}, want: valAuthor},
		{name: "modern get", args: []string{cmdConfig, subGet, keyUserName}, want: valAuthor},
		{
			name: "subsection", args: []string{cmdConfig, subGet, keyOriginURL},
			want: "https://example.com/x.git\n",
		},
		{
			name: "subsection containing dots", args: []string{cmdConfig, subGet, "remote.team.one.url"},
			want: "https://t.example/o.git\n",
		},
		{name: "key is case-insensitive", args: []string{cmdConfig, subGet, "USER.NAME"}, want: valAuthor},
		{name: "multivalue reports the last", args: []string{cmdConfig, subGet, keyFooBar}, want: "two\n"},
		{name: "get --all", args: []string{cmdConfig, subGet, flagAll, keyFooBar}, want: "one\ntwo\n"},
		{name: "legacy --get-all", args: []string{cmdConfig, "--get-all", keyFooBar}, want: "one\ntwo\n"},

		// An explicitly empty value is not the same as a missing one.
		{name: "empty value prints a blank line", args: []string{cmdConfig, subGet, "e.empty"}, want: "\n"},
		{name: "missing key is silent", args: []string{cmdConfig, subGet, keyMissing}, want: "", wantCode: 1},
		{name: "missing key legacy form", args: []string{cmdConfig, keyMissing}, want: "", wantCode: 1},
		{
			name: "missing subsection", args: []string{cmdConfig, subGet, "remote.other.url"},
			want: "", wantCode: 1,
		},

		{name: "key without a section", args: []string{cmdConfig, subGet, "user"}, wantCode: 1},
		{name: "key without a variable", args: []string{cmdConfig, subGet, "user."}, wantCode: 1},
		{name: "invalid variable name", args: []string{cmdConfig, subGet, "a.1b"}, wantCode: 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo, home := newConfigRepo(t, baseConfig)

			stdout, _, code := runConfig(t, repo, home, tc.args...)
			if code != tc.wantCode {
				t.Fatalf("gogit %v: exit %d, want %d (stdout %q)", tc.args, code, tc.wantCode, stdout)
			}

			if stdout != tc.want {
				t.Fatalf("gogit %v: stdout %q, want %q", tc.args, stdout, tc.want)
			}
		})
	}
}

func TestConfigWrite(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		wantCode int
		// wantConfig is the expected config file, or "" to skip the check.
		wantConfig string
		// unchanged asserts the file was not touched at all.
		unchanged bool
	}{
		{
			name: "implicit set replaces in place",
			args: []string{cmdConfig, keyUserName, valNewName},
			wantConfig: strings.Replace(baseConfig,
				"name = A U Thor", "name = New Name", 1),
		},
		{
			name: "modern set replaces in place",
			args: []string{cmdConfig, subSet, keyUserName, valNewName},
			wantConfig: strings.Replace(baseConfig,
				"name = A U Thor", "name = New Name", 1),
		},
		{
			name: "set writes into the right subsection",
			args: []string{cmdConfig, subSet, keyOriginURL, "https://new/z.git"},
			wantConfig: strings.Replace(baseConfig,
				"url = https://example.com/x.git", "url = https://new/z.git", 1),
		},
		{
			name: "set on a dotted subsection",
			args: []string{cmdConfig, subSet, "remote.team.one.url", "https://new/o.git"},
			wantConfig: strings.Replace(baseConfig,
				"url = https://t.example/o.git", "url = https://new/o.git", 1),
		},
		{
			name:       "a new section is appended",
			args:       []string{cmdConfig, subSet, "new.key", "v"},
			wantConfig: baseConfig + "[new]\n\tkey = v\n",
		},
		{
			name: "a new subsection is appended",
			args: []string{cmdConfig, subSet, "remote.other.url", "https://o/p.git"},
			wantConfig: baseConfig +
				"[remote \"other\"]\n\turl = https://o/p.git\n",
		},
		{
			name: "--add appends without replacing",
			args: []string{cmdConfig, flagAdd, keyFooBar, valThree},
			wantConfig: strings.Replace(baseConfig,
				"\tbar = two\n", "\tbar = two\n\tbar = three\n", 1),
		},
		{
			name:      "set refuses to collapse multiple values",
			args:      []string{cmdConfig, subSet, keyFooBar, valThree},
			wantCode:  5,
			unchanged: true,
		},
		{
			name:      "implicit set refuses to collapse multiple values",
			args:      []string{cmdConfig, keyFooBar, valThree},
			wantCode:  5,
			unchanged: true,
		},
		{
			name: "set --all collapses them deliberately",
			args: []string{cmdConfig, subSet, flagAll, keyFooBar, valThree},
			wantConfig: strings.Replace(baseConfig,
				"\tbar = one\n\tbar = two\n", "\tbar = three\n", 1),
		},
		{
			name: "--replace-all collapses them deliberately",
			args: []string{cmdConfig, "--replace-all", keyFooBar, valThree},
			wantConfig: strings.Replace(baseConfig,
				"\tbar = one\n\tbar = two\n", "\tbar = three\n", 1),
		},
		{
			name: "unset removes one line",
			args: []string{cmdConfig, subUnset, keyUserName},
			wantConfig: strings.Replace(baseConfig,
				"[user]\n\tname = A U Thor\n", "", 1),
		},
		{
			name: "legacy --unset-all removes every occurrence",
			args: []string{cmdConfig, flagUnsetAll, keyFooBar},
			wantConfig: strings.Replace(baseConfig,
				"[foo]\n\tbar = one\n\tbar = two\n", "", 1),
		},
		{
			name: "unset on a subsection",
			args: []string{cmdConfig, subUnset, keyOriginURL},
			wantConfig: strings.Replace(baseConfig,
				"[remote \"origin\"]\n\turl = https://example.com/x.git\n", "", 1),
		},
		{
			name:      "unset of a missing key exits 5",
			args:      []string{cmdConfig, subUnset, keyMissing},
			wantCode:  5,
			unchanged: true,
		},
		{
			name:      "legacy --unset-all of a missing key exits 5",
			args:      []string{cmdConfig, flagUnsetAll, keyMissing},
			wantCode:  5,
			unchanged: true,
		},
		{
			name:      "unset refuses a multivalued key without --all",
			args:      []string{cmdConfig, subUnset, keyFooBar},
			wantCode:  5,
			unchanged: true,
		},
		{
			name:      "an invalid key never reaches the file",
			args:      []string{cmdConfig, subSet, "a_x.b", "v"},
			wantCode:  1,
			unchanged: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo, home := newConfigRepo(t, baseConfig)
			path := filepath.Join(repo, ".git", cmdConfig)

			_, stderr, code := runConfig(t, repo, home, tc.args...)
			if code != tc.wantCode {
				t.Fatalf("gogit %v: exit %d, want %d (stderr %q)", tc.args, code, tc.wantCode, stderr)
			}

			got := readFileString(t, path)

			switch {
			case tc.unchanged:
				if got != baseConfig {
					t.Fatalf("gogit %v modified the config:\n%s", tc.args, got)
				}
			case tc.wantConfig != "":
				if got != tc.wantConfig {
					t.Fatalf("gogit %v:\n--- got ---\n%s\n--- want ---\n%s", tc.args, got, tc.wantConfig)
				}
			}
		})
	}
}

// TestConfigSetPreservesCommentsAndLayout is the guarantee that made a
// format-preserving writer necessary: a canonical re-encode would delete
// every comment and blank line in the file.
func TestConfigSetPreservesCommentsAndLayout(t *testing.T) {
	t.Parallel()

	const src = `# a comment worth keeping
[user]
	; and an inline note
	name = Old Name
	email = a@b.c

[remote "origin"]
	url = https://x/y.git
`

	repo, home := newConfigRepo(t, src)
	path := filepath.Join(repo, ".git", cmdConfig)

	if _, stderr, code := runConfig(t, repo, home, cmdConfig, subSet, keyUserName, valNewName); code != 0 {
		t.Fatalf("set failed: exit %d, stderr %q", code, stderr)
	}

	want := strings.Replace(src, "name = Old Name", "name = New Name", 1)
	if got := readFileString(t, path); got != want {
		t.Fatalf("--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestConfigParseFailureNeverRewritesFile proves a read or parse failure
// cannot truncate or canonicalise the config.
func TestConfigParseFailureNeverRewritesFile(t *testing.T) {
	t.Parallel()

	const src = `# keep me
[user]
	name = Old Name

bogus line here
`

	repo, home := newConfigRepo(t, src)
	path := filepath.Join(repo, ".git", cmdConfig)

	for _, args := range [][]string{
		{cmdConfig, subGet, keyUserName},
		{cmdConfig, subSet, keyUserName, valNewName},
		{cmdConfig, subUnset, keyUserName},
		{cmdConfig, flagAdd, keyUserName, "Another"},
	} {
		_, stderr, code := runConfig(t, repo, home, args...)
		if code != 128 {
			t.Errorf("gogit %v: exit %d, want 128 (stderr %q)", args, code, stderr)
		}

		// git names the repository config relative to the top level.
		if want := "fatal: bad config line 5 in file .git/config\n"; stderr != want {
			t.Errorf("gogit %v: stderr %q, want %q", args, stderr, want)
		}

		if got := readFileString(t, path); got != src {
			t.Fatalf("gogit %v rewrote a malformed config:\n%s", args, got)
		}
	}
}

func TestConfigScopePrecedence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		want     string
		wantCode int
	}{
		{name: "local wins over global", args: []string{cmdConfig, subGet, keyPr}, want: "LOCAL\n"},
		{
			name: "-c wins over local",
			args: []string{"-c", overridePr, cmdConfig, subGet, keyPr}, want: "CMD\n",
		},
		{
			name: "--local ignores global and -c",
			args: []string{"-c", overridePr, cmdConfig, subGet, flagLocal, keyPr}, want: "LOCAL\n",
		},
		{name: "--global selects the global file", args: []string{cmdConfig, flagGlobal, keyPr}, want: "GLOBAL\n"},
		{name: "global-only key is visible by default", args: []string{cmdConfig, subGet, "g.only"}, want: "FROMGLOBAL\n"},
		{
			name: "--local does not see a global-only key",
			args: []string{cmdConfig, subGet, flagLocal, "g.only"}, wantCode: 1,
		},
		{
			name: "-c with a subsection",
			args: []string{"-c", "remote.origin.url=CMD", cmdConfig, subGet, keyOriginURL}, want: "CMD\n",
		},
		{
			name: "-c contributes to --all in precedence order",
			args: []string{"-c", overridePr, cmdConfig, subGet, flagAll, keyPr}, want: "GLOBAL\nLOCAL\nCMD\n",
		},
		{
			name: "-c can set an empty value",
			args: []string{"-c", "pr.k=", cmdConfig, subGet, keyPr}, want: "\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo, home := newConfigRepo(t, "[pr]\n\tk = LOCAL\n")
			writeConfig(t, filepath.Join(home, ".gitconfig"), "[pr]\n\tk = GLOBAL\n[g]\n\tonly = FROMGLOBAL\n")

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

func TestConfigWritesDefaultToLocalScope(t *testing.T) {
	t.Parallel()

	repo, home := newConfigRepo(t, "[pr]\n\tk = LOCAL\n")
	global := filepath.Join(home, ".gitconfig")
	writeConfig(t, global, "[pr]\n\tk = GLOBAL\n")

	if _, stderr, code := runConfig(t, repo, home, cmdConfig, subSet, keyPr, "CHANGED"); code != 0 {
		t.Fatalf("set failed: exit %d, stderr %q", code, stderr)
	}

	if got, want := readFileString(t, filepath.Join(repo, ".git", cmdConfig)), "[pr]\n\tk = CHANGED\n"; got != want {
		t.Fatalf("local config = %q, want %q", got, want)
	}

	if got, want := readFileString(t, global), "[pr]\n\tk = GLOBAL\n"; got != want {
		t.Fatalf("a default write touched the global config: %q", got)
	}
}

func TestConfigGlobalWrite(t *testing.T) {
	t.Parallel()

	repo, home := newConfigRepo(t, "[pr]\n\tk = LOCAL\n")
	global := filepath.Join(home, ".gitconfig")
	writeConfig(t, global, "[pr]\n\tk = GLOBAL\n")

	if _, stderr, code := runConfig(t, repo, home, cmdConfig, subSet, flagGlobal, keyPr, "CHANGED"); code != 0 {
		t.Fatalf("set --global failed: exit %d, stderr %q", code, stderr)
	}

	if got, want := readFileString(t, global), "[pr]\n\tk = CHANGED\n"; got != want {
		t.Fatalf("global config = %q, want %q", got, want)
	}

	if got, want := readFileString(t, filepath.Join(repo, ".git", cmdConfig)), "[pr]\n\tk = LOCAL\n"; got != want {
		t.Fatalf("--global write touched the local config: %q", got)
	}
}

func TestConfigFile(t *testing.T) {
	t.Parallel()

	repo, home := newConfigRepo(t, baseConfig)
	external := filepath.Join(t.TempDir(), "external.cfg")
	writeConfig(t, external, "[a]\n\tb = c\n")

	stdout, stderr, code := runConfig(t, repo, home, cmdConfig, subGet, flagFile, external, "a.b")
	if code != 0 || stdout != "c\n" {
		t.Fatalf("--file read: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}

	// --file must not fall back to the repository.
	if _, _, code := runConfig(t, repo, home, cmdConfig, subGet, flagFile, external, keyUserName); code != 1 {
		t.Fatalf("--file leaked repository values: exit %d", code)
	}

	if _, stderr, code := runConfig(t, repo, home, cmdConfig, subSet, flagFile, external, "a.b", "d"); code != 0 {
		t.Fatalf("--file write: exit %d, stderr %q", code, stderr)
	}

	if got, want := readFileString(t, external), "[a]\n\tb = d\n"; got != want {
		t.Fatalf("external file = %q, want %q", got, want)
	}

	if got := readFileString(t, filepath.Join(repo, ".git", cmdConfig)); got != baseConfig {
		t.Fatalf("--file write touched the repository config:\n%s", got)
	}

	// A --file write outside any repository still works.
	fresh := filepath.Join(t.TempDir(), "fresh.cfg")
	if _, stderr, code := runConfig(t, t.TempDir(), home, cmdConfig, subSet, flagFile, fresh, "x.y", "z"); code != 0 {
		t.Fatalf("--file write outside a repo: exit %d, stderr %q", code, stderr)
	}

	if got, want := readFileString(t, fresh), "[x]\n\ty = z\n"; got != want {
		t.Fatalf("new file = %q, want %q", got, want)
	}
}

func TestConfigUnsetRemovesSameLineSection(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	home := filepath.Join(base, "home")
	file := filepath.Join(base, "config")

	mkdirAll(t, home)
	writeConfig(t, file, "[a] value = old # remove with the value\n[b]\n\tother = kept\n")

	_, stderr, err := runGogitEnv(t, base, configEnv(home), cmdConfig, subUnset, flagFile, file, "a.value")
	if err != nil {
		t.Fatalf("unset failed: %v (stderr %q)", err, stderr)
	}

	if got, want := readFileString(t, file), "[b]\n\tother = kept\n"; got != want {
		t.Fatalf("config after unset = %q, want %q", got, want)
	}
}

func TestConfigIgnoresUnreadableOptionalGlobal(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	home := filepath.Join(base, "home")
	global := filepath.Join(base, "unreadable-global")

	mkdirAll(t, home)

	if err := os.Mkdir(global, 0o755); err != nil {
		t.Fatal(err)
	}

	env := append(configEnv(home), "GIT_CONFIG_GLOBAL="+global)

	stdout, stderr, err := runGogitEnv(t, base, env, cmdConfig, subGet, "missing.key")
	if stdout != "" || stderr != "" {
		t.Fatalf("unreadable optional global produced stdout=%q stderr=%q", stdout, stderr)
	}

	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 1 {
		t.Fatalf("exit error = %v, want status 1", err)
	}
}

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

			_, _, code := runConfig(t, repo, home, tc.args...)
			if code == 0 {
				t.Fatalf("gogit %v unexpectedly succeeded", tc.args)
			}

			if got := readFileString(t, filepath.Join(repo, ".git", cmdConfig)); got != baseConfig {
				t.Fatalf("gogit %v modified the config:\n%s", tc.args, got)
			}
		})
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
	writeConfig(t, filepath.Join(main, ".git", cmdConfig), "[user]\n\tname = MAIN\n")
	writeConfig(t, filepath.Join(wt, ".git"), "gitdir: "+wtGitDir+"\n")
	writeConfig(t, filepath.Join(wtGitDir, "HEAD"), "ref: refs/heads/other\n")
	writeConfig(t, filepath.Join(wtGitDir, "commondir"), "../..\n")

	stdout, stderr, code := runConfig(t, wt, home, cmdConfig, subGet, keyUserName)
	if code != 0 || stdout != "MAIN\n" {
		t.Fatalf("worktree read: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}

	// A write from a linked worktree belongs in the common directory.
	if _, stderr, code := runConfig(t, wt, home, cmdConfig, subSet, keyUserName, "CHANGED"); code != 0 {
		t.Fatalf("worktree write: exit %d, stderr %q", code, stderr)
	}

	if got, want := readFileString(t, filepath.Join(main, ".git", cmdConfig)), "[user]\n\tname = CHANGED\n"; got != want {
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

		// Resolving through commondir yields an absolute path, and that is
		// what git reports.
		want := "fatal: bad config line 3 in file " + filepath.Join(main, ".git", "config") + "\n"

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
