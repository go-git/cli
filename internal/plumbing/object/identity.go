// Package object provides extensions to go-git's plumbing/object that are
// intended to be upstreamed. Currently it covers parsing of the
// "<unix-seconds> <±HHMM>" format used by the GIT_AUTHOR_DATE,
// GIT_COMMITTER_DATE, and related environment variables.
package object

import (
	"fmt"
	"time"
)

// ParseGitDate parses the date format used by GIT_AUTHOR_DATE / GIT_COMMITTER_DATE:
// "<unix-seconds> <±HHMM>". Returns a time.Time anchored at the given Unix
// instant and located in a FixedZone built from the offset.
//
// Malformed input — wrong number of fields, non-numeric seconds, missing or
// invalid zone — returns an error rather than panicking.
func ParseGitDate(s string) (time.Time, error) {
	var (
		secs int64
		zone string
	)

	if _, err := fmt.Sscanf(s, "%d %s", &secs, &zone); err != nil {
		return time.Time{}, fmt.Errorf("invalid git date %q: %w", s, err)
	}

	// time.Parse with the canonical "-0700" layout validates length and digits
	// and yields the correctly-signed offset (manual arithmetic on the sign
	// byte is easy to get wrong for negative zones).
	zt, err := time.Parse("-0700", zone)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid zone %q in git date %q: %w", zone, s, err)
	}

	_, offset := zt.Zone()

	return time.Unix(secs, 0).In(time.FixedZone(zone, offset)), nil
}
