package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	homeIncludePath    = "home expansion"
	configSelectedFile = "selected"
	configExplicitFile = "explicit"
	flagSystem         = "--system"
	configTargetKey    = "x.a"
	configChangedValue = "changed"
)

func runIsolatedConfig(t *testing.T, dir, home string, env, args []string) (string, string, int) {
	t.Helper()

	cmd := exec.Command(gogitBin, args...)
	cmd.Dir = dir

	for _, item := range os.Environ() {
		if !strings.HasPrefix(item, "GIT_") {
			cmd.Env = append(cmd.Env, item)
		}
	}

	cmd.Env = append(cmd.Env, configEnv(home)...)
	cmd.Env = append(cmd.Env, env...)

	var out, stderr bytes.Buffer

	cmd.Stdout, cmd.Stderr = &out, &stderr
	err := cmd.Run()
	code := 0

	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatal(err)
		}

		code = exit.ExitCode()
	}

	return out.String(), stderr.String(), code
}

func TestConfigEnvironmentFileSelection(t *testing.T) {
	t.Parallel()

	for _, modern := range []bool{false, true} {
		for _, tc := range []configSelectionCase{
			{name: "environment", target: configSelectedFile},
			{name: "empty environment", envEmpty: true, wantCode: 4},
			{name: "file overrides", flags: []string{flagFile, configExplicitFile}, target: configExplicitFile},
			{
				name: "file overrides empty", envEmpty: true,
				flags: []string{flagFile, configExplicitFile}, target: configExplicitFile,
			},
			{name: "empty file overrides", flags: []string{"--file="}, wantCode: 4},
			{name: "local conflict", flags: []string{flagLocal}, wantCode: 129},
			{name: "global conflict", flags: []string{flagGlobal}, wantCode: 129},
			{name: "system conflict", flags: []string{flagSystem}, wantCode: 129},
			{name: "empty environment conflict", envEmpty: true, flags: []string{flagLocal}, wantCode: 129},
		} {
			name := "legacy/" + tc.name
			if modern {
				name = "modern/" + tc.name
			}

			t.Run(name, func(t *testing.T) {
				t.Parallel()
				checkConfigSelection(t, modern, tc)
			})
		}
	}
}

type configSelectionCase struct {
	name     string
	envEmpty bool
	flags    []string
	wantCode int
	target   string
}

func checkConfigSelection(t *testing.T, modern bool, tc configSelectionCase) {
	t.Helper()
	repo, home := newConfigRepo(t, "[x]\na = repository\n")
	writeConfig(t, filepath.Join(repo, configSelectedFile), "[x]\na = selected\n")
	writeConfig(t, filepath.Join(repo, configExplicitFile), "[x]\na = explicit\n")

	before := map[string]string{}
	for _, path := range []string{".git/config", configSelectedFile, configExplicitFile} {
		before[path] = readFileString(t, filepath.Join(repo, path))
	}

	value := configSelectedFile
	if tc.envEmpty {
		value = ""
	}

	env := []string{"GIT_CONFIG=" + value}

	read, write := []string{cmdConfig}, []string{cmdConfig}
	if modern {
		read = append(read, subGet)
		write = append(write, subSet)
	}

	read = append(read, tc.flags...)

	write = append(write, tc.flags...)
	if !modern {
		read = append(read, flagGet)
	}

	read = append(read, configTargetKey)
	write = append(write, configTargetKey, configChangedValue)

	wantRead := tc.wantCode
	if wantRead == 4 {
		wantRead = 1
	}

	out, stderr, code := runIsolatedConfig(t, repo, home, env, read)
	if code != wantRead || (wantRead == 0 && out != tc.target+"\n") {
		t.Fatalf("read = %q, %d, %s", out, code, stderr)
	}

	out, stderr, code = runIsolatedConfig(t, repo, home, env, write)
	if code != tc.wantCode || out != "" {
		t.Fatalf("write = %q, %d, %s", out, code, stderr)
	}

	for path, original := range before {
		got := readFileString(t, filepath.Join(repo, path))
		if path == tc.target {
			if !strings.Contains(got, configChangedValue) {
				t.Errorf("selected file not changed: %q", got)
			}
		} else if got != original {
			t.Errorf("unselected %s changed: %q", path, got)
		}

		if _, err := os.Stat(filepath.Join(repo, path) + ".lock"); !os.IsNotExist(err) {
			t.Errorf("lock remains: %v", err)
		}
	}
}

func TestConfigPhysicalIncludePaths(t *testing.T) {
	t.Parallel()

	for _, kind := range []string{"absolute", "relative", homeIncludePath, "nested file"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			repo, home := newConfigRepo(t, "")

			base := filepath.Join(repo, ".git")
			if kind == homeIncludePath {
				base = home
			}

			mkdirAll(t, filepath.Join(base, "real", "sub"))

			if err := os.Symlink("real/sub", filepath.Join(base, "jump")); err != nil {
				t.Skip(err)
			}

			writeConfig(t, filepath.Join(base, "cfg"), "[x]\na = wrong\n")
			writeConfig(t, filepath.Join(base, "real", "cfg"), "[x]\na = intended\n")

			path := "jump/../cfg"
			if kind == "absolute" {
				path = base + "/" + path
			}

			if kind == homeIncludePath {
				path = "~/" + path
			}

			if kind == "nested file" {
				writeConfig(t, filepath.Join(base, "real", "included"), "[include]\npath = cfg\n")

				path = "jump/../included"
			}

			writeConfig(t, filepath.Join(repo, ".git", "config"), "[include]\npath = "+path+"\n")

			out, stderr, code := runConfig(t, repo, home, cmdConfig, subGet, configTargetKey)
			if code != 0 || out != "intended\n" {
				t.Fatalf("include = %q, %d, %s", out, code, stderr)
			}
		})
	}
}

func TestConfigPathPreservesSuffix(t *testing.T) {
	t.Parallel()
	repo, home := newConfigRepo(t, "[x]\na = ~/link/../file/\n")

	out, stderr, code := runConfig(t, repo, home, cmdConfig, subGet, flagPath, configTargetKey)
	if code != 0 || out != home+"/link/../file/\n" {
		t.Fatalf("path = %q, %d, %s", out, code, stderr)
	}
}

func TestConfigOnBranchRequiresBranch(t *testing.T) {
	t.Parallel()

	for _, state := range []string{"outside", "detached", "unborn"} {
		t.Run(state, func(t *testing.T) {
			t.Parallel()

			repo, home := newConfigRepo(t, "")
			if state == "outside" {
				repo = home
			}

			if state == "detached" {
				writeConfig(t, filepath.Join(repo, ".git", "HEAD"), strings.Repeat("a", 40)+"\n")
			}

			include := filepath.Join(home, "include")
			writeConfig(t, include, "[x]\na = included\n")
			writeConfig(t, filepath.Join(home, ".gitconfig"), "[includeIf \"onbranch:*\"]\npath = "+include+"\n")

			out, stderr, code := runConfig(t, repo, home, cmdConfig, subGet, configTargetKey)
			if state == "unborn" {
				if code != 0 || out != "included\n" {
					t.Fatalf("named branch = %q, %d, %s", out, code, stderr)
				}
			} else if code != 1 || out != "" {
				t.Fatalf("absent branch = %q, %d, %s", out, code, stderr)
			}
		})
	}
}

func TestConfigExplicitSystemWithNoSystem(t *testing.T) {
	t.Parallel()
	repo, home := newConfigRepo(t, "[x]\na = repository\n")
	system := filepath.Join(home, "system")
	writeConfig(t, system, "[x]\na = system\n")

	env := []string{"GIT_CONFIG_SYSTEM=" + system}
	for _, args := range [][]string{
		{cmdConfig, flagSystem, flagGet, configTargetKey},
		{cmdConfig, subGet, flagSystem, configTargetKey},
	} {
		out, stderr, code := runIsolatedConfig(t, repo, home, env, args)
		if code != 0 || out != "system\n" {
			t.Fatalf("system = %q, %d, %s", out, code, stderr)
		}
	}

	for _, args := range [][]string{
		{cmdConfig, flagSystem, configTargetKey, configChangedValue},
		{cmdConfig, subSet, flagSystem, configTargetKey, configChangedValue},
	} {
		_, stderr, code := runIsolatedConfig(t, repo, home, env, args)
		if code != 0 {
			t.Fatalf("system write = %d, %s", code, stderr)
		}
	}

	if got := readFileString(t, system); !strings.Contains(got, configChangedValue) {
		t.Fatal(got)
	}

	out, stderr, code := runIsolatedConfig(t, repo, home, env, []string{cmdConfig, subGet, configTargetKey})
	if code != 0 || out != "repository\n" {
		t.Fatalf("implicit read = %q, %d, %s", out, code, stderr)
	}

	if got := readFileString(t, filepath.Join(repo, ".git", "config")); got != "[x]\na = repository\n" {
		t.Fatal(got)
	}
}
