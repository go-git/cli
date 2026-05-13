package main

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"strconv"
	"time"
)

func runSHA1(args []string) error {
	return runHash(sha1.New(), args)
}

func runSHA256(args []string) error {
	return runHash(sha256.New(), args)
}

// runHash writes the hash of stdin: raw bytes when args is "-b", hex string + newline otherwise.
func runHash(h hash.Hash, args []string) error {
	if _, err := io.Copy(h, os.Stdin); err != nil {
		return err
	}

	sum := h.Sum(nil)

	if len(args) >= 1 && args[0] == "-b" {
		_, err := os.Stdout.Write(sum)

		return err
	}

	_, err := fmt.Fprintln(os.Stdout, hex.EncodeToString(sum))

	return err
}

// runDate implements `date is64bit` and `date time_t-is64bit`. Both return
// success on a host whose current Unix time exceeds 0x7fffffff (i.e. time_t
// is 64-bit). Used by upstream test-lib's lazy prereqs only.
func runDate(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: date {is64bit|time_t-is64bit}")
	}

	switch args[0] {
	case "is64bit", "time_t-is64bit":
		if time.Now().Unix() > 0x7fffffff {
			return nil
		}

		os.Exit(1)
	}

	return fmt.Errorf("unimplemented date subcommand: %s", args[0])
}

// runPathUtils currently implements only `path-utils file-size <path>`.
func runPathUtils(args []string) error {
	if len(args) != 2 || args[0] != "file-size" {
		return errors.New("usage: path-utils file-size <path>")
	}

	info, err := os.Stat(args[1])
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(os.Stdout, strconv.FormatInt(info.Size(), 10))

	return err
}

// runEnvHelper prints the value of an environment variable (empty if unset).
func runEnvHelper(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: env-helper <name>")
	}

	_, err := fmt.Fprintln(os.Stdout, os.Getenv(args[0]))

	return err
}
