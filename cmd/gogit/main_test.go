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

var gogitBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "gogit-test-")
	if err != nil {
		panic(err)
	}

	gogitBin = filepath.Join(dir, "gogit")
	build := exec.Command("go", "build", "-o", gogitBin, ".")

	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		_ = os.RemoveAll(dir)

		panic(err)
	}

	// os.Exit skips deferred functions, so run cleanup explicitly before exit.
	code := m.Run()
	_ = os.RemoveAll(dir)

	os.Exit(code)
}

func runGogit(t *testing.T, dir string, args ...string) (string, string, error) {
	t.Helper()

	cmd := exec.Command(gogitBin, args...)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	return stdout.String(), stderr.String(), err
}

func runGogitEnv(t *testing.T, dir string, env []string, args ...string) (string, string, error) { //nolint:unparam
	t.Helper()

	cmd := exec.Command(gogitBin, args...)
	cmd.Dir = dir

	cmd.Env = append(os.Environ(), env...)

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	return stdout.String(), stderr.String(), err
}

func runGogitStdin(t *testing.T, dir string, stdin string, args ...string) (string, string, error) {
	t.Helper()

	cmd := exec.Command(gogitBin, args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	return stdout.String(), stderr.String(), err
}

func gitIdentityEnv(repo string) []string {
	return []string{
		"GIT_AUTHOR_NAME=A U Thor",
		"GIT_AUTHOR_EMAIL=author@example.com",
		"GIT_AUTHOR_DATE=1112911993 -0700",
		"GIT_COMMITTER_NAME=C O Mitter",
		"GIT_COMMITTER_EMAIL=committer@example.com",
		"GIT_COMMITTER_DATE=1112911993 -0700",
		"HOME=" + repo,
	}
}

func TestExecPath(t *testing.T) {
	t.Parallel()

	stdout, _, err := runGogit(t, t.TempDir(), "--exec-path")
	if err != nil {
		t.Fatalf("--exec-path failed: %v", err)
	}

	if len(stdout) == 0 {
		t.Fatal("--exec-path produced no output")
	}
}

func TestVersion(t *testing.T) {
	t.Parallel()

	stdout, _, err := runGogit(t, t.TempDir(), "--version")
	if err != nil {
		t.Fatalf("--version failed: %v", err)
	}

	if len(stdout) == 0 {
		t.Fatal("--version produced no output")
	}
}

func TestRootCmdNoArgsExitsOne(t *testing.T) {
	t.Parallel()

	_, _, err := runGogit(t, t.TempDir())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected exec.ExitError, got %T", err)
	}

	if ee.ExitCode() != 1 {
		t.Errorf("expected exit code 1, got %d", ee.ExitCode())
	}
}
