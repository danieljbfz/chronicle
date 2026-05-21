package composition

import (
	"fmt"
	"strconv"
	"time"
)

// This file holds the small human-readable formatters that
// chronicle uses anywhere it shows numbers, byte counts, or
// timestamps to a person. They are exported because the CLI
// layer also renders these kinds of values, and keeping the
// formatting logic in one place stops the two layers from
// drifting away from each other (an earlier version of
// chronicle had two near-identical copies of HumanBytes,
// one here and one in cmd/chronicle, that quietly fell out
// of sync).
//
// The functions are pure: same input, same output, no I/O.
// They never log, never look at the clock except where the
// docstring says so (HumanAge), and never allocate anything
// the caller has to release.

// HumanBytes turns a byte count into a short human-readable
// string. The function picks the largest unit that gives a
// value under 1024, so 2050 becomes "2.0KB" instead of
// "2050B" and a small file like 856 bytes stays "856B"
// instead of becoming "0.8KB". The output uses 1024-based
// units throughout, matching the convention every other
// disk-usage tool on the system already follows.
func HumanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// HumanAge turns a past timestamp into a short relative
// string like "2h ago" or "3d ago". The trash listing uses
// this to give the user a sense of how much retention time
// is left without making them do date math. We bucket into
// minutes, hours, and days because finer granularity
// (seconds, weeks) would either be useless ("3s ago") or
// misleading ("4w ago" reads like four whole calendar
// weeks even when the actual gap was 28 days that crossed
// a month boundary).
//
// The zero value of time.Time is Go's "no timestamp recorded"
// sentinel, and an adapter that did not find a timestamp in
// its source data leaves the field at that value. Passing it
// to time.Since would produce a nonsense reading along the
// lines of "106751d ago" (the number of days between Go's
// zero time and the moment of the call). The guard at the top
// returns the literal "unknown" instead, which the
// presentation layer can render as-is or pair with a muted
// style so the absence reads as deliberate rather than as a
// broken number.
func HumanAge(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours())/24)
	}
}

// Pluralize picks the singular or plural noun based on n.
// Use it instead of the parenthetical "(s)" or "(y/ies)"
// fallbacks: those read as a developer hedge ("1 entr(y/ies)
// in the trash") and accumulate a uniform user-facing
// awkwardness across the CLI. With this helper the same
// line reads as natural English ("1 entry in the trash" /
// "3 entries in the trash").
//
// Callers pass the singular and the plural separately so
// English irregulars (entry / entries, file / files) work
// without the helper having to learn pluralization rules.
func Pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

// HumanInt formats an integer with thousands separators so
// large counts stay readable at a glance. The function
// handles negative numbers correctly even though chronicle
// never produces them, because the cost of one if-statement
// is less than the cost of finding out the hard way later.
func HumanInt(n int) string {
	if n < 0 {
		return "-" + HumanInt(-n)
	}
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	// Walk the string from the right, inserting a comma
	// every three digits. We build the result in a small
	// byte slice so we do not allocate intermediate
	// strings.
	parts := make([]byte, 0, len(s)+len(s)/3)
	first := len(s) % 3
	if first > 0 {
		parts = append(parts, s[:first]...)
	}
	for i := first; i < len(s); i += 3 {
		if len(parts) > 0 {
			parts = append(parts, ',')
		}
		parts = append(parts, s[i:i+3]...)
	}
	return string(parts)
}
