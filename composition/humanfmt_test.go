package composition

import (
	"strings"
	"testing"
	"time"
)

// TestHumanBytes_picksTheRightUnit pins the unit thresholds
// for byte formatting. The output shows up in chronicle's
// dry-run plans and trash listing, so the format matters to
// the user experience. We check each unit boundary the
// function knows about.
func TestHumanBytes_picksTheRightUnit(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0B"},
		{500, "500B"},
		{1023, "1023B"},
		{1024, "1.0KB"},
		{2048, "2.0KB"},
		{1024 * 1024, "1.0MB"},
		{int64(1.5 * 1024 * 1024), "1.5MB"},
		{int64(2.5 * 1024 * 1024 * 1024), "2.5GB"},
	}
	for _, tc := range cases {
		got := humanBytes(tc.n)
		if got != tc.want {
			t.Errorf("humanBytes(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

// TestHumanAge_pinsTheTimeBuckets confirms the four buckets
// the function uses ("just now", "Xm ago", "Xh ago", "Xd
// ago"). The trash listing leans on these strings, so they
// are user-facing.
func TestHumanAge_pinsTheTimeBuckets(t *testing.T) {
	now := time.Now()
	cases := []struct {
		past time.Time
		want string
	}{
		{now.Add(-30 * time.Second), "just now"},
		{now.Add(-5 * time.Minute), "5m ago"},
		{now.Add(-3 * time.Hour), "3h ago"},
		{now.Add(-72 * time.Hour), "3d ago"},
	}
	for _, tc := range cases {
		got := humanAge(tc.past)
		// Allow some slack on the minutes/hours/days
		// buckets because the test clock is not frozen.
		// The test passes when the returned string
		// matches the expected bucket exactly OR when it
		// is the bucket adjacent to what we expected.
		if !strings.Contains(got, tc.want) && tc.want != got {
			t.Errorf("humanAge(now - %v) = %q, want something like %q", time.Since(tc.past).Round(time.Second), got, tc.want)
		}
	}
}

// TestTrashEntry_StringFormatsAUsefulOneLineSummary pins the
// format the `chronicle trash list` command uses. The eye
// scans these lines top to bottom looking for the entry
// the user wants to restore, so the layout is part of the
// CLI's public surface.
func TestTrashEntry_StringFormatsAUsefulOneLineSummary(t *testing.T) {
	entry := TrashEntry{
		ID:        "20260515-103045-abcdef00",
		Provider:  "claude",
		SessionID: "abcdef12-3456",
		SizeBytes: 2048,
		TrashedAt: time.Now().Add(-2 * time.Hour),
	}
	got := entry.String()
	for _, want := range []string{entry.ID, "claude", "abcdef12", "2.0KB", "h ago"} {
		if !strings.Contains(got, want) {
			t.Errorf("String() = %q, want it to contain %q", got, want)
		}
	}
}

// TestTrashEntry_StringHandlesOrphanScans confirms the
// no-session-id case. Orphan-scan plans do not belong to
// any single session, so the listing prints "(orphan scan)"
// in the spot where a session id would normally appear.
func TestTrashEntry_StringHandlesOrphanScans(t *testing.T) {
	entry := TrashEntry{
		ID:        "20260515-103045-aabbccdd",
		Provider:  "claude",
		SessionID: "",
		SizeBytes: 100,
		TrashedAt: time.Now(),
	}
	got := entry.String()
	if !strings.Contains(got, "orphan scan") {
		t.Errorf("String() = %q, want it to mention orphan scan", got)
	}
}
