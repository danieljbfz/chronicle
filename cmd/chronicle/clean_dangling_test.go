package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/danieljbfz/chronicle/composition"
	"github.com/danieljbfz/chronicle/contracts"
)

// TestFilterDanglingEntries_keepsOnlyMissingDirs is the
// CLI-side filter contract. We pass in a mix of live and
// dangling listings and confirm only the dangling ones
// come back. The composition layer returns everything;
// the CLI is the layer that decides what to act on.
func TestFilterDanglingEntries_keepsOnlyMissingDirs(t *testing.T) {
	in := []composition.ConfigProjectListing{
		{Provider: "claude", Entry: contracts.ConfigProjectEntry{Key: "/keep", Exists: true}},
		{Provider: "claude", Entry: contracts.ConfigProjectEntry{Key: "/gone1", Exists: false}},
		{Provider: "claude", Entry: contracts.ConfigProjectEntry{Key: "/keep2", Exists: true}},
		{Provider: "claude", Entry: contracts.ConfigProjectEntry{Key: "/gone2", Exists: false}},
	}
	out := filterDanglingEntries(in)
	if len(out) != 2 {
		t.Fatalf("filtered = %d, want 2", len(out))
	}
	for _, l := range out {
		if l.Entry.Exists {
			t.Errorf("dangling slice should not contain Exists=true entry %q", l.Entry.Key)
		}
	}
}

// TestRunDanglingCleanup_dryRunPrintsPlan covers the
// dry-run rendering. The user should see the count, each
// entry, and a reminder that --apply is what actually
// performs the removal.
func TestRunDanglingCleanup_dryRunPrintsPlan(t *testing.T) {
	dangling := []composition.ConfigProjectListing{
		{Provider: "claude", Entry: contracts.ConfigProjectEntry{
			Key: "/Users/x/work/gone-project", SizeBytes: 1024,
		}},
	}
	var buf bytes.Buffer
	// We pass nil for App because the dry-run path never
	// touches it; the apply path would, and that is
	// covered by the composition-layer tests against a
	// real fake App.
	if err := runDanglingCleanup(nil, dangling, false, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"Found 1 dangling config-project",
		"claude",
		"/Users/x/work/gone-project",
		"1.0KB",
		"--apply",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q in:\n%s", want, out)
		}
	}
}

// TestRunDanglingCleanup_emptySaysSo confirms the
// no-dangling-entries branch. Every project in the config
// still exists, so the CLI prints a clear message instead
// of a blank screen.
func TestRunDanglingCleanup_emptySaysSo(t *testing.T) {
	var buf bytes.Buffer
	if err := runDanglingCleanup(nil, nil, false, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "No dangling config-project entries") {
		t.Errorf("empty output should explain itself, got: %q", buf.String())
	}
}
