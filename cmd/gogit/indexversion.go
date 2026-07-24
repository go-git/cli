package main

import (
	"fmt"
	"io"
	"strconv"

	"github.com/go-git/go-git/v6/config"
)

const (
	defaultIndexVersion   uint32 = 2
	manyFilesIndexVersion uint32 = 4
	gitConfigTrue                = "true"
)

// envLookup mirrors os.LookupEnv. Injected so tests can run without touching
// the process environment.
type envLookup func(string) (string, bool)

// pickIndexVersion returns the index format version to use when writing a
// new index. Precedence (highest first):
//
//  1. GIT_INDEX_VERSION env var
//  2. index.version config
//  3. feature.manyFiles=true → version 4
//  4. default version 2
//
// Bogus or out-of-range values at steps 1 and 2 fall through to the next
// source. If hadExistingIndex is false, the fall-through emits an upstream-
// compatible warning to stderr.
//
// Version 3 is parseable but mirrors upstream's behaviour: it is silently
// demoted to the default. Upstream treats it as an "explicit request for the
// default" so neither a warning nor further fall-through is appropriate.
func pickIndexVersion(cfg *config.Config, env envLookup, hadExistingIndex bool, stderr io.Writer) uint32 {
	if v, ok := env("GIT_INDEX_VERSION"); ok {
		if parsed, valid := parseGitIndexVersion(v); valid {
			return demoteV3(parsed)
		}

		if !hadExistingIndex {
			fmt.Fprintf(stderr,
				"warning: GIT_INDEX_VERSION set, but the value is invalid.\nUsing version %d\n",
				defaultIndexVersion)
		}
	}

	if cfg != nil {
		if cv := cfg.Raw.Section("index").Option("version"); cv != "" {
			if parsed, valid := parseGitIndexVersion(cv); valid {
				return demoteV3(parsed)
			}

			if !hadExistingIndex {
				fmt.Fprintf(stderr,
					"warning: index.version set, but the value is invalid.\nUsing version %d\n",
					defaultIndexVersion)
			}
		}

		if mf := cfg.Raw.Section("feature").Option("manyFiles"); mf == gitConfigTrue {
			return manyFilesIndexVersion
		}
	}

	return defaultIndexVersion
}

func parseGitIndexVersion(s string) (uint32, bool) {
	v, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0, false
	}

	if v < 2 || v > 4 {
		return 0, false
	}

	return uint32(v), true
}

func demoteV3(v uint32) uint32 {
	if v == 3 {
		return defaultIndexVersion
	}

	return v
}
