package config

import "strings"

// parser walks a configuration file byte by byte, recording the position of
// every section header and variable so they can later be spliced in place.
type parser struct {
	data []byte
	pos  int
	line int

	f      *File
	curSec *sectionRec
}

func (p *parser) run() error {
	defer func() {
		if n := len(p.f.sections); n > 0 {
			p.f.sections[n-1].regionEnd = len(p.data)
		}
	}()

	for p.pos < len(p.data) {
		lineStart := p.pos
		p.skipBlank()

		if p.atLineEnd() {
			p.consumeLineEnd()

			continue
		}

		if p.data[p.pos] == '#' || p.data[p.pos] == ';' {
			p.skipToLineEnd()
			p.consumeLineEnd()

			continue
		}

		sawHeader := false

		if p.data[p.pos] == '[' {
			if err := p.parseHeader(lineStart); err != nil {
				return err
			}

			sawHeader = true

			p.skipBlank()

			if p.atLineEnd() || p.data[p.pos] == '#' || p.data[p.pos] == ';' {
				p.curSec.plain = p.atLineEnd()
				p.skipToLineEnd()
				p.consumeLineEnd()
				p.curSec.lineEnd = p.pos
				p.curSec.entryEnd = p.pos

				continue
			}
			// Git allows a variable to follow the header on the same line.
		}

		if err := p.parseVariable(lineStart, !sawHeader); err != nil {
			return err
		}
	}

	return nil
}

func (p *parser) parseHeader(lineStart int) error {
	p.pos++ // consume '['

	start := p.pos
	for p.pos < len(p.data) && (isAlnum(p.data[p.pos]) || p.data[p.pos] == '-' || p.data[p.pos] == '.') {
		p.pos++
	}

	name := string(p.data[start:p.pos])
	if name == "" {
		return p.err()
	}

	var key Key

	p.skipBlank()

	switch {
	case p.pos < len(p.data) && p.data[p.pos] == '"':
		sub, err := p.parseSubsection()
		if err != nil {
			return err
		}

		if !validSectionName(name) {
			return p.err()
		}

		key = Key{Section: name, Subsection: sub, HasSubsection: true}
	case strings.Contains(name, "."):
		// Deprecated "[section.subsection]" form. Git lower-cases the whole
		// header, so the subsection is case-insensitive here only.
		sec, sub, _ := strings.Cut(name, ".")
		if !validSectionName(sec) {
			return p.err()
		}

		key = Key{
			Section: sec,
			// Git folds the whole "[section.subsection]" header, so only in
			// this deprecated form is the subsection case-insensitive.
			Subsection:    strings.ToLower(sub),
			HasSubsection: true,
		}
	default:
		if !validSectionName(name) {
			return p.err()
		}

		key = Key{Section: name}
	}

	p.skipBlank()

	if p.pos >= len(p.data) || p.data[p.pos] != ']' {
		return p.err()
	}

	p.pos++

	sec := &sectionRec{key: key, lineStart: lineStart, headerEnd: p.pos}
	if n := len(p.f.sections); n > 0 {
		// A section occurrence owns the file up to the next header.
		p.f.sections[n-1].regionEnd = lineStart
	}

	p.f.sections = append(p.f.sections, sec)
	p.curSec = sec

	return nil
}

func (p *parser) parseSubsection() (string, error) {
	p.pos++ // consume '"'

	var b strings.Builder

	for {
		if p.pos >= len(p.data) || p.data[p.pos] == '\n' {
			return "", p.err()
		}

		c := p.data[p.pos]
		if c == 0 {
			return "", p.err()
		}

		if c == '"' {
			p.pos++

			return b.String(), nil
		}

		if c == '\\' {
			p.pos++

			if p.pos >= len(p.data) || p.data[p.pos] == '\n' {
				return "", p.err()
			}

			b.WriteByte(p.data[p.pos])
			p.pos++

			continue
		}

		b.WriteByte(c)

		p.pos++
	}
}

func (p *parser) parseVariable(lineStart int, alone bool) error {
	if p.curSec == nil {
		// A variable before any section header has no key Git could name.
		return p.err()
	}

	nameStart := p.pos
	for p.pos < len(p.data) && (isAlnum(p.data[p.pos]) || p.data[p.pos] == '-') {
		p.pos++
	}

	name := string(p.data[nameStart:p.pos])
	if !validVariableName(name) {
		return p.err()
	}

	o := &optionRec{
		key: Key{
			Section:       p.curSec.key.Section,
			Subsection:    p.curSec.key.Subsection,
			HasSubsection: p.curSec.key.HasSubsection,
			Name:          name,
		},
		lineStart: lineStart,
		nameEnd:   p.pos,
		alone:     alone,
	}

	p.skipBlank()

	if p.pos < len(p.data) && p.data[p.pos] == '=' {
		p.pos++
		p.skipBlank()

		o.valueStart = p.pos

		value, err := p.parseValue()
		if err != nil {
			return err
		}

		o.value = value
	} else {
		o.valueless = true
		o.valueStart = o.nameEnd

		// Only a comment may follow a bare variable name. Anything else is
		// a malformed line, which git rejects rather than guessing at.
		if !p.atLineEnd() && p.data[p.pos] != '#' && p.data[p.pos] != ';' {
			return p.err()
		}
	}

	p.skipToLineEnd()

	o.logicalEnd = p.pos
	if p.pos < len(p.data) && p.data[p.pos] == '\r' {
		// git replaces the CR along with the value, so a rewritten line ends
		// up with a bare LF even in a CRLF file.
		o.logicalEnd++
	}

	p.consumeLineEnd()
	o.lineEnd = p.pos

	o.secIdx = len(p.f.sections) - 1
	p.f.options = append(p.f.options, o)
	p.curSec.entryEnd = p.pos

	return nil
}

// parseValue reads a variable's value, honouring quoted runs, backslash
// escapes and backslash-newline continuations, and trimming unquoted trailing
// whitespace.
func (p *parser) parseValue() (string, error) { //nolint:gocognit // parser state machine
	var (
		b        strings.Builder
		inQuote  bool
		lastKeep int
	)

	for p.pos < len(p.data) {
		c := p.data[p.pos]
		if c == 0 {
			return "", p.err()
		}

		switch {
		case c == '\n' || (c == '\r' && p.peekIsLF()):
			if inQuote {
				return "", p.err()
			}

			return b.String()[:lastKeep], nil

		case c == '"':
			inQuote = !inQuote
			p.pos++

		case c == '\\':
			p.pos++

			if p.pos >= len(p.data) {
				return "", p.err()
			}

			e := p.data[p.pos]
			if e == '\r' && p.peekIsLF() {
				p.pos++
				e = '\n'
			}

			if e == '\n' {
				// Line continuation: the value carries on below.
				p.pos++
				p.line++

				continue
			}

			decoded, ok := unescape(e)
			if !ok {
				return "", p.err()
			}

			b.WriteByte(decoded)

			p.pos++

			lastKeep = b.Len()

		case !inQuote && (c == '#' || c == ';'):
			return b.String()[:lastKeep], nil

		default:
			b.WriteByte(c)

			p.pos++

			if inQuote || (c != ' ' && c != '\t') {
				lastKeep = b.Len()
			}
		}
	}

	if inQuote {
		return "", p.err()
	}

	return b.String()[:lastKeep], nil
}

func unescape(c byte) (byte, bool) {
	switch c {
	case 'n':
		return '\n', true
	case 't':
		return '\t', true
	case 'b':
		return '\b', true
	case '\\':
		return '\\', true
	case '"':
		return '"', true
	}

	return 0, false
}

func (p *parser) skipBlank() {
	for p.pos < len(p.data) && (p.data[p.pos] == ' ' || p.data[p.pos] == '\t') {
		p.pos++
	}
}

func (p *parser) atLineEnd() bool {
	return p.pos >= len(p.data) || p.data[p.pos] == '\n' || (p.data[p.pos] == '\r' && p.peekIsLF())
}

func (p *parser) peekIsLF() bool {
	return p.pos+1 < len(p.data) && p.data[p.pos+1] == '\n'
}

func (p *parser) skipToLineEnd() {
	for !p.atLineEnd() {
		p.pos++
	}
}

func (p *parser) consumeLineEnd() {
	if p.pos < len(p.data) && p.data[p.pos] == '\r' {
		p.pos++
	}

	if p.pos < len(p.data) && p.data[p.pos] == '\n' {
		p.pos++
		p.line++
	}
}

func (p *parser) err() error {
	return &ParseError{Line: p.line}
}
