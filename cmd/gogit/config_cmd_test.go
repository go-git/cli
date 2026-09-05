package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const emptyXDGConfigHome = "XDG_CONFIG_HOME="

// configEnv isolates a test from the developer's real configuration: HOME
// points at a scratch directory and the system config is switched off, the
// same way upstream's test-lib.sh does it.
func configEnv(home string) []string {
	return []string{"HOME=" + home, "GIT_CONFIG_NOSYSTEM=1", emptyXDGConfigHome}
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
	changedPr  = "[pr]\n\tk = CHANGED\n"
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

func TestConfigWritesFlagLikeValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		key  string
	}{
		{name: "legacy dash value", args: []string{cmdConfig, "value.legacy", "-value"}, key: "value.legacy"},
		{name: "legacy known flag value", args: []string{cmdConfig, "value.legacyflag", flagGlobal}, key: "value.legacyflag"},
		{name: "modern dash value", args: []string{cmdConfig, subSet, "value.modern", "-value"}, key: "value.modern"},
		{
			name: "modern known flag value",
			args: []string{cmdConfig, subSet, "value.modernflag", flagGlobal},
			key:  "value.modernflag",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo, home := newConfigRepo(t, "")
			if _, stderr, code := runConfig(t, repo, home, tc.args...); code != 0 {
				t.Fatalf("write: exit %d, stderr %q", code, stderr)
			}

			stdout, stderr, code := runConfig(t, repo, home, cmdConfig, subGet, tc.key)
			if code != 0 || stdout != tc.args[len(tc.args)-1]+"\n" {
				t.Fatalf("read: exit %d, stdout %q, stderr %q", code, stdout, stderr)
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
		{
			name: "-c accepts an implicit boolean",
			args: []string{"-c", "feature.enabled", cmdConfig, subGet, "feature.enabled"}, want: "\n",
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

func TestConfigIncludes(t *testing.T) {
	t.Parallel()

	repo, home := newConfigRepo(t, "")
	included := filepath.Join(repo, ".git", "included.cfg")
	writeConfig(t, included, "[order]\n\tvalue = INCLUDED\n")

	gitDirPattern := filepath.ToSlash(filepath.Join(repo, ".git"))
	writeConfig(t, filepath.Join(repo, ".git", "config"), fmt.Sprintf(`[order]
	value = BEFORE
[include]
	path = included.cfg
[includeIf "gitdir:%s"]
	path = included.cfg
[includeIf "onbranch:main"]
	path = included.cfg
[includeIf "hasconfig:remote.*.url:https://example.com/**"]
	path = included.cfg
[remote "origin"]
	url = https://example.com/repo
[order]
	value = AFTER
`, gitDirPattern))

	stdout, stderr, code := runConfig(t, repo, home, cmdConfig, subGet, flagAll, "order.value")
	if code != 0 {
		t.Fatalf("included read: exit %d, stderr %q", code, stderr)
	}

	if want := "BEFORE\n" + strings.Repeat("INCLUDED\n", 4) + "AFTER\n"; stdout != want {
		t.Fatalf("included values = %q, want %q", stdout, want)
	}

	stdout, _, code = runConfig(t, repo, home, cmdConfig, subGet, flagFile,
		filepath.Join(repo, ".git", "config"), flagAll, "order.value")
	if code != 0 || stdout != "BEFORE\nAFTER\n" {
		t.Fatalf("--file should not follow includes by default: exit %d, stdout %q", code, stdout)
	}
}

func TestConfigCommandLineIncludes(t *testing.T) {
	t.Parallel()

	repo, home := newConfigRepo(t, "")
	included := filepath.Join(repo, "command-line.cfg")
	writeConfig(t, included, "[x]\n\ty = from-include\n")

	tests := []struct {
		name     string
		override string
		wantCode int
	}{
		{name: "unconditional", override: "include.path=" + included},
		{name: "relative to working directory", override: "include.path=command-line.cfg", wantCode: 128},
		{name: "conditional", override: "includeIf.onbranch:main.path=" + included},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stdout, stderr, code := runConfig(t, repo, home,
				"-c", tc.override, cmdConfig, subGet, "x.y")
			if code != tc.wantCode || (code == 0 && stdout != "from-include\n") || (code != 0 && stdout != "") {
				t.Fatalf("command-line include: exit %d, stdout %q, stderr %q", code, stdout, stderr)
			}
		})
	}

	stdout, stderr, code := runConfig(t, repo, home,
		"-c", "x.y=before",
		"-c", "include.path="+included,
		"-c", "x.y=after",
		cmdConfig, subGet, flagAll, "x.y")
	if code != 0 || stdout != "before\nfrom-include\nafter\n" {
		t.Fatalf("ordered command-line include: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestConfigCommandLineIncludeReadsStdinOnce(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	home := filepath.Join(base, "home")
	mkdirAll(t, home)

	stdout, stderr, err := runGogitEnvStdin(
		t,
		base,
		configEnv(home),
		"[x]\n\ty = hit\n",
		"-c",
		"include.path=/dev/stdin",
		cmdConfig,
		subGet,
		"x.y",
	)
	if err != nil || stdout != "hit\n" || stderr != "" {
		t.Fatalf("stdin include: stdout %q, stderr %q, err %v", stdout, stderr, err)
	}
}

func TestConfigRejectsRemoteURLFromHasConfigInclude(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		conditional string
		nested      string
	}{
		{
			name:        "direct",
			conditional: "[remote \"other\"]\n\turl = https://other/repo\n[x]\n\ty = included\n",
		},
		{
			name:        "nested",
			conditional: "[include]\n\tpath = nested.cfg\n[x]\n\ty = included\n",
			nested:      "[remote \"other\"]\n\turl = https://other/repo\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo, home := newConfigRepo(t, `[remote "origin"]
	url = https://example/repo
[includeIf "hasconfig:remote.*.url:https://example/**"]
	path = conditional.cfg
`)
			writeConfig(t, filepath.Join(repo, ".git", "conditional.cfg"), tc.conditional)

			if tc.nested != "" {
				writeConfig(t, filepath.Join(repo, ".git", "nested.cfg"), tc.nested)
			}

			stdout, stderr, code := runConfig(t, repo, home, cmdConfig, subGet, "x.y")
			if code != 128 || stdout != "" {
				t.Fatalf("hasconfig include: exit %d, stdout %q, stderr %q", code, stdout, stderr)
			}

			want := "fatal: remote URLs cannot be configured in file directly or indirectly " +
				"included by includeIf.hasconfig:remote.*.url\n"
			if stderr != want {
				t.Fatalf("stderr = %q, want %q", stderr, want)
			}
		})
	}
}

func TestGitWildMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		pattern     string
		value       string
		insensitive bool
		want        bool
	}{
		{pattern: "**/group/**", value: "/tmp/group/repo/.git", want: true},
		{pattern: "feature/**", value: "feature/team/topic", want: true},
		{pattern: "a/**/c", value: "a/c", want: true},
		{pattern: "a/**/c", value: "a/b/d/c", want: true},
		{pattern: "a**c", value: "ab/c", want: false},
		{pattern: "a**c", value: "abbc", want: true},
		{pattern: "release/[0-9]?", value: "release/12", want: true},
		{pattern: "a[!x]b", value: "a/b", want: false},
		{pattern: "a[!x]b", value: "ayb", want: true},
		{pattern: "a[]]b", value: "a]b", want: true},
		{pattern: `a\*b`, value: "a*b", want: true},
		{pattern: `[[:alpha:]]`, value: "z", want: true},
		{pattern: `[[:digit:]]`, value: "z", want: false},
		{pattern: "repo", value: "REPO", insensitive: true, want: true},
		{pattern: "repo", value: "REPO", want: false},
	}

	for _, test := range tests {
		if got := gitWildMatch(test.pattern, test.value, test.insensitive); got != test.want {
			t.Errorf("gitWildMatch(%q, %q) = %v, want %v", test.pattern, test.value, got, test.want)
		}
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

	if got, want := readFileString(t, filepath.Join(repo, ".git", cmdConfig)), changedPr; got != want {
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

	if got, want := readFileString(t, global), changedPr; got != want {
		t.Fatalf("global config = %q, want %q", got, want)
	}

	if got, want := readFileString(t, filepath.Join(repo, ".git", cmdConfig)), "[pr]\n\tk = LOCAL\n"; got != want {
		t.Fatalf("--global write touched the local config: %q", got)
	}
}

func TestConfigGlobalPrefersHomeFileOverXDG(t *testing.T) {
	t.Parallel()

	repo, home := newConfigRepo(t, "")
	xdg := filepath.Join(home, ".config", "git", "config")
	global := filepath.Join(home, ".gitconfig")

	mkdirAll(t, filepath.Dir(xdg))
	writeConfig(t, xdg, "[pr]\n\tk = XDG\n")
	writeConfig(t, global, "[pr]\n\tk = HOME\n")

	if _, stderr, code := runConfig(t, repo, home, cmdConfig, subSet, flagGlobal, keyPr, "CHANGED"); code != 0 {
		t.Fatalf("set --global failed: exit %d, stderr %q", code, stderr)
	}

	if got, want := readFileString(t, global), changedPr; got != want {
		t.Fatalf("home config = %q, want %q", got, want)
	}

	if got, want := readFileString(t, xdg), "[pr]\n\tk = XDG\n"; got != want {
		t.Fatalf("XDG config was modified: got %q, want %q", got, want)
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

func TestConfigEmptyFileDoesNotFallBackToRepository(t *testing.T) {
	t.Parallel()

	repo, home := newConfigRepo(t, baseConfig)

	stdout, stderr, code := runConfig(t, repo, home, cmdConfig, subGet, flagFile, "", keyUserName)
	if code != 1 || stdout != "" || stderr != "" {
		t.Fatalf("empty --file read: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}

	before := readFileString(t, filepath.Join(repo, ".git", cmdConfig))

	_, _, code = runConfig(t, repo, home, cmdConfig, subSet, flagFile, "", "x.y", "changed")
	if code == 0 {
		t.Fatal("empty --file write succeeded")
	}

	if got := readFileString(t, filepath.Join(repo, ".git", cmdConfig)); got != before {
		t.Fatalf("empty --file write changed repository config:\n%s", got)
	}
}

func TestConfigLockFailureUsesGitStatus(t *testing.T) {
	t.Parallel()

	repo, home := newConfigRepo(t, baseConfig)
	lockPath := filepath.Join(repo, ".git", "config.lock")
	writeConfig(t, lockPath, "held")

	before := readFileString(t, filepath.Join(repo, ".git", cmdConfig))

	_, stderr, code := runConfig(t, repo, home, cmdConfig, subSet, keyUserName, "Changed")
	if code != 255 {
		t.Fatalf("locked write: exit %d, want 255 (stderr %q)", code, stderr)
	}

	if !strings.Contains(stderr, "could not lock config file") {
		t.Fatalf("locked write stderr = %q", stderr)
	}

	if got := readFileString(t, filepath.Join(repo, ".git", cmdConfig)); got != before {
		t.Fatalf("locked write changed config:\n%s", got)
	}
}

func TestConfigFileDashUsesStdin(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	home := filepath.Join(base, "home")
	mkdirAll(t, home)

	stdout, stderr, err := runGogitEnvStdin(
		t,
		base,
		configEnv(home),
		"[user]\n\tname = Alice\n",
		cmdConfig,
		subGet,
		flagFile,
		"-",
		keyUserName,
	)
	if err != nil || stdout != "Alice\n" || stderr != "" {
		t.Fatalf("stdin read: stdout %q, stderr %q, err %v", stdout, stderr, err)
	}

	stdout, stderr, err = runGogitEnvStdin(
		t,
		base,
		configEnv(home),
		"",
		cmdConfig,
		subSet,
		flagFile,
		"-",
		keyUserName,
		"Alice",
	)

	var exitErr *exec.ExitError

	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 128 || stdout != "" ||
		stderr != "fatal: writing to stdin is not supported\n" {
		t.Fatalf("stdin write: stdout %q, stderr %q, err %v", stdout, stderr, err)
	}

	if _, err := os.Stat(filepath.Join(base, "-")); !os.IsNotExist(err) {
		t.Fatalf("stdin write created a literal '-' file: %v", err)
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
