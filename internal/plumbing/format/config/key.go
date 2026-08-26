// Package config parses and edits Git configuration files while preserving
// their original formatting.
//
// The go-git raw config decoder/encoder pair round-trips through a canonical
// representation: it drops every comment and cannot represent an empty
// subsection name ("[user \"\"]" is folded into "[user]"). Both are lossy in
// ways that silently corrupt a user's file, so config file mutation here is
// done by splicing bytes in the original document instead.
package config

import "strings"

// Key is a parsed configuration key such as "remote.origin.url".
//
// Every field holds the spelling it was given, because git writes a variable
// using the spelling from the command line rather than folding it. Matching,
// by contrast, ignores case for section and variable names, so compare keys
// with Matches and never with ==.
type Key struct {
	// Section is the section name as spelled. Section names match
	// case-insensitively in Git.
	Section string
	// Subsection is the subsection name. Subsection names match
	// case-sensitively in Git.
	Subsection string
	// HasSubsection distinguishes "user..name", which addresses the empty
	// subsection [user ""], from "user.name", which addresses [user].
	HasSubsection bool
	// Name is the variable name as spelled. Variable names match
	// case-insensitively in Git.
	Name string
}

// KeyNoSectionError reports a key with no '.' separator, matching git's
// "key does not contain a section" diagnostic.
type KeyNoSectionError struct{ Key string }

func (e *KeyNoSectionError) Error() string {
	return "key does not contain a section: " + e.Key
}

// KeyNoVariableError reports a key that ends at the section separator, matching
// git's "key does not contain variable name" diagnostic.
type KeyNoVariableError struct{ Key string }

func (e *KeyNoVariableError) Error() string {
	return "key does not contain variable name: " + e.Key
}

// KeyInvalidError reports a key whose section or variable name uses characters
// Git does not accept.
type KeyInvalidError struct{ Key string }

func (e *KeyInvalidError) Error() string {
	return "invalid key: " + e.Key
}

// ParseKey splits a fully qualified configuration key.
//
// Git splits at the first and the last '.': everything between them is the
// subsection, which may itself contain dots ("remote.team.one.url" addresses
// [remote "team.one"] url). A key with exactly two components has no
// subsection; "a..b" has an empty one.
func ParseKey(key string) (Key, error) {
	first := strings.IndexByte(key, '.')
	if first < 0 {
		return Key{}, &KeyNoSectionError{Key: key}
	}

	last := strings.LastIndexByte(key, '.')
	if last == len(key)-1 {
		return Key{}, &KeyNoVariableError{Key: key}
	}

	k := Key{
		Section: key[:first],
		Name:    key[last+1:],
	}

	if first != last {
		k.Subsection = key[first+1 : last]
		k.HasSubsection = true
	}

	if !validSectionName(k.Section) || !validVariableName(k.Name) {
		return Key{}, &KeyInvalidError{Key: key}
	}

	return k, nil
}

// String renders the key in the form ParseKey accepts.
func (k Key) String() string {
	if k.HasSubsection {
		return k.Section + "." + k.Subsection + "." + k.Name
	}

	return k.Section + "." + k.Name
}

// Matches reports whether k addresses the same variable as other. Section and
// variable names compare case-insensitively; subsection names compare
// byte-for-byte, and an empty subsection is distinct from none at all.
func (k Key) Matches(other Key) bool {
	return k.sameSection(other) && strings.EqualFold(k.Name, other.Name)
}

// sameSection reports whether both keys address the same section header.
func (k Key) sameSection(other Key) bool {
	return strings.EqualFold(k.Section, other.Section) &&
		k.HasSubsection == other.HasSubsection &&
		k.Subsection == other.Subsection
}

// validSectionName accepts alphanumerics and '-'. Unlike variable names, a
// section name may start with a digit.
func validSectionName(s string) bool {
	if s == "" {
		return false
	}

	for i := range len(s) {
		if !isAlnum(s[i]) && s[i] != '-' {
			return false
		}
	}

	return true
}

// validVariableName accepts alphanumerics and '-', and requires an alphabetic
// first character.
func validVariableName(s string) bool {
	if s == "" || !isAlpha(s[0]) {
		return false
	}

	for i := range len(s) {
		if !isAlnum(s[i]) && s[i] != '-' {
			return false
		}
	}

	return true
}

func isAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isAlnum(c byte) bool {
	return isAlpha(c) || (c >= '0' && c <= '9')
}
