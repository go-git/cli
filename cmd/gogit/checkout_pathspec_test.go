package main

import (
	"path/filepath"
	"testing"
)

func TestResolvePathspec(t *testing.T) {
	t.Parallel()

	root := "/repo"

	tests := []struct {
		name    string
		cwd     string
		spec    string
		want    string
		wantErr bool
	}{
		{name: "file at root from root", cwd: "/repo", spec: "file0", want: "file0"},
		{name: "subdir file from root", cwd: "/repo", spec: "dir1/file1", want: "dir1/file1"},
		{name: "dotdot back to root from subdir", cwd: "/repo/dir1", spec: "../file0", want: "file0"},
		{name: "sibling via dotdot", cwd: "/repo/dir1", spec: "../dir2/file2", want: "dir2/file2"},
		{name: "complex relative", cwd: "/repo/dir1", spec: "../dir1/../dir1/file1", want: "dir1/file1"},
		{name: "directory pathspec", cwd: "/repo", spec: "dir1", want: "dir1"},
		{name: "parent escape from root", cwd: "/repo", spec: "../Makefile", wantErr: true},
		{name: "parent file from subdir", cwd: "/repo/dir1", spec: "../file0", want: "file0"},
		{name: "escape from subdir", cwd: "/repo/dir1", spec: "../../file0", wantErr: true},
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
