package config

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ParseError reports a line Git would reject with
// "fatal: bad config line N in file <path>".
type ParseError struct{ Line int }

func (e *ParseError) Error() string {
	return fmt.Sprintf("bad config line %d", e.Line)
}

// ErrMultipleValues is returned by Set when the key already has more than one
// value, mirroring git's refusal to collapse them into one.
var ErrMultipleValues = errors.New("cannot overwrite multiple values with a single value")

// File is a parsed configuration file that retains its original bytes, so
// mutations can splice individual variables without disturbing comments,
// blank lines, indentation or the ordering of anything else.
type File struct {
	data     []byte
	sections []*sectionRec
	options  []*optionRec
}

// Entry is one configuration variable in file order.
type Entry struct {
	Key      Key
	Value    string
	Implicit bool
}

// sectionRec is one occurrence of a section header in the file. The same
// section may be opened more than once.
type sectionRec struct {
	key Key // only Section, Subsection and HasSubsection are meaningful

	// entryEnd is where a new variable belonging to this header occurrence
	// should be inserted: just past the header line initially, then just
	// past the last variable line parsed under it.
	entryEnd int

	lineStart int // start of the header's physical line
	headerEnd int // just past the header's closing ']'
	lineEnd   int // just past the header line's terminator

	// plain reports a header line carrying nothing but the header itself.
	// git keeps a header that has a trailing comment even once its last
	// variable is removed.
	plain bool

	// regionEnd is where the next header begins, or end of file.
	regionEnd int
}

// optionRec is one occurrence of a variable in the file, with the byte ranges
// needed to rewrite or delete it in place.
type optionRec struct {
	key   Key
	value string

	// valueless marks a bare "name" with no '=', which Git treats as a
	// boolean true but renders as an empty string without --type=bool.
	valueless bool

	secIdx    int // index into File.sections of the governing header
	lineStart int // start of the first physical line this variable occupies
	lineEnd   int // just past the terminator of its last physical line
	nameEnd   int // just past the variable name

	// valueStart..logicalEnd is what Set replaces. It runs to the end of the
	// last physical line, so a trailing comment is dropped on rewrite, as
	// git does.
	valueStart int
	logicalEnd int

	// alone reports that no section header shares the variable's first line,
	// so Unset can delete whole lines rather than a byte range.
	alone bool
}

// Parse reads a configuration file. It returns a *ParseError for any
// construct Git itself would reject, so a malformed file is never partially
// understood and then rewritten.
func Parse(data []byte) (*File, error) {
	p := &parser{data: data, line: 1, f: &File{data: data}}
	if len(data) >= 3 && data[0] == 0xef && data[1] == 0xbb && data[2] == 0xbf {
		p.pos = 3
	}

	if err := p.run(); err != nil {
		return nil, err
	}

	return p.f, nil
}

// Bytes returns the current file contents.
func (f *File) Bytes() []byte {
	return f.data
}

// Entries returns the variables in their original file order.
func (f *File) Entries() []Entry {
	entries := make([]Entry, 0, len(f.options))
	for _, o := range f.options {
		entries = append(entries, Entry{Key: o.key, Value: o.value, Implicit: o.valueless})
	}

	return entries
}

// Values returns every value recorded for key, in file order. A nil result
// means the key is absent, which callers must distinguish from a key whose
// single value is the empty string.
func (f *File) Values(key Key) []string {
	var out []string

	for _, o := range f.options {
		if o.key.Matches(key) {
			out = append(out, o.value)
		}
	}

	return out
}

// Get returns the last value for key, which is the one Git reports for a
// plain lookup, and whether the key is present at all.
func (f *File) Get(key Key) (string, bool) {
	vals := f.Values(key)
	if len(vals) == 0 {
		return "", false
	}

	return vals[len(vals)-1], true
}

// Set replaces the value of key. It refuses a key with several values, as git
// does, returning ErrMultipleValues; use Add or ReplaceAll instead.
func (f *File) Set(key Key, value string) error {
	var found []*optionRec

	for _, o := range f.options {
		if o.key.Matches(key) {
			found = append(found, o)
		}
	}

	if len(found) > 1 {
		return ErrMultipleValues
	}

	if len(found) == 0 {
		return f.insert(key, value)
	}

	return f.rewrite(found[0], key, value)
}

// ReplaceAll collapses every value of key into a single value.
func (f *File) ReplaceAll(key Key, value string) error {
	var found []*optionRec

	for _, o := range f.options {
		if o.key.Matches(key) {
			found = append(found, o)
		}
	}

	switch len(found) {
	case 0:
		return f.insert(key, value)
	case 1:
		return f.rewrite(found[0], key, value)
	}

	// Git keeps the last matching occurrence in place and drops the earlier
	// ones, preserving the section ordering around the surviving value.
	edits := []edit{f.writeEdit(found[len(found)-1], key, value)}
	for _, o := range found[:len(found)-1] {
		edits = append(edits, f.deleteEdit(o))
	}

	return f.apply(edits)
}

// Add appends a new value for key without touching existing ones.
func (f *File) Add(key Key, value string) error {
	return f.insert(key, value)
}

// UnsetAll removes every occurrence of key and reports how many were removed.
func (f *File) UnsetAll(key Key) (int, error) {
	var (
		edits []edit
		n     int
	)

	for _, o := range f.options {
		if o.key.Matches(key) {
			edits = append(edits, f.deleteEdit(o))
			n++
		}
	}

	if n == 0 {
		return 0, nil
	}

	sectionEdits := f.emptySectionEdits(edits)
	for _, sectionEdit := range sectionEdits {
		edits = slices.DeleteFunc(edits, func(e edit) bool {
			return e.start >= sectionEdit.start && e.end <= sectionEdit.end
		})
	}

	return n, f.apply(append(edits, sectionEdits...))
}

// emptySectionEdits returns the header lines that become empty once edits are
// applied. git drops a section header whose last variable is removed, but only
// when nothing else — not even a comment — remains under it, and only when the
// header line itself carries nothing but the header.
func (f *File) emptySectionEdits(deletions []edit) []edit {
	touched := map[int]bool{}

	for _, o := range f.options {
		if isDeletedOption(o, deletions) {
			touched[o.secIdx] = true
		}
	}

	var out []edit

	processed := map[int]bool{}

	for idx := range touched {
		if processed[idx] {
			continue
		}

		var group []int

		for candidate, s := range f.sections {
			if s.key.sameSection(f.sections[idx].key) {
				group = append(group, candidate)
				processed[candidate] = true
			}
		}

		if groupHasRemainingOption(f.options, group, deletions) ||
			groupHasContent(f.data, f.sections, group, deletions) {
			continue
		}

		allPlain := true

		for _, candidate := range group {
			if !f.sectionPlainAfterDeletions(candidate, deletions) {
				allPlain = false

				break
			}
		}

		if !allPlain {
			continue
		}

		for _, candidate := range group {
			s := f.sections[candidate]
			out = append(out, edit{start: s.lineStart, end: s.lineEnd})
		}
	}

	return out
}

func (f *File) sectionPlainAfterDeletions(section int, deletions []edit) bool {
	if f.sections[section].plain {
		return true
	}

	for _, o := range f.options {
		if o.secIdx == section && !o.alone {
			return isDeletedOption(o, deletions)
		}
	}

	return false
}

func isDeletedOption(o *optionRec, deletions []edit) bool {
	nameStart := o.nameEnd - len(o.key.Name)

	for _, d := range deletions {
		if nameStart >= d.start && o.logicalEnd <= d.end {
			return true
		}
	}

	return false
}

func groupHasRemainingOption(options []*optionRec, group []int, deletions []edit) bool {
	for _, o := range options {
		for _, idx := range group {
			if o.secIdx == idx && !isDeletedOption(o, deletions) {
				return true
			}
		}
	}

	return false
}

func groupHasContent(data []byte, sections []*sectionRec, group []int, deletions []edit) bool {
	for _, idx := range group {
		s := sections[idx]
		for i := s.lineEnd; i < s.regionEnd && i < len(data); i++ {
			deleted := false

			for _, d := range deletions {
				if i >= d.start && i < d.end {
					deleted = true

					break
				}
			}

			if !deleted && !isSpace(data[i]) {
				return true
			}
		}
	}

	return false
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

func (f *File) rewrite(o *optionRec, key Key, value string) error {
	return f.apply([]edit{f.writeEdit(o, key, value)})
}

// writeEdit returns the splice that makes o hold value. git rewrites the whole
// "name = value" pair using the spelling from the command line, so a variable
// already in the file can come back differently capitalised.
func (f *File) writeEdit(o *optionRec, key Key, value string) edit {
	text := key.Name + " = " + encodeValue(value)

	if !o.alone {
		// The variable shares its line with its section header. git moves it
		// onto a line of its own rather than rewriting in place.
		return edit{start: f.sections[o.secIdx].headerEnd, end: o.logicalEnd, text: "\n\t" + text}
	}

	// Start at the name rather than the value, so the leading indentation is
	// kept but the old spelling is not.
	return edit{start: o.nameEnd - len(o.key.Name), end: o.logicalEnd, text: text}
}

// insert places a new variable after the last variable of the section it
// belongs to, appending a new section at end of file when there is none.
func (f *File) insert(key Key, value string) error {
	line := "\t" + key.Name + " = " + encodeValue(value) + "\n"

	for _, s := range slices.Backward(f.sections) {
		if s.key.sameSection(key) {
			text := line
			if s.entryEnd > 0 && f.data[s.entryEnd-1] != '\n' {
				// The section is the last line and lacks a terminator.
				text = "\n" + text
			}

			return f.apply([]edit{{start: s.entryEnd, end: s.entryEnd, text: text}})
		}
	}

	var b strings.Builder

	if len(f.data) > 0 && f.data[len(f.data)-1] != '\n' {
		b.WriteByte('\n')
	}

	b.WriteString(encodeHeader(key))
	b.WriteString(line)

	return f.apply([]edit{{start: len(f.data), end: len(f.data), text: b.String()}})
}

type edit struct {
	start, end int
	text       string
}

func (f *File) deleteEdit(o *optionRec) edit {
	if o.alone {
		return edit{start: o.lineStart, end: o.lineEnd}
	}

	return edit{start: f.sections[o.secIdx].headerEnd, end: o.logicalEnd}
}

// apply splices edits into the document and re-parses, so recorded offsets
// always describe the current bytes.
func (f *File) apply(edits []edit) error {
	// Descending order keeps earlier offsets valid as later ones are spliced.
	slices.SortFunc(edits, func(a, b edit) int { return b.start - a.start })

	data := f.data
	for _, e := range edits {
		out := make([]byte, 0, len(data)-(e.end-e.start)+len(e.text))
		out = append(out, data[:e.start]...)
		out = append(out, e.text...)
		out = append(out, data[e.end:]...)
		data = out
	}

	next, err := Parse(data)
	if err != nil {
		return err
	}

	*f = *next

	return nil
}

func encodeHeader(key Key) string {
	if !key.HasSubsection {
		return "[" + key.Section + "]\n"
	}

	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)

	return "[" + key.Section + ` "` + r.Replace(key.Subsection) + "\"]\n"
}

// encodeValue renders a value the way git does: control characters and quotes
// are always escaped, and the result is wrapped in quotes only when leading or
// trailing spaces or a comment character would otherwise change its meaning.
func encodeValue(v string) string {
	var b strings.Builder

	for i := range len(v) {
		switch v[i] {
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		default:
			b.WriteByte(v[i])
		}
	}

	out := b.String()
	if strings.HasPrefix(v, " ") || strings.HasSuffix(v, " ") ||
		strings.ContainsAny(v, "#;") {
		return `"` + out + `"`
	}

	return out
}
