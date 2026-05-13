package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseGitDate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		in         string
		wantSecs   int64
		wantOffset int
		wantErr    bool
	}{
		{name: "test_tick negative offset", in: "1112911993 -0700", wantSecs: 1112911993, wantOffset: -7 * 3600},
		{name: "positive offset", in: "1112911993 +0530", wantSecs: 1112911993, wantOffset: 5*3600 + 30*60},
		{name: "missing zone", in: "1112911993", wantErr: true},
		{name: "short zone panics in old impl", in: "1112911993 -7", wantErr: true},
		{name: "non-numeric zone", in: "1112911993 abcd", wantErr: true},
		{name: "non-numeric seconds", in: "abc -0700", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseGitDate(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseGitDate(%q) expected error, got time %v", tc.in, got)
				}

				return
			}

			if err != nil {
				t.Fatalf("parseGitDate(%q): %v", tc.in, err)
			}

			if got.Unix() != tc.wantSecs {
				t.Fatalf("seconds: got %d want %d", got.Unix(), tc.wantSecs)
			}

			_, offset := got.Zone()
			if offset != tc.wantOffset {
				t.Fatalf("offset: got %d want %d (got time %s)", offset, tc.wantOffset, got.Format(time.RFC3339))
			}
		})
	}
}

func TestCommitWithEnvIdentity(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()

	if _, _, err := runGogit(t, repo, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repo, "file0"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := runGogit(t, repo, "add", "file0"); err != nil {
		t.Fatalf("add: %v", err)
	}

	if _, stderr, err := runGogitEnv(t, repo, gitIdentityEnv(repo), "commit", "-m", "populate tree"); err != nil {
		t.Fatalf("commit failed: %v\nstderr: %s", err, stderr)
	}
}
