package object_test

import (
	"testing"
	"time"

	internalobject "github.com/go-git/cli/internal/plumbing/object"
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
		{name: "short zone rejected", in: "1112911993 -7", wantErr: true},
		{name: "non-numeric zone", in: "1112911993 abcd", wantErr: true},
		{name: "non-numeric seconds", in: "abc -0700", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := internalobject.ParseGitDate(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseGitDate(%q) expected error, got time %v", tc.in, got)
				}

				return
			}

			if err != nil {
				t.Fatalf("ParseGitDate(%q): %v", tc.in, err)
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
