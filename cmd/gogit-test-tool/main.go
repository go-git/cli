// Package main is the gogit-test-tool helper: a drop-in for upstream's
// t/helper/test-tool used during conformance runs. Subcommands implemented
// here mirror the upstream subcommands the curated tests actually exercise.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: gogit-test-tool <subcommand> [args...]")
	}

	sub, rest := args[0], args[1:]

	switch sub {
	case "genrandom":
		return runGenRandom(rest)
	case "delta":
		return runDelta(rest)
	case "sha1":
		return runSHA1(rest)
	case "sha256":
		return runSHA256(rest)
	case "date":
		return runDate(rest)
	case "path-utils":
		return runPathUtils(rest)
	case "env-helper":
		return runEnvHelper(rest)
	default:
		return fmt.Errorf("unimplemented subcommand: %s", sub)
	}
}

// stdoutWriter returns the writer that subcommands emit raw output to.
// Indirected so tests can drop a buffer in if needed (not done in v1).
func stdoutWriter() io.Writer {
	return os.Stdout
}
