package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	formatcfg "github.com/go-git/go-git/v6/plumbing/format/config"
	"github.com/spf13/cobra"
)

var configUnsetAll bool

func init() {
	configCmd.Flags().BoolVar(&configUnsetAll, "unset-all", false, "Remove all occurrences of the key")
	rootCmd.AddCommand(configCmd)
}

var configCmd = &cobra.Command{
	Use:   "config <key> [<value>]",
	Short: "Get or set repository configuration",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(_ *cobra.Command, args []string) error {
		gitDir, err := findGitDir()
		if err != nil {
			return err
		}

		cfgPath := filepath.Join(gitDir, "config")

		raw := formatcfg.New()

		if data, rerr := os.ReadFile(cfgPath); rerr == nil {
			if err := formatcfg.NewDecoder(strings.NewReader(string(data))).Decode(raw); err != nil {
				return fmt.Errorf("parse config: %w", err)
			}
		}

		section, key, err := splitConfigKey(args[0])
		if err != nil {
			return err
		}

		if configUnsetAll {
			if !raw.Section(section).HasOption(key) {
				// git exits 5 when the key is not found; test_unconfig treats 5 as ok.
				os.Exit(5) //nolint:gocritic // intentional non-error exit for git compat
			}

			raw.Section(section).RemoveOption(key)

			return writeConfigFile(cfgPath, raw)
		}

		if len(args) == 1 {
			fmt.Println(raw.Section(section).Option(key))

			return nil
		}

		raw.Section(section).SetOption(key, args[1])

		return writeConfigFile(cfgPath, raw)
	},
	DisableFlagsInUseLine: true,
	SilenceUsage:          true,
	SilenceErrors:         true,
}

func writeConfigFile(cfgPath string, raw *formatcfg.Config) error {
	f, err := os.Create(cfgPath)
	if err != nil {
		return fmt.Errorf("open config for write: %w", err)
	}

	defer f.Close()

	return formatcfg.NewEncoder(f).Encode(raw)
}

// splitConfigKey splits "section.key" into (section, key, nil).
func splitConfigKey(key string) (string, string, error) {
	parts := strings.SplitN(key, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid config key %q: want <section>.<key>", key)
	}

	return parts[0], parts[1], nil
}

// findGitDir locates the .git directory starting from the current directory.
func findGitDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		gitDir := filepath.Join(dir, ".git")
		if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
			return gitDir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}

		dir = parent
	}

	return "", errors.New("not a git repository")
}
