package revfile_test

import (
	"errors"
	"strings"
	"testing"

	internalrevfile "github.com/go-git/cli/internal/plumbing/format/revfile"
)

func TestValidateHeader(t *testing.T) {
	t.Parallel()

	good := append([]byte{'R', 'I', 'D', 'X'},
		0x00, 0x00, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x01,
	)

	tests := []struct {
		name    string
		data    []byte
		wantErr error
	}{
		{name: "good", data: good, wantErr: nil},
		{name: "short", data: []byte{'R', 'I'}, wantErr: internalrevfile.ErrUnknownSignature},
		{
			name:    "bad magic",
			data:    append([]byte{'X', 'I', 'D', 'X'}, good[4:]...),
			wantErr: internalrevfile.ErrUnknownSignature,
		},
		{
			name:    "bad version",
			data:    append([]byte{'R', 'I', 'D', 'X', 0, 0, 0, 2}, good[8:]...),
			wantErr: internalrevfile.ErrUnsupportedVersion,
		},
		{
			name:    "bad hash",
			data:    []byte{'R', 'I', 'D', 'X', 0, 0, 0, 1, 0, 0, 0, 9},
			wantErr: internalrevfile.ErrUnsupportedHashFunc,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := internalrevfile.ValidateHeader(tc.data)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %v; want %v", err, tc.wantErr)
			}
		})
	}
}

func TestValidateRowPositionsRejectsOutOfRange(t *testing.T) {
	t.Parallel()

	ch := make(chan uint32, 4)

	ch <- 0

	ch <- 1

	ch <- 99 // out of range for objCount=3

	ch <- 2

	close(ch)

	err := internalrevfile.ValidateRowPositions(ch, 3)
	if !errors.Is(err, internalrevfile.ErrInvalidRowPosition) {
		t.Fatalf("got %v; want ErrInvalidRowPosition", err)
	}
}

func TestValidateRowPositionsCleanRun(t *testing.T) {
	t.Parallel()

	ch := make(chan uint32, 3)

	ch <- 0

	ch <- 2

	ch <- 1

	close(ch)

	if err := internalrevfile.ValidateRowPositions(ch, 3); err != nil {
		t.Fatalf("got %v; want nil", err)
	}
}

func TestFsckMessage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		err     error
		wantSub string
	}{
		{name: "unknown signature", err: internalrevfile.ErrUnknownSignature, wantSub: "unknown signature"},
		{name: "unsupported version", err: internalrevfile.ErrUnsupportedVersion, wantSub: "unsupported version"},
		{name: "unsupported hash", err: internalrevfile.ErrUnsupportedHashFunc, wantSub: "unsupported hash id"},
		{name: "row position", err: internalrevfile.ErrInvalidRowPosition, wantSub: "invalid rev-index position"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := internalrevfile.FsckMessage("pack-abc.rev", tc.err)
			if !strings.Contains(got, tc.wantSub) {
				t.Fatalf("FsckMessage(%v) = %q; want substring %q", tc.err, got, tc.wantSub)
			}

			if !strings.Contains(got, "pack-abc.rev") {
				t.Fatalf("FsckMessage(%v) = %q; want basename embedded", tc.err, got)
			}
		})
	}
}
