package main

import (
	"fmt"
	"strings"
	"sync"

	gitconfig "github.com/go-git/cli/internal/plumbing/format/config"
	"github.com/go-git/go-git/v6/config"
)

var (
	configOverridesRaw []string
	configOverrides    = map[string]string{}
	configImplicit     = map[string]bool{}
	configOverrideMu   sync.Mutex

	// configOverrideList keeps the -c overrides in the order they were given
	// and with their keys normalised, which the config command needs to
	// report repeated values and to match subsection spellings. The map above
	// stays keyed by the raw string for the existing callers.
	configOverrideList []configOverride
)

// configOverride is a single -c key=value pair with its key parsed.
type configOverride struct {
	key      gitconfig.Key
	value    string
	implicit bool
}

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
	applyConfigOverrideValue(key, value, false)
}

func applyConfigOverrideValue(key, value string, implicit bool) {
	configOverrideMu.Lock()
	defer configOverrideMu.Unlock()

	configOverrides[key] = value
	configImplicit[key] = implicit
}

func resetConfigOverrides() {
	configOverrideMu.Lock()
	defer configOverrideMu.Unlock()

	configOverrides = map[string]string{}
	configImplicit = map[string]bool{}
	configOverridesRaw = nil
	configOverrideList = nil
}

// applyConfigOverridesFromFlags parses raw `-c k=v` values previously captured
// by cobra and populates the override map.
func applyConfigOverridesFromFlags() error {
	for _, raw := range configOverridesRaw {
		k, v, ok := splitKV(raw)
		implicit := !ok

		if !ok {
			k = raw
			v = ""

			if k == "" {
				return fmt.Errorf("invalid -c value %q", raw)
			}
		}

		key, kerr := gitconfig.ParseKey(k)
		if kerr != nil {
			return fmt.Errorf("invalid -c value %q: %w", raw, kerr)
		}

		configOverrideList = append(configOverrideList, configOverride{
			key: key, value: v, implicit: implicit,
		})

		applyConfigOverrideValue(k, v, implicit)
	}

	return nil
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
//nolint:unparam // repoCfg is part of the shared config helper contract.
func configBool(key string, repoCfg *config.Config, defaultVal bool) bool {
	configOverrideMu.Lock()
	v, ok := configOverrides[key]
	implicit := configImplicit[key]
	configOverrideMu.Unlock()

	_ = repoCfg

	if ok {
		if implicit {
			return true
		}

		return strings.EqualFold(v, "true")
	}

	return defaultVal
}
