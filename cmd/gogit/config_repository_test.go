package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	gitfileSelection     = "gitfile"
	discoveredRepository = "discovered"
	missingCommonRefs    = "missing common refs"
)

func TestConfigValidatesRepositoryBeforeWriting(t *testing.T) {
	t.Parallel()

	for _, shape := range []string{
		"empty", "malformed HEAD", "missing worktree HEAD", "malformed worktree HEAD", missingCommonRefs,
	} {
		for _, selection := range []string{discoveredRepository, "GIT_DIR", gitfileSelection} {
			t.Run(shape+"/"+selection, func(t *testing.T) {
				t.Parallel()
				checkInvalidRepositoryWrite(t, shape, selection)
			})
		}
	}
}

func checkInvalidRepositoryWrite(t *testing.T, shape, selection string) {
	t.Helper()
	repo, home := newConfigRepo(t, "[x]\na = ancestor\n")
	child := filepath.Join(repo, "child")

	candidate := filepath.Join(child, ".git")
	if selection == gitfileSelection {
		candidate = filepath.Join(repo, "candidate")
	}

	mkdirAll(t, candidate)
	mkdirAll(t, child)
	populateInvalidRepository(t, repo, candidate, shape)

	env := []string{}
	if selection == "GIT_DIR" {
		env = append(env, "GIT_DIR="+candidate)
	}

	if selection == gitfileSelection {
		writeConfig(t, filepath.Join(child, ".git"), "gitdir: "+candidate+"\n")
	}

	for _, args := range [][]string{
		{cmdConfig, configTargetKey, configChangedValue},
		{cmdConfig, subSet, configTargetKey, configChangedValue},
	} {
		_, stderr, code := runIsolatedConfig(t, child, home, env, args)

		want := 128
		if selection == discoveredRepository {
			want = 0
		}

		if code != want {
			t.Fatalf("write = %d, want %d, %s", code, want, stderr)
		}

		if _, err := os.Stat(filepath.Join(candidate, "config")); !os.IsNotExist(err) {
			t.Fatalf("invalid repository config created: %v", err)
		}

		if _, err := os.Stat(filepath.Join(candidate, "config.lock")); !os.IsNotExist(err) {
			t.Fatalf("invalid repository lock created: %v", err)
		}
	}

	got := readFileString(t, filepath.Join(repo, ".git", "config"))

	want := "[x]\na = ancestor\n"
	if selection == discoveredRepository {
		want = "[x]\na = changed\n"
	}

	if got != want {
		t.Fatalf("ancestor = %q, want %q", got, want)
	}
}

func TestConfigDiscoveredEmptyRepositoryWithoutAncestor(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	mkdirAll(t, filepath.Join(base, ".git"))

	_, stderr, code := runIsolatedConfig(t, base, base,
		[]string{"GIT_CEILING_DIRECTORIES=" + filepath.Dir(base)},
		[]string{cmdConfig, subSet, configTargetKey, configChangedValue})
	if code != 128 {
		t.Fatalf("empty repository write = %d, %s", code, stderr)
	}

	if _, err := os.Stat(filepath.Join(base, ".git", "config")); !os.IsNotExist(err) {
		t.Fatalf("config created: %v", err)
	}
}

func populateInvalidRepository(t *testing.T, repo, candidate, shape string) {
	t.Helper()

	if shape != "empty" {
		mkdirAll(t, filepath.Join(candidate, "objects"))
		mkdirAll(t, filepath.Join(candidate, "refs"))
		writeConfig(t, filepath.Join(candidate, "HEAD"), "invalid\n")
	}

	if strings.Contains(shape, "worktree") || shape == missingCommonRefs {
		writeConfig(t, filepath.Join(candidate, "commondir"), filepath.Join(repo, ".git")+"\n")

		if shape == "missing worktree HEAD" {
			if err := os.Remove(filepath.Join(candidate, "HEAD")); err != nil {
				t.Fatal(err)
			}
		}

		if shape == missingCommonRefs {
			common := filepath.Join(repo, "incomplete-common")
			mkdirAll(t, filepath.Join(common, "objects"))
			writeConfig(t, filepath.Join(candidate, "commondir"), common+"\n")
			writeConfig(t, filepath.Join(candidate, "HEAD"), "ref: refs/heads/main\n")
		}
	}
}

func TestConfigGitDirPreservesPhysicalPath(t *testing.T) {
	t.Parallel()

	repo, home := newConfigRepo(t, "[x]\na = repository\n")
	if err := os.Symlink(".git/objects", filepath.Join(repo, "jump")); err != nil {
		t.Skip(err)
	}

	included := filepath.Join(home, "included")
	writeConfig(t, included, "[x]\nb = included\n")
	writeConfig(t, filepath.Join(home, ".gitconfig"), "[includeIf \"onbranch:main\"]\npath = "+included+"\n")

	env := []string{"GIT_DIR=" + repo + "/jump/.."}

	out, stderr, code := runIsolatedConfig(t, repo, home, env, []string{cmdConfig, subGet, "x.b"})
	if code != 0 || out != "included\n" {
		t.Fatalf("physical branch context = %q, %d, %s", out, code, stderr)
	}

	_, stderr, code = runIsolatedConfig(t, repo, home, env,
		[]string{cmdConfig, subSet, configTargetKey, configChangedValue})
	if code != 0 {
		t.Fatalf("physical repository write = %d, %s", code, stderr)
	}

	if got := readFileString(t, filepath.Join(repo, ".git", "config")); !strings.Contains(got, configChangedValue) {
		t.Fatal(got)
	}

	if _, err := os.Stat(filepath.Join(repo, "config")); !os.IsNotExist(err) {
		t.Fatalf("wrong repository config created: %v", err)
	}
}
