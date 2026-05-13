package main

import (
	"fmt"
	"strings"
	"sync"

	"github.com/go-git/go-git/v6/config"
)

var (
	configOverridesRaw []string
	configOverrides    = map[string]string{}
	configOverrideMu   sync.Mutex
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
// by cobra and populates the override map.
func applyConfigOverridesFromFlags() error {
	for _, raw := range configOverridesRaw {
		k, v, ok := splitKV(raw)
		if !ok {
			return fmt.Errorf("invalid -c value %q (want key=value)", raw)
		}

		applyConfigOverride(k, v)
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
