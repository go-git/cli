package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/cli/internal/plumbing/format/config"
)

const symlinkParentConfig = "jump/../cfg"

func TestUpdateFilePreservesPhysicalPath(t *testing.T) {
	t.Parallel()

	for _, tc := range []physicalWriteCase{
		{name: "parent traversal", path: symlinkParentConfig},
		{name: "link payload", path: "link", link: symlinkParentConfig},
		{name: "dangling payload", path: "link", link: symlinkParentConfig, missing: true},
		{name: "missing final file", path: symlinkParentConfig, missing: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			checkPhysicalWrite(t, tc)
		})
	}
}

type physicalWriteCase struct {
	name, path, link string
	missing          bool
}

func checkPhysicalWrite(t *testing.T, tc physicalWriteCase) {
	t.Helper()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "real", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink("real/sub", filepath.Join(dir, "jump")); err != nil {
		t.Skip(err)
	}

	if tc.link != "" {
		if err := os.Symlink(tc.link, filepath.Join(dir, "link")); err != nil {
			t.Fatal(err)
		}
	}

	other := filepath.Join(dir, "cfg")
	intended := filepath.Join(dir, "real", "cfg")

	original := []byte("[a]\nb = original\n")
	if err := os.WriteFile(other, original, 0o644); err != nil {
		t.Fatal(err)
	}

	if !tc.missing {
		if err := os.WriteFile(intended, original, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	path := dir + "/" + tc.path
	if err := config.UpdateFile(path, func(f *config.File) error {
		return f.Set(mustKey(t, "a.b"), "updated")
	}); err != nil {
		t.Fatal(err)
	}

	f, err := config.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if got, ok := f.Get(mustKey(t, "a.b")); !ok || got != "updated" {
		t.Fatalf("read selected path = %q, %v", got, ok)
	}

	data, err := os.ReadFile(other)
	if err != nil || string(data) != string(original) {
		t.Fatalf("unintended file changed: %q, %v", data, err)
	}

	f, err = config.ReadFile(intended)
	if err != nil {
		t.Fatal(err)
	}

	if got, ok := f.Get(mustKey(t, "a.b")); !ok || got != "updated" {
		t.Fatalf("physical target = %q, %v", got, ok)
	}

	if _, err := os.Stat(intended + ".lock"); !os.IsNotExist(err) {
		t.Fatalf("lock remains: %v", err)
	}
}

func TestUpdateFileRejectsInvalidTraversal(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"missing/../cfg", "plain/../cfg", "loop/cfg", "loop", "missing/cfg"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "plain"), nil, 0o644); err != nil {
				t.Fatal(err)
			}

			if err := os.Symlink("loop", filepath.Join(dir, "loop")); err != nil {
				t.Skip(err)
			}

			called := false

			err := config.UpdateFile(dir+"/"+path, func(*config.File) error {
				called = true

				return nil
			})
			if err == nil || called {
				t.Fatalf("invalid traversal accepted: error %v, mutation %v", err, called)
			}

			if _, err := os.Stat(filepath.Join(dir, "cfg")); !os.IsNotExist(err) {
				t.Fatalf("created unintended config: %v", err)
			}
		})
	}
}

func TestUpdateFilePhysicalTargetLockAndRecovery(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "real", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink("real/sub", filepath.Join(dir, "jump")); err != nil {
		t.Skip(err)
	}

	target := filepath.Join(dir, "real", "cfg")

	original := []byte("[a]\nb = old\n")
	if err := os.WriteFile(target, original, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(target+".lock", nil, 0o644); err != nil {
		t.Fatal(err)
	}

	path := dir + "/jump/../cfg"

	mutate := func(f *config.File) error {
		return f.Set(mustKey(t, "a.b"), "updated")
	}
	if err := config.UpdateFile(path, mutate); err == nil {
		t.Fatal("write bypassed the physical target lock")
	}

	if err := os.Remove(target + ".lock"); err != nil {
		t.Fatal(err)
	}

	rejected := os.ErrPermission
	if err := config.UpdateFile(path, func(f *config.File) error {
		if err := mutate(f); err != nil {
			t.Fatal(err)
		}

		return rejected
	}); !errors.Is(err, rejected) {
		t.Fatalf("mutation error = %v", err)
	}

	if data, err := os.ReadFile(target); err != nil || string(data) != string(original) {
		t.Fatalf("failed write changed target: %q, %v", data, err)
	}

	if _, err := os.Stat(target + ".lock"); !os.IsNotExist(err) {
		t.Fatalf("lock remains: %v", err)
	}

	if err := config.UpdateFile(path, mutate); err != nil {
		t.Fatalf("next write: %v", err)
	}
}

func TestUpdateFileParentPermissionFailure(t *testing.T) {
	t.Parallel()

	parent := filepath.Join(t.TempDir(), "protected")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(parent, "config")

	original := []byte("[a]\nb = original\n")
	if err := os.WriteFile(target, original, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(parent, 0); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := os.Chmod(parent, 0o700); err != nil {
			t.Error(err)
		}
	})

	if _, err := os.ReadFile(target); !os.IsPermission(err) {
		t.Skip("directory permissions are not enforced for this user")
	}

	called := false

	err := config.UpdateFile(target, func(*config.File) error {
		called = true

		return nil
	})
	if !errors.Is(err, os.ErrPermission) || called {
		t.Fatalf("permission failure = %v, mutation %v", err, called)
	}

	if err := os.Chmod(parent, 0o700); err != nil {
		t.Fatal(err)
	}

	if got, err := os.ReadFile(target); err != nil || string(got) != string(original) {
		t.Fatalf("target changed: %q, %v", got, err)
	}

	if _, err := os.Stat(target + ".lock"); !os.IsNotExist(err) {
		t.Fatalf("lock remains: %v", err)
	}

	if err := config.UpdateFile(target, func(f *config.File) error {
		return f.Set(mustKey(t, "a.b"), "updated")
	}); err != nil {
		t.Fatalf("next write: %v", err)
	}
}
