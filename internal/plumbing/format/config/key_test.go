package config_test

import (
	"errors"
	"testing"

	config "github.com/go-git/cli/internal/plumbing/format/config"
)

// Literals shared by the tests in this package.
const (
	secUser   = "user"
	secRemote = "remote"
	varName   = "name"
	varURL    = "url"

	keyUserName  = "user.name"
	keyEmptySub  = "user..name"
	keyOriginURL = "remote.origin.url"
	keyDottedSub = "remote.team.one.url"

	keyUpperName = "USER.NAME"
	keyMixedSub  = "remote.Origin.url"

	valPlain = "plain"
)

func TestParseKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want config.Key
		// wantErr matches the expected error type; nil means success.
		wantErr func(error) bool
	}{
		{
			name: "two components",
			in:   keyUserName,
			want: config.Key{Section: secUser, Name: varName},
		},
		{
			name: "subsection",
			in:   keyOriginURL,
			want: config.Key{Section: secRemote, Subsection: "origin", HasSubsection: true, Name: varURL},
		},
		{
			name: "subsection containing dots splits at first and last dot",
			in:   keyDottedSub,
			want: config.Key{Section: secRemote, Subsection: "team.one", HasSubsection: true, Name: varURL},
		},
		{
			name: "empty subsection is distinct from no subsection",
			in:   keyEmptySub,
			want: config.Key{Section: secUser, Subsection: "", HasSubsection: true, Name: varName},
		},
		{
			name: "spelling is preserved, not folded",
			in:   keyUpperName,
			want: config.Key{Section: "USER", Name: "NAME"},
		},
		{
			name: "subsection keeps its case",
			in:   keyMixedSub,
			want: config.Key{Section: secRemote, Subsection: "Origin", HasSubsection: true, Name: varURL},
		},
		{
			name: "section may start with a digit",
			in:   "0section.name",
			want: config.Key{Section: "0section", Name: varName},
		},
		{
			name: "hyphens are allowed",
			in:   "a-b.c-d",
			want: config.Key{Section: "a-b", Name: "c-d"},
		},
		{name: "no separator", in: secUser, wantErr: isNoSection},
		{name: "empty", in: "", wantErr: isNoSection},
		{name: "trailing separator", in: "user.", wantErr: isNoVariable},
		{name: "variable starting with a digit", in: "a.1b", wantErr: isInvalidKey},
		{name: "variable starting with a hyphen", in: "a.-b", wantErr: isInvalidKey},
		{name: "underscore in section", in: "a_x.b", wantErr: isInvalidKey},
		{name: "underscore in variable", in: "a.b_y", wantErr: isInvalidKey},
		{name: "space in variable", in: "a.b c", wantErr: isInvalidKey},
		{name: "space in section", in: "a b.c", wantErr: isInvalidKey},
		{name: "leading separator", in: ".b", wantErr: isInvalidKey},
		{name: "newline in subsection", in: "a.foo\nbar.b", wantErr: isInvalidKey},
		{name: "NUL in subsection", in: "a.foo\x00bar.b", wantErr: isInvalidKey},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := config.ParseKey(tc.in)

			if tc.wantErr != nil {
				if err == nil {
					t.Fatalf("ParseKey(%q) = %+v, want an error", tc.in, got)
				}

				if !tc.wantErr(err) {
					t.Fatalf("ParseKey(%q) returned the wrong error type: %v (%T)", tc.in, err, err)
				}

				return
			}

			if err != nil {
				t.Fatalf("ParseKey(%q) failed: %v", tc.in, err)
			}

			if got != tc.want {
				t.Fatalf("ParseKey(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}

func isNoSection(err error) bool {
	var target *config.KeyNoSectionError

	return errors.As(err, &target)
}

func isNoVariable(err error) bool {
	var target *config.KeyNoVariableError

	return errors.As(err, &target)
}

func isInvalidKey(err error) bool {
	var target *config.KeyInvalidError

	return errors.As(err, &target)
}

func TestKeyString(t *testing.T) {
	t.Parallel()

	for _, in := range []string{keyUserName, keyOriginURL, keyDottedSub, keyEmptySub} {
		t.Run(in, func(t *testing.T) {
			t.Parallel()

			k, err := config.ParseKey(in)
			if err != nil {
				t.Fatalf("ParseKey(%q): %v", in, err)
			}

			if got := k.String(); got != in {
				t.Fatalf("Key.String() = %q, want %q", got, in)
			}
		})
	}
}

// TestKeyMatches pins the comparison rules: section and variable names fold,
// subsection names do not, and an empty subsection is its own thing. Keys must
// never be compared with ==, which would make these all unequal.
func TestKeyMatches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{name: "identical", a: keyUserName, b: keyUserName, want: true},
		{name: "section folds", a: "USER.name", b: keyUserName, want: true},
		{name: "variable folds", a: "user.NAME", b: keyUserName, want: true},
		{name: "both fold", a: keyUpperName, b: keyUserName, want: true},
		{name: "subsection does not fold", a: keyMixedSub, b: keyOriginURL, want: false},
		{name: "subsection section folds", a: "REMOTE.origin.url", b: keyOriginURL, want: true},
		{name: "empty subsection is not no subsection", a: keyEmptySub, b: keyUserName, want: false},
		{name: "different variable", a: "user.email", b: keyUserName, want: false},
		{name: "different section", a: "core.name", b: keyUserName, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			a, err := config.ParseKey(tc.a)
			if err != nil {
				t.Fatalf("ParseKey(%q): %v", tc.a, err)
			}

			b, err := config.ParseKey(tc.b)
			if err != nil {
				t.Fatalf("ParseKey(%q): %v", tc.b, err)
			}

			if got := a.Matches(b); got != tc.want {
				t.Fatalf("%q.Matches(%q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}

			if got := b.Matches(a); got != tc.want {
				t.Fatalf("Matches is not symmetric for %q and %q", tc.a, tc.b)
			}
		})
	}
}
