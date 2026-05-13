package main

import (
	"bytes"
	"strings"
	"testing"

	gogitconfig "github.com/go-git/go-git/v6/config"
)

func TestPickIndexVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		envValue         string
		envSet           bool
		configVersion    string
		manyFiles        string
		hadExistingIndex bool
		wantVersion      uint32
		wantWarnPrefix   string
	}{
		{name: "default", wantVersion: 2},
		{name: "env=3 silently demotes to 2", envSet: true, envValue: "3", wantVersion: 2},
		{name: "env=4", envSet: true, envValue: "4", wantVersion: 4},
		{
			name: "env=2bogus", envSet: true, envValue: "2bogus", wantVersion: 2,
			wantWarnPrefix: "warning: GIT_INDEX_VERSION set, but the value is invalid.\nUsing version 2\n",
		},
		{
			name: "env=1 out of bounds", envSet: true, envValue: "1", wantVersion: 2,
			wantWarnPrefix: "warning: GIT_INDEX_VERSION set, but the value is invalid.\nUsing version 2\n",
		},
		{name: "env bogus but existing index", envSet: true, envValue: "1", hadExistingIndex: true, wantVersion: 2},
		{name: "config=3 silently demotes to 2", configVersion: "3", wantVersion: 2},
		{
			name: "config=5 invalid", configVersion: "5", wantVersion: 2,
			wantWarnPrefix: "warning: index.version set, but the value is invalid.\nUsing version 2\n",
		},
		{name: "config invalid but existing index", configVersion: "5", hadExistingIndex: true, wantVersion: 2},
		{name: "manyFiles default 4", manyFiles: "true", wantVersion: 4},
		{name: "manyFiles overridden by config=2", configVersion: "2", manyFiles: "true", wantVersion: 2},
		{name: "env wins over config", envSet: true, envValue: "4", configVersion: "2", wantVersion: 4},
		{name: "env wins over manyFiles", envSet: true, envValue: "2", manyFiles: "true", wantVersion: 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := gogitconfig.NewConfig()
			if tc.configVersion != "" {
				cfg.Raw.Section("index").SetOption("version", tc.configVersion)
			}

			if tc.manyFiles != "" {
				cfg.Raw.Section("feature").SetOption("manyFiles", tc.manyFiles)
			}

			env := func(string) (string, bool) { return "", false }
			if tc.envSet {
				env = func(name string) (string, bool) {
					if name == "GIT_INDEX_VERSION" {
						return tc.envValue, true
					}

					return "", false
				}
			}

			var stderr bytes.Buffer

			got := pickIndexVersion(cfg, env, tc.hadExistingIndex, &stderr)
			if got != tc.wantVersion {
				t.Fatalf("version = %d; want %d", got, tc.wantVersion)
			}

			if tc.wantWarnPrefix == "" {
				if stderr.Len() != 0 {
					t.Fatalf("unexpected stderr: %q", stderr.String())
				}

				return
			}

			if !strings.HasPrefix(stderr.String(), tc.wantWarnPrefix) {
				t.Fatalf("stderr = %q; want prefix %q", stderr.String(), tc.wantWarnPrefix)
			}
		})
	}
}
