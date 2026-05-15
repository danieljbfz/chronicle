package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/danieljbfz/chronicle/composition"
	"github.com/danieljbfz/chronicle/contracts"
)

// sampleStats builds a small two-provider Stats fixture the
// rendering tests share. Real values come from App.Stats in
// production. The rendering layer only needs a well-formed
// in-memory value to exercise its branches.
func sampleStats() composition.Stats {
	oldest := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newest := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	return composition.Stats{
		GeneratedAt: time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC),
		Total: composition.Aggregate{
			Sessions:  150,
			Messages:  12345,
			SizeBytes: 5 * 1024 * 1024,
			OldestAt:  oldest,
			NewestAt:  newest,
		},
		Providers: []composition.ProviderStats{
			{Name: "claude", Projects: 3, Aggregate: composition.Aggregate{Sessions: 100, Messages: 10000, SizeBytes: 4 * 1024 * 1024, OldestAt: oldest, NewestAt: newest}},
			{Name: "copilot", Projects: 2, Aggregate: composition.Aggregate{Sessions: 50, Messages: 2345, SizeBytes: 1 * 1024 * 1024}},
		},
		TopProjects: []composition.ProjectStats{
			{Provider: "claude", ProjectID: contracts.ProjectID("p1"), Path: "/Users/x/work/foo", Aggregate: composition.Aggregate{Sessions: 60, SizeBytes: 2 * 1024 * 1024}},
			{Provider: "copilot", ProjectID: contracts.ProjectID("p2"), DisplayName: "ws-bar", Aggregate: composition.Aggregate{Sessions: 40, SizeBytes: 1024 * 1024}},
		},
	}
}

// TestWriteStatsText_includesTotalsByProviderAndTopProjects
// is the rendering happy path. The summary has to surface
// the totals row, the per-provider breakdown, and the top
// projects list. We assert on the formatted strings so the
// output stays stable and so a regression in any one of
// the three sections is caught individually.
func TestWriteStatsText_includesTotalsByProviderAndTopProjects(t *testing.T) {
	var buf bytes.Buffer
	if err := writeStatsText(&buf, sampleStats()); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	for _, want := range []string{
		"Totals",
		"Sessions: 150",
		"Messages: 12,345",
		"5.0MB",
		"Active:   2026-01-01 -> 2026-05-15",
		"By provider",
		"claude: 100 sessions, 10,000 messages, 4.0MB across 3 projects",
		"copilot: 50 sessions, 2,345 messages, 1.0MB across 2 projects",
		"Top 2 projects by session count",
		"/Users/x/work/foo",
		"ws-bar",
		"Generated at 2026-05-15T12:00:00Z",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q in:\n%s", want, out)
		}
	}
}

// TestWriteStatsText_skipsActiveLineWhenRangeMissing
// confirms the "no timestamps" branch. A fresh install
// or a provider whose summaries lack timestamps must not
// print "Active: 0001-01-01 -> 0001-01-01," which would
// be misleading.
func TestWriteStatsText_skipsActiveLineWhenRangeMissing(t *testing.T) {
	stats := sampleStats()
	stats.Total.OldestAt = time.Time{}
	stats.Total.NewestAt = time.Time{}

	var buf bytes.Buffer
	if err := writeStatsText(&buf, stats); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "Active:") {
		t.Errorf("output should omit the Active line when no range, got:\n%s", buf.String())
	}
}

// TestDateRange_zeroTimestampsReturnsEmpty pins the helper
// behind the active-line decision. The empty string is the
// caller's signal that the line should be skipped.
func TestDateRange_zeroTimestampsReturnsEmpty(t *testing.T) {
	got := dateRange(composition.Aggregate{})
	if got != "" {
		t.Errorf("dateRange(zero) = %q, want empty", got)
	}
}

// TestDateRange_formatsSpanInDays confirms the rendered
// string. Days are rounded down (the difference between
// 2026-01-01 and 2026-05-15 is exactly 134 days), and the
// arrow uses ASCII so the line stays readable in any
// terminal regardless of font support for unicode arrows.
func TestDateRange_formatsSpanInDays(t *testing.T) {
	got := dateRange(composition.Aggregate{
		OldestAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		NewestAt: time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC),
	})
	want := "2026-01-01 -> 2026-05-15  (134 days)"
	if got != want {
		t.Errorf("dateRange = %q, want %q", got, want)
	}
}

// TestWriteStatsJSON_emitsOneIndentedDocument confirms the
// JSON shape. Stats is a single document (not JSON Lines)
// because the output is one snapshot, not a stream. We
// round-trip through Unmarshal to assert on the structured
// fields rather than fragile substring checks.
func TestWriteStatsJSON_emitsOneIndentedDocument(t *testing.T) {
	var buf bytes.Buffer
	if err := writeStatsJSON(&buf, sampleStats()); err != nil {
		t.Fatal(err)
	}

	var got statsJSON
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("stats JSON did not parse: %v", err)
	}
	if got.Total.Sessions != 150 || got.Total.Messages != 12345 {
		t.Errorf("totals = %+v, want sessions=150 messages=12345", got.Total)
	}
	if len(got.Providers) != 2 {
		t.Fatalf("providers = %d, want 2", len(got.Providers))
	}
	if got.Providers[0].Name != "claude" {
		t.Errorf("first provider = %q, want claude", got.Providers[0].Name)
	}
	if len(got.TopProjects) != 2 {
		t.Fatalf("top projects = %d, want 2", len(got.TopProjects))
	}
	if got.TopProjects[0].Path != "/Users/x/work/foo" {
		t.Errorf("top[0].path = %q, want /Users/x/work/foo", got.TopProjects[0].Path)
	}
	// The indented form keeps human-readable JSON, which
	// only matters if a developer prints it without piping
	// through jq. We confirm the indentation so a future
	// switch to a one-line encoder breaks this test.
	if !strings.Contains(buf.String(), "\n  ") {
		t.Errorf("expected indented JSON, got:\n%s", buf.String())
	}
}

// TestToAggregateJSON_omitsZeroTimestamps confirms the
// JSON wire-format hides empty time values instead of
// surfacing them as "0001-01-01T00:00:00Z," which would
// confuse downstream tooling.
func TestToAggregateJSON_omitsZeroTimestamps(t *testing.T) {
	got := toAggregateJSON(composition.Aggregate{
		Sessions: 5,
	})
	if got.OldestAt != "" || got.NewestAt != "" {
		t.Errorf("zero timestamps should round-trip as empty strings, got oldest=%q newest=%q", got.OldestAt, got.NewestAt)
	}
}
