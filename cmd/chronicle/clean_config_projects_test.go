package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/danieljbfz/chronicle/composition"
	"github.com/danieljbfz/chronicle/contracts"
)

// TestFilterStaleConfigEntries_keepsOnlyMissingDirs is
// the CLI-side filter contract. We pass in a mix of live
// and stale listings and confirm only the stale ones come
// back. The composition layer returns everything; the CLI
// is the layer that decides what to act on.
func TestFilterStaleConfigEntries_keepsOnlyMissingDirs(t *testing.T) {
	in := []composition.ConfigProjectListing{
		{Provider: "claude", Entry: contracts.ConfigProjectEntry{Key: "/keep", Exists: true}},
		{Provider: "claude", Entry: contracts.ConfigProjectEntry{Key: "/gone1", Exists: false}},
		{Provider: "claude", Entry: contracts.ConfigProjectEntry{Key: "/keep2", Exists: true}},
		{Provider: "claude", Entry: contracts.ConfigProjectEntry{Key: "/gone2", Exists: false}},
	}
	out := filterStaleConfigEntries(in)
	if len(out) != 2 {
		t.Fatalf("filtered = %d, want 2", len(out))
	}
	for _, l := range out {
		if l.Entry.Exists {
			t.Errorf("stale slice should not contain Exists=true entry %q", l.Entry.Key)
		}
	}
}

// TestRunConfigProjectsCleanup_dryRunPrintsPlan covers
// the dry-run rendering. The user should see the count,
// each entry, and a reminder that --apply is what
// actually performs the removal.
func TestRunConfigProjectsCleanup_dryRunPrintsPlan(t *testing.T) {
	stale := []composition.ConfigProjectListing{
		{Provider: "claude", Entry: contracts.ConfigProjectEntry{
			Key: "/Users/x/work/gone-project", SizeBytes: 1024,
		}},
	}
	var buf bytes.Buffer
	// We pass nil for App because the dry-run path never
	// touches it; the apply path would, and that is
	// covered by the composition-layer tests against a
	// real fake App.
	if err := runConfigProjectsCleanup(nil, stale, false, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"Found 1 stale config-project",
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

// TestRunConfigProjectsCleanup_emptySaysSo confirms the
// no-stale-entries branch. Every project in the config
// still exists, so the CLI prints a clear message instead
// of a blank screen.
func TestRunConfigProjectsCleanup_emptySaysSo(t *testing.T) {
	var buf bytes.Buffer
	if err := runConfigProjectsCleanup(nil, nil, false, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "No stale config-project entries") {
		t.Errorf("empty output should explain itself, got: %q", buf.String())
	}
}
