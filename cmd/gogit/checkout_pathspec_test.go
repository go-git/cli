package main

import (
	"path/filepath"
	"testing"
)

const (
	testRepoRoot = "/repo"
	testFile0    = "file0"
	testDirFile1 = "dir1/file1"
	testSubDir1  = "/repo/dir1"
)

func TestResolvePathspec(t *testing.T) {
	t.Parallel()

	root := testRepoRoot

	tests := []struct {
		name    string
		cwd     string
		spec    string
		want    string
		wantErr bool
	}{
		{name: "file at root from root", cwd: testRepoRoot, spec: testFile0, want: testFile0},
		{name: "subdir file from root", cwd: testRepoRoot, spec: testDirFile1, want: testDirFile1},
		{name: "dotdot back to root from subdir", cwd: testSubDir1, spec: "../file0", want: testFile0},
		{name: "sibling via dotdot", cwd: testSubDir1, spec: "../dir2/file2", want: "dir2/file2"},
		{name: "complex relative", cwd: testSubDir1, spec: "../dir1/../dir1/file1", want: testDirFile1},
		{name: "directory pathspec", cwd: testRepoRoot, spec: "dir1", want: "dir1"},
		{name: "parent escape from root", cwd: testRepoRoot, spec: "../Makefile", wantErr: true},
		{name: "parent file from subdir", cwd: testSubDir1, spec: "../file0", want: testFile0},
		{name: "escape from subdir", cwd: testSubDir1, spec: "../../file0", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolvePathspec(filepath.FromSlash(root), filepath.FromSlash(tc.cwd), tc.spec)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != filepath.FromSlash(tc.want) {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}
