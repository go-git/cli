package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	config "github.com/go-git/cli/internal/plumbing/format/config"
)

func mustKey(t *testing.T, s string) config.Key {
	t.Helper()

	k, err := config.ParseKey(s)
	if err != nil {
		t.Fatalf("ParseKey(%q): %v", s, err)
	}

	return k
}

func mustParse(t *testing.T, src string) *config.File {
	t.Helper()

	f, err := config.Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}

	return f
}

func TestValues(t *testing.T) {
	t.Parallel()

	const src = `# leading comment
[user]
	; why
	name = A U Thor
	email = a@b.c
[remote "origin"]
	url = https://example.com/x.git
[remote "team.one"]
	url = https://t.example/o.git
[user ""]
	name = EMPTYSUB
[foo]
	bar = one
	bar = two
[e]
	empty =
[b]
	flag
[crlf]` + "\r\n\tk = v\r\n" + `[same] inline = yes
[a.SUB]
	dotted = legacy
[q]
	quoted = "x  y"
	esc = "tab\there"
	trail = value # comment
`

	tests := []struct {
		name string
		key  string
		want []string
	}{
		{name: "simple", key: keyUserName, want: []string{"A U Thor"}},
		{name: "case-insensitive key", key: "USER.NAME", want: []string{"A U Thor"}},
		{name: "subsection", key: keyOriginURL, want: []string{"https://example.com/x.git"}},
		{name: "subsection with dots", key: keyDottedSub, want: []string{"https://t.example/o.git"}},
		{name: "empty subsection", key: keyEmptySub, want: []string{"EMPTYSUB"}},
		{name: "multivalue in file order", key: "foo.bar", want: []string{"one", "two"}},
		{name: "explicitly empty value", key: "e.empty", want: []string{""}},
		{name: "valueless variable", key: "b.flag", want: []string{""}},
		{name: "crlf line endings", key: "crlf.k", want: []string{"v"}},
		{name: "option on the header line", key: "same.inline", want: []string{"yes"}},
		{name: "legacy dotted section", key: "a.sub.dotted", want: []string{"legacy"}},
		{name: "legacy dotted section is case-folded", key: "a.SUB.dotted", want: nil},
		{name: "quoted value keeps inner spaces", key: "q.quoted", want: []string{"x  y"}},
		{name: "escape sequences decoded", key: "q.esc", want: []string{"tab\there"}},
		{name: "trailing comment excluded", key: "q.trail", want: []string{"value"}},
		{name: "absent key", key: "no.such", want: nil},
		{name: "absent subsection", key: "remote.other.url", want: nil},
	}

	f := mustParse(t, src)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := f.Values(mustKey(t, tc.key))
			if len(got) != len(tc.want) {
				t.Fatalf("Values(%s) = %q, want %q", tc.key, got, tc.want)
			}

			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("Values(%s) = %q, want %q", tc.key, got, tc.want)
				}
			}
		})
	}
}

// TestGetDistinguishesMissingFromEmpty pins the difference the command layer
// turns into "exit 1 with no output" versus "exit 0 with one blank line".
func TestGetDistinguishesMissingFromEmpty(t *testing.T) {
	t.Parallel()

	f := mustParse(t, "[e]\n\tempty =\n")

	if v, ok := f.Get(mustKey(t, "e.empty")); !ok || v != "" {
		t.Fatalf("Get(e.empty) = (%q, %v), want (\"\", true)", v, ok)
	}

	if v, ok := f.Get(mustKey(t, "e.missing")); ok {
		t.Fatalf("Get(e.missing) = (%q, %v), want (\"\", false)", v, ok)
	}
}

func TestParseRejectsMalformed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		src      string
		wantLine int
	}{
		{name: "bare words", src: "[a]\nbogus line here\n", wantLine: 2},
		{name: "variable before any section", src: "b = c\n", wantLine: 1},
		{name: "unterminated section", src: "[a\n", wantLine: 1},
		{name: "unterminated subsection quote", src: "[a \"sub\n", wantLine: 1},
		{name: "unterminated value quote", src: "[a]\n\tb = \"oops\n", wantLine: 2},
		{name: "invalid section character", src: "[a_b]\n\tc = d\n", wantLine: 1},
		{name: "variable starting with a digit", src: "[a]\n\t1b = c\n", wantLine: 2},
		{name: "junk after a value", src: "[a]\n\tb = c\n\td e f\n", wantLine: 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := config.Parse([]byte(tc.src))
			if err == nil {
				t.Fatalf("Parse(%q) succeeded, want a parse error", tc.src)
			}

			var perr *config.ParseError
			if !errors.As(err, &perr) {
				t.Fatalf("Parse(%q) error = %v (%T), want *ParseError", tc.src, err, err)
			}

			if perr.Line != tc.wantLine {
				t.Fatalf("Parse(%q) reported line %d, want %d", tc.src, perr.Line, tc.wantLine)
			}
		})
	}
}

func TestMutationsPreserveFormatting(t *testing.T) {
	t.Parallel()

	const src = `# a comment worth keeping
[user]
	; and an inline note
	name = Old Name
	email = a@b.c

[remote "origin"]
	url = https://x/y.git
[foo]
	bar = one
	bar = two
`

	tests := []struct {
		name string
		do   func(*testing.T, *config.File)
		want string
	}{
		{
			name: "set rewrites only the value",
			do: func(t *testing.T, f *config.File) {
				t.Helper()

				if err := f.Set(mustKey(t, keyUserName), "New Name"); err != nil {
					t.Fatal(err)
				}
			},
			want: strings.Replace(src, "name = Old Name", "name = New Name", 1),
		},
		{
			name: "set on a subsection targets the subsection",
			do: func(t *testing.T, f *config.File) {
				t.Helper()

				if err := f.Set(mustKey(t, keyOriginURL), "https://new/z.git"); err != nil {
					t.Fatal(err)
				}
			},
			want: strings.Replace(src, "url = https://x/y.git", "url = https://new/z.git", 1),
		},
		{
			name: "add appends after the section's last variable",
			do: func(t *testing.T, f *config.File) {
				t.Helper()

				if err := f.Add(mustKey(t, keyUserName), "Second"); err != nil {
					t.Fatal(err)
				}
			},
			want: strings.Replace(src, "\temail = a@b.c\n", "\temail = a@b.c\n\tname = Second\n", 1),
		},
		{
			name: "a new section is appended at end of file",
			do: func(t *testing.T, f *config.File) {
				t.Helper()

				if err := f.Set(mustKey(t, "new.key"), "v"); err != nil {
					t.Fatal(err)
				}
			},
			want: src + "[new]\n\tkey = v\n",
		},
		{
			name: "unset removes the line and nothing else",
			do: func(t *testing.T, f *config.File) {
				t.Helper()

				if _, err := f.UnsetAll(mustKey(t, keyUserName)); err != nil {
					t.Fatal(err)
				}
			},
			want: strings.Replace(src, "\tname = Old Name\n", "", 1),
		},
		{
			name: "unset --all removes every occurrence and the empty section",
			do: func(t *testing.T, f *config.File) {
				t.Helper()

				if _, err := f.UnsetAll(mustKey(t, "foo.bar")); err != nil {
					t.Fatal(err)
				}
			},
			want: strings.Replace(src, "[foo]\n\tbar = one\n\tbar = two\n", "", 1),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := mustParse(t, src)
			tc.do(t, f)

			if got := string(f.Bytes()); got != tc.want {
				t.Fatalf("result mismatch\n--- got ---\n%s\n--- want ---\n%s", got, tc.want)
			}
		})
	}
}

func TestSetRefusesMultipleValues(t *testing.T) {
	t.Parallel()

	f := mustParse(t, "[foo]\n\tbar = one\n\tbar = two\n")

	err := f.Set(mustKey(t, "foo.bar"), "three")
	if !errors.Is(err, config.ErrMultipleValues) {
		t.Fatalf("Set on a multivalued key = %v, want ErrMultipleValues", err)
	}

	if got := string(f.Bytes()); got != "[foo]\n\tbar = one\n\tbar = two\n" {
		t.Fatalf("refused Set modified the file:\n%s", got)
	}

	if err := f.ReplaceAll(mustKey(t, "foo.bar"), "three"); err != nil {
		t.Fatalf("ReplaceAll: %v", err)
	}

	if got, want := string(f.Bytes()), "[foo]\n\tbar = three\n"; got != want {
		t.Fatalf("ReplaceAll = %q, want %q", got, want)
	}
}

func TestUnsetAllReportsMissingKey(t *testing.T) {
	t.Parallel()

	f := mustParse(t, "[a]\n\tb = c\n")

	n, err := f.UnsetAll(mustKey(t, "a.missing"))
	if err != nil || n != 0 {
		t.Fatalf("UnsetAll(a.missing) = (%d, %v), want (0, nil)", n, err)
	}

	if got := string(f.Bytes()); got != "[a]\n\tb = c\n" {
		t.Fatalf("no-op UnsetAll modified the file: %q", got)
	}
}

// TestSetQuotesValuesLikeGit exercises value encoding through the public API:
// escapes are always applied, and quotes are added only when leading or
// trailing spaces or a comment character would otherwise change the meaning.
func TestSetQuotesValuesLikeGit(t *testing.T) {
	t.Parallel()

	tests := []struct{ name, in, want string }{
		{name: valPlain, in: valPlain, want: valPlain},
		{name: "empty", in: "", want: ""},
		{name: "inner space", in: "a b", want: "a b"},
		{name: "leading space", in: " lead", want: `" lead"`},
		{name: "trailing space", in: "trail ", want: `"trail "`},
		{name: "hash", in: "has # hash", want: `"has # hash"`},
		{name: "semicolon", in: "has ; semi", want: `"has ; semi"`},
		{name: "quote", in: `has "quote"`, want: `has \"quote\"`},
		{name: "backslash", in: `has \back`, want: `has \\back`},
		{name: "tab", in: "has\ttab", want: `has\ttab`},
		{name: "newline", in: "two\nlines", want: `two\nlines`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := mustParse(t, "[a]\n\tb = old\n")
			if err := f.Set(mustKey(t, "a.b"), tc.in); err != nil {
				t.Fatal(err)
			}

			want := "[a]\n\tb = " + tc.want + "\n"
			if got := string(f.Bytes()); got != want {
				t.Fatalf("Set(%q) wrote %q, want %q", tc.in, got, want)
			}

			// The encoding must survive a round trip unchanged.
			again := mustParse(t, string(f.Bytes()))
			if got, _ := again.Get(mustKey(t, "a.b")); got != tc.in {
				t.Fatalf("round trip of %q produced %q", tc.in, got)
			}
		})
	}
}

// TestReadFileErrorsAreNotEmptyConfigs guards against a read failure being
// mistaken for an absent file, which would let a later write truncate a
// config that is merely unreadable right now.
func TestReadFileErrorsAreNotEmptyConfigs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	if f, err := config.ReadFile(filepath.Join(dir, "does-not-exist")); err != nil {
		t.Fatalf("missing file should parse as empty, got %v", err)
	} else if len(f.Bytes()) != 0 {
		t.Fatalf("missing file parsed to %q, want empty", f.Bytes())
	}

	// A directory stands in for any read error that is not "not exist".
	if _, err := config.ReadFile(dir); err == nil {
		t.Fatal("reading a directory should fail, not yield an empty config")
	}
}

func TestWriteFileIsAtomicAndKeepsMode(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config")

	if err := os.WriteFile(path, []byte("[a]\n\tb = c\n"), 0o640); err != nil {
		t.Fatal(err)
	}

	f := mustParse(t, "[a]\n\tb = d\n")

	if err := config.WriteFile(path, f, config.FileMode(path)); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if st.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %v, want 0640", st.Mode().Perm())
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != "[a]\n\tb = d\n" {
		t.Fatalf("contents = %q", got)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 1 {
		t.Fatalf("WriteFile left temp files behind: %v", entries)
	}
}
