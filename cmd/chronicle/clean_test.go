package main

import (
	"testing"
	"time"
)

// TestParseDayDuration_acceptsDaySuffix pins the most
// common usage. "30d" should produce 30 days. We accept
// the day suffix because retention thresholds are almost
// always expressed in days, and time.ParseDuration alone
// would force the user to type "720h" for the same value.
func TestParseDayDuration_acceptsDaySuffix(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"1d", 24 * time.Hour},
		{"30d", 30 * 24 * time.Hour},
		{"90d", 90 * 24 * time.Hour},
		{"365d", 365 * 24 * time.Hour},
	}
	for _, tc := range cases {
		got, err := parseDayDuration(tc.in)
		if err != nil {
			t.Errorf("parseDayDuration(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseDayDuration(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestParseDayDuration_acceptsStandardDurations confirms
// the function still handles the formats time.ParseDuration
// accepts. A user who types "12h" or "30m" should get the
// same result they would with the stdlib parser.
func TestParseDayDuration_acceptsStandardDurations(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"12h", 12 * time.Hour},
		{"30m", 30 * time.Minute},
		{"1h30m", 90 * time.Minute},
	}
	for _, tc := range cases {
		got, err := parseDayDuration(tc.in)
		if err != nil {
			t.Errorf("parseDayDuration(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseDayDuration(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestParseDayDuration_rejectsBadInputs covers the error
// branches. Empty strings, non-numeric prefixes before "d",
// negatives, and outright garbage should all produce a
// clear error rather than silently round to zero.
func TestParseDayDuration_rejectsBadInputs(t *testing.T) {
	bad := []string{
		"",
		"abc",
		"30days",  // we accept a single "d" only
		"d",       // missing the integer
		"-5d",     // negative
		"-1h",     // negative (stdlib branch)
		"30 days", // space
		"1.5d",    // fractional days are not supported by intentions
	}
	for _, in := range bad {
		if _, err := parseDayDuration(in); err == nil {
			t.Errorf("parseDayDuration(%q) returned nil error, want one", in)
		}
	}
}
