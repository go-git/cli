package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-git/go-git/v6/config"
	formatcfg "github.com/go-git/go-git/v6/plumbing/format/config"
)

var (
	configOverridesRaw []string
	configOverrides    = map[string]string{}
	configOverrideMu   sync.Mutex

	// configBackupPath is the .git/config we patched on this command's
	// behalf via `-c`. restoreConfigBackup() puts it back on exit.
	configBackupPath string
	configBackup     []byte
	configBackupCreated bool
)

// splitKV splits "<key>=<value>" into (key, value, true). Invalid input
// (no '=' or empty key) returns ("", "", false). Empty value is allowed.
func splitKV(s string) (string, string, bool) {
	idx := strings.IndexByte(s, '=')
	if idx <= 0 {
		return "", "", false
	}

	return s[:idx], s[idx+1:], true
}

func applyConfigOverride(key, value string) {
	configOverrideMu.Lock()
	defer configOverrideMu.Unlock()

	configOverrides[key] = value
}

func resetConfigOverrides() {
	configOverrideMu.Lock()
	defer configOverrideMu.Unlock()

	configOverrides = map[string]string{}
	configOverridesRaw = nil
}

// applyConfigOverridesFromFlags parses raw `-c k=v` values previously captured
// by cobra, populates the override map, and persists each value into
// .git/config so go-git's storage construction (which eagerly reads the file
// at PlainOpen time) sees the overridden values. The original config is
// restored after the subcommand returns via restoreConfigBackup.
func applyConfigOverridesFromFlags() error {
	for _, raw := range configOverridesRaw {
		k, v, ok := splitKV(raw)
		if !ok {
			return fmt.Errorf("invalid -c value %q (want key=value)", raw)
		}

		applyConfigOverride(k, v)
	}

	if len(configOverridesRaw) == 0 {
		return nil
	}

	return persistConfigOverridesToGitDir()
}

// persistConfigOverridesToGitDir writes each -c override into the on-disk
// .git/config so storage construction picks it up. Saves the original
// contents (or notes their absence) for restoreConfigBackup to revert.
//
// Outside a repository the override map is still populated for callers that
// consult it directly (configBool/hasConfigOverride); persistence is a no-op
// because there's no storage to influence.
func persistConfigOverridesToGitDir() error {
	gitDir, err := findGitDir()
	if err != nil {
		return nil //nolint:nilerr // not in a repo: persistence is a no-op
	}

	cfgPath := filepath.Join(gitDir, "config")

	existing, readErr := os.ReadFile(cfgPath)
	if readErr != nil && !os.IsNotExist(readErr) {
		return fmt.Errorf("read .git/config: %w", readErr)
	}

	raw := formatcfg.New()

	if len(existing) > 0 {
		if err := formatcfg.NewDecoder(bytes.NewReader(existing)).Decode(raw); err != nil {
			return fmt.Errorf("parse .git/config: %w", err)
		}
	}

	configOverrideMu.Lock()
	for k, v := range configOverrides {
		section, key, err := splitConfigKey(k)
		if err != nil {
			configOverrideMu.Unlock()

			return err
		}

		raw.Section(section).SetOption(key, v)
	}
	configOverrideMu.Unlock()

	configBackupPath = cfgPath
	configBackup = existing
	configBackupCreated = os.IsNotExist(readErr)

	return writeConfigFile(cfgPath, raw)
}

// restoreConfigBackup reverts the .git/config to what it was before
// persistConfigOverridesToGitDir ran. Safe to call when no backup was
// taken — it's a no-op in that case. Called from main() after rootCmd's
// Execute completes (success or error), so the on-disk config is back to
// its starting state by the time the process exits.
func restoreConfigBackup() {
	if configBackupPath == "" {
		return
	}

	if configBackupCreated {
		_ = os.Remove(configBackupPath)
	} else {
		_ = os.WriteFile(configBackupPath, configBackup, 0o644)
	}

	configBackupPath = ""
	configBackup = nil
	configBackupCreated = false
}

// hasConfigOverride reports whether key has been explicitly set via -c.
func hasConfigOverride(key string) bool {
	configOverrideMu.Lock()
	_, ok := configOverrides[key]
	configOverrideMu.Unlock()

	return ok
}

// configBool returns the effective boolean value for the given config key.
// Lookup order: -c override > defaultVal. Empty-string override means false.
// repoCfg is accepted for future expansion but not consulted in v1.
//
//nolint:unparam // key/repoCfg used by future callers (Task 7+).
func configBool(key string, repoCfg *config.Config, defaultVal bool) bool {
	configOverrideMu.Lock()
	v, ok := configOverrides[key]
	configOverrideMu.Unlock()

	_ = repoCfg

	if ok {
		return strings.EqualFold(v, "true")
	}

	return defaultVal
}
