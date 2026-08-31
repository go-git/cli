package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	if source.file == nil {
		var values []string

		for _, override := range source.overrides {
			if override.key.Matches(key) {
				values = append(values, override.value)
			}
		}

		return values, nil
	}

	var values []string

	for _, entry := range source.file.Entries() {
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
	var expression strings.Builder
	expression.WriteByte('^')

	for i := 0; i < len(pattern); {
		switch pattern[i] {
		case '*':
			start := i
			for i < len(pattern) && pattern[i] == '*' {
				i++
			}

			doubleStar := i-start >= 2 && (start == 0 || pattern[start-1] == '/') &&
				(i == len(pattern) || pattern[i] == '/')
			if !doubleStar {
				expression.WriteString("[^/]*")

				continue
			}

			if i < len(pattern) {
				expression.WriteString("(?:.*/)?")

				i++
			} else {
				expression.WriteString(".*")
			}
		case '?':
			expression.WriteString("[^/]")

			i++
		case '[':
			end := strings.IndexByte(pattern[i+1:], ']')
			if end < 0 {
				expression.WriteString(`\[`)

				i++

				continue
			}

			end += i + 1
			class := pattern[i+1 : end]

			expression.WriteByte('[')

			if strings.HasPrefix(class, "!") {
				expression.WriteByte('^')

				class = class[1:]
			}

			expression.WriteString(strings.ReplaceAll(class, `\`, `\\`))
			expression.WriteByte(']')

			i = end + 1
		default:
			expression.WriteString(regexp.QuoteMeta(pattern[i : i+1]))
			i++
		}
	}

	expression.WriteByte('$')

	patternExpression := expression.String()
	if insensitive {
		patternExpression = "(?i:" + patternExpression + ")"
	}

	compiled, err := regexp.Compile(patternExpression)

	return err == nil && compiled.MatchString(value)
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
	if source.file == nil {
		var urls []string

		for _, override := range source.overrides {
			if isRemoteURL(override.key) {
				urls = append(urls, override.value)
			}
		}

		return urls, nil
	}

	var urls []string

	for _, entry := range source.file.Entries() {
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
