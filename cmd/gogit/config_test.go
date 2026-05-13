package main

import (
	"testing"
)

func TestSplitKV(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		k, v string
		ok   bool
	}{
		{in: "pack.writeReverseIndex=true", k: "pack.writeReverseIndex", v: "true", ok: true},
		{in: "pack.writeReverseIndex=", k: "pack.writeReverseIndex", v: "", ok: true},
		{in: "noequals", ok: false},
		{in: "", ok: false},
		{in: "a=b=c", k: "a", v: "b=c", ok: true},
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()

			k, v, ok := splitKV(tc.in)
			if ok != tc.ok || (ok && (k != tc.k || v != tc.v)) {
				t.Fatalf("splitKV(%q) = (%q, %q, %v); want (%q, %q, %v)", tc.in, k, v, ok, tc.k, tc.v, tc.ok)
			}
		})
	}
}

//nolint:paralleltest // mutates global configOverrides
func TestConfigBoolOverridesRepoConfig(t *testing.T) {
	resetConfigOverrides()
	t.Cleanup(resetConfigOverrides)

	if got := configBool("pack.writeReverseIndex", nil, false); got != false {
		t.Fatalf("default false: got %v", got)
	}

	applyConfigOverride("pack.writeReverseIndex", "true")

	if got := configBool("pack.writeReverseIndex", nil, false); got != true {
		t.Fatalf("override true: got %v", got)
	}

	applyConfigOverride("pack.writeReverseIndex", "")

	if got := configBool("pack.writeReverseIndex", nil, true); got != false {
		t.Fatalf("empty override means false: got %v", got)
	}
}
