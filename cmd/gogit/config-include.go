package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gitconfig "github.com/go-git/cli/internal/plumbing/format/config"
)

const maxConfigIncludeDepth = 10

const hasConfigRemoteURLCondition = "hasconfig:remote.*.url:"

type configIncludeContext struct {
	gitDirs    []string
	branch     string
	remoteURLs []string
}

func effectiveConfigValues(sources []configSource, key gitconfig.Key) ([]string, error) {
	ctx := configContext()

	urls, err := collectRemoteURLs(sources, &ctx)
	if err != nil {
		return nil, err
	}

	ctx.remoteURLs = urls

	var values []string

	for _, source := range sources {
		found, err := sourceValues(source, key, &ctx, map[string]bool{}, 0, false)
		if err != nil {
			return nil, err
		}

		values = append(values, found...)
	}

	return values, nil
}

func sourceValues(
	source configSource,
	key gitconfig.Key,
	ctx *configIncludeContext,
	stack map[string]bool,
	depth int,
	includedByHasConfig bool,
) ([]string, error) {
	var values []string

	for _, entry := range sourceEntries(source) {
		if includedByHasConfig && isRemoteURL(entry.Key) {
			return nil, &gitExitError{
				code: exitFatal,
				msg: "fatal: remote URLs cannot be configured in file directly or indirectly " +
					"included by includeIf.hasconfig:remote.*.url",
			}
		}

		if entry.Key.Matches(key) {
			values = append(values, entry.Value)
		}

		if !source.includes {
			continue
		}

		included, ok, err := includedSource(source, entry, ctx, stack, depth)
		if err != nil {
			return nil, err
		}

		if !ok {
			continue
		}

		condition, _ := includeCondition(entry.Key)
		found, err := sourceValues(
			included,
			key,
			ctx,
			stack,
			depth+1,
			includedByHasConfig || strings.HasPrefix(condition, hasConfigRemoteURLCondition),
		)
		delete(stack, included.location.path)

		if err != nil {
			return nil, err
		}

		values = append(values, found...)
	}

	return values, nil
}

func sourceEntries(source configSource) []gitconfig.Entry {
	if source.file != nil {
		return source.file.Entries()
	}

	entries := make([]gitconfig.Entry, 0, len(source.overrides))
	for _, override := range source.overrides {
		entries = append(entries, gitconfig.Entry{
			Key: override.key, Value: override.value, Implicit: override.implicit,
		})
	}

	return entries
}

func includedSource(
	parent configSource,
	entry gitconfig.Entry,
	ctx *configIncludeContext,
	stack map[string]bool,
	depth int,
) (configSource, bool, error) {
	condition, include := includeCondition(entry.Key)
	if !include || !conditionMatches(condition, parent.location.path, ctx) {
		return configSource{}, false, nil
	}

	if depth >= maxConfigIncludeDepth {
		return configSource{}, false, errors.New("maximum config include depth exceeded")
	}

	path, err := resolveIncludePath(entry.Value, parent.location.path)
	if err != nil {
		return configSource{}, false, err
	}

	if stack[path] {
		return configSource{}, false, fmt.Errorf("config include cycle at %s", path)
	}

	stack[path] = true

	source, err := loadSource(absoluteFile(path))
	if err != nil {
		delete(stack, path)

		return configSource{}, false, err
	}

	source.includes = true

	return source, true, nil
}

func includeCondition(key gitconfig.Key) (string, bool) {
	if !strings.EqualFold(key.Name, "path") {
		return "", false
	}

	switch {
	case strings.EqualFold(key.Section, "include") && !key.HasSubsection:
		return "", true
	case strings.EqualFold(key.Section, "includeIf") && key.HasSubsection:
		return key.Subsection, true
	default:
		return "", false
	}
}

func conditionMatches(condition, sourcePath string, ctx *configIncludeContext) bool {
	if condition == "" {
		return true
	}

	if pattern, ok := strings.CutPrefix(condition, "gitdir:"); ok {
		return matchGitDirs(pattern, sourcePath, ctx.gitDirs, false)
	}

	if pattern, ok := strings.CutPrefix(condition, "gitdir/i:"); ok {
		return matchGitDirs(pattern, sourcePath, ctx.gitDirs, true)
	}

	if pattern, ok := strings.CutPrefix(condition, "onbranch:"); ok {
		if strings.HasSuffix(pattern, "/") {
			pattern += "**"
		}

		return gitWildMatch(pattern, ctx.branch, false)
	}

	if pattern, ok := strings.CutPrefix(condition, hasConfigRemoteURLCondition); ok {
		for _, remoteURL := range ctx.remoteURLs {
			if gitWildMatch(pattern, remoteURL, false) {
				return true
			}
		}
	}

	return false
}

func matchGitDirs(pattern, sourcePath string, gitDirs []string, insensitive bool) bool {
	switch {
	case strings.HasPrefix(pattern, "~/"):
		if home := os.Getenv("HOME"); home != "" {
			pattern = filepath.Join(home, pattern[2:])
		}
	case strings.HasPrefix(pattern, "./"):
		pattern = filepath.Join(filepath.Dir(sourcePath), pattern[2:])
	case !filepath.IsAbs(pattern):
		pattern = "**/" + pattern
	}

	if strings.HasSuffix(pattern, "/") {
		pattern += "**"
	}

	patterns := []string{filepath.ToSlash(pattern)}
	if !strings.ContainsAny(pattern, "*?[") {
		if resolved, err := filepath.EvalSymlinks(pattern); err == nil && resolved != pattern {
			patterns = append(patterns, filepath.ToSlash(resolved))
		}
	}

	for _, candidatePattern := range patterns {
		for _, gitDir := range gitDirs {
			candidate := filepath.ToSlash(gitDir)
			if gitWildMatch(candidatePattern, candidate, insensitive) ||
				gitWildMatch(candidatePattern, candidate+"/", insensitive) {
				return true
			}
		}
	}

	return false
}

func gitWildMatch(pattern, value string, insensitive bool) bool {
	matcher := wildMatcher{
		pattern:     pattern,
		value:       value,
		insensitive: insensitive,
		memo:        map[wildPosition]bool{},
	}

	return matcher.match(0, 0)
}

type wildPosition struct{ pattern, value int }

type wildMatcher struct {
	pattern     string
	value       string
	insensitive bool
	memo        map[wildPosition]bool
}

func (m *wildMatcher) match(patternPos, valuePos int) bool {
	state := wildPosition{patternPos, valuePos}
	if matched, ok := m.memo[state]; ok {
		return matched
	}

	matched := m.matchPosition(patternPos, valuePos)
	m.memo[state] = matched

	return matched
}

func (m *wildMatcher) matchPosition(patternPos, valuePos int) bool {
	if patternPos == len(m.pattern) {
		return valuePos == len(m.value)
	}

	switch m.pattern[patternPos] {
	case '*':
		return m.matchStar(patternPos, valuePos)
	case '?':
		return valuePos < len(m.value) && m.value[valuePos] != '/' &&
			m.match(patternPos+1, valuePos+1)
	case '[':
		end, classMatched, ok := matchWildClass(
			m.pattern, patternPos, m.value, valuePos, m.insensitive,
		)
		if ok {
			return classMatched && m.match(end, valuePos+1)
		}
	}

	return valuePos < len(m.value) &&
		equalWildByte(m.pattern[patternPos], m.value[valuePos], m.insensitive) &&
		m.match(patternPos+1, valuePos+1)
}

func (m *wildMatcher) matchStar(patternPos, valuePos int) bool {
	end := patternPos
	for end < len(m.pattern) && m.pattern[end] == '*' {
		end++
	}

	doubleStar := end-patternPos >= 2 && (patternPos == 0 || m.pattern[patternPos-1] == '/') &&
		(end == len(m.pattern) || m.pattern[end] == '/')
	if doubleStar {
		return m.matchDoubleStar(end, valuePos)
	}

	for i := valuePos; ; i++ {
		if m.match(end, i) {
			return true
		}

		if i == len(m.value) || m.value[i] == '/' {
			return false
		}
	}
}

func (m *wildMatcher) matchDoubleStar(patternEnd, valuePos int) bool {
	if patternEnd == len(m.pattern) {
		return true
	}

	if m.match(patternEnd+1, valuePos) {
		return true
	}

	for i := valuePos; i < len(m.value); i++ {
		if m.value[i] == '/' && m.match(patternEnd+1, i+1) {
			return true
		}
	}

	return false
}

func matchWildClass(
	pattern string,
	patternPos int,
	value string,
	valuePos int,
	insensitive bool,
) (int, bool, bool) {
	if valuePos >= len(value) || value[valuePos] == '/' {
		return 0, false, false
	}

	i := patternPos + 1
	negated := false

	if i < len(pattern) && (pattern[i] == '!' || pattern[i] == '^') {
		negated = true
		i++
	}

	classStart := i
	if i < len(pattern) && pattern[i] == ']' {
		i++
	}

	for i < len(pattern) && pattern[i] != ']' {
		if pattern[i] == '\\' && i+1 < len(pattern) {
			i += 2
		} else {
			i++
		}
	}

	if i == len(pattern) {
		return 0, false, false
	}

	matched := wildClassContains(pattern[classStart:i], value[valuePos], insensitive)
	if negated {
		matched = !matched
	}

	return i + 1, matched, true
}

func wildClassContains(class string, value byte, insensitive bool) bool {
	for i := 0; i < len(class); {
		start := class[i]
		if start == '\\' && i+1 < len(class) {
			i++
			start = class[i]
		}

		i++

		if i+1 < len(class) && class[i] == '-' {
			i++

			end := class[i]
			if end == '\\' && i+1 < len(class) {
				i++
				end = class[i]
			}

			i++

			candidate := foldWildByte(value, insensitive)
			if candidate >= foldWildByte(start, insensitive) && candidate <= foldWildByte(end, insensitive) {
				return true
			}

			continue
		}

		if equalWildByte(start, value, insensitive) {
			return true
		}
	}

	return false
}

func equalWildByte(left, right byte, insensitive bool) bool {
	return foldWildByte(left, insensitive) == foldWildByte(right, insensitive)
}

func foldWildByte(value byte, insensitive bool) byte {
	if insensitive && value >= 'A' && value <= 'Z' {
		return value + ('a' - 'A')
	}

	return value
}

func resolveIncludePath(path, sourcePath string) (string, error) {
	expanded, err := expandPath(path)
	if err != nil {
		return "", err
	}

	if !filepath.IsAbs(expanded) {
		expanded = filepath.Join(filepath.Dir(sourcePath), expanded)
	}

	return filepath.Clean(expanded), nil
}

func configContext() configIncludeContext {
	gitDir, _, err := discoverGitDir()
	if err != nil {
		return configIncludeContext{}
	}

	abs, err := filepath.Abs(gitDir)
	if err != nil {
		abs = filepath.Clean(gitDir)
	}

	gitDirs := []string{abs}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil && resolved != abs {
		gitDirs = append(gitDirs, resolved)
	}

	return configIncludeContext{
		gitDirs: gitDirs,
		branch:  currentBranch(gitDir),
	}
}

func currentBranch(gitDir string) string {
	data, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return ""
	}

	ref, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "ref: refs/heads/")
	if !ok {
		return ""
	}

	return ref
}

func collectRemoteURLs(sources []configSource, ctx *configIncludeContext) ([]string, error) {
	var urls []string

	for _, source := range sources {
		found, err := sourceRemoteURLs(source, ctx, map[string]bool{}, 0)
		if err != nil {
			return nil, err
		}

		urls = append(urls, found...)
	}

	return urls, nil
}

func sourceRemoteURLs(
	source configSource,
	ctx *configIncludeContext,
	stack map[string]bool,
	depth int,
) ([]string, error) {
	var urls []string

	for _, entry := range sourceEntries(source) {
		if isRemoteURL(entry.Key) {
			urls = append(urls, entry.Value)
		}

		condition, include := includeCondition(entry.Key)
		if !source.includes || !include || strings.HasPrefix(condition, "hasconfig:") ||
			!conditionMatches(condition, source.location.path, ctx) {
			continue
		}

		included, ok, err := includedSource(source, entry, ctx, stack, depth)
		if err != nil {
			return nil, err
		}

		if !ok {
			continue
		}

		found, err := sourceRemoteURLs(included, ctx, stack, depth+1)
		delete(stack, included.location.path)

		if err != nil {
			return nil, err
		}

		urls = append(urls, found...)
	}

	return urls, nil
}

func isRemoteURL(key gitconfig.Key) bool {
	return strings.EqualFold(key.Section, "remote") && key.HasSubsection &&
		strings.EqualFold(key.Name, "url")
}
