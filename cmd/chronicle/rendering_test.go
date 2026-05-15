package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/danieljbfz/chronicle/composition"
	"github.com/danieljbfz/chronicle/contracts"
)

// TestWriteMemoryList_emptyShowsExplicitMessage confirms the
// no-memory case prints something useful instead of a blank
// screen. A blank screen would make the user think
// `chronicle memory list` had crashed.
func TestWriteMemoryList_emptyShowsExplicitMessage(t *testing.T) {
	var buf bytes.Buffer
	if err := writeMemoryList(&buf, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "No per-project memory files") {
		t.Errorf("empty memory list should explain itself, got: %q", buf.String())
	}
}

// TestWriteMemoryList_rendersOneLinePerEntry pins the
// format. Each entry takes one line and includes the
// fields the user needs to decide what to do: provider,
// project, filename, size, and modification date.
func TestWriteMemoryList_rendersOneLinePerEntry(t *testing.T) {
	entries := []composition.MemoryListing{{
		Provider:   "claude",
		ProjectID:  "-Users-test-proj",
		FileName:   "MEMORY.md",
		SizeBytes:  1024,
		ModifiedAt: "2026-05-15 10:30",
	}}
	var buf bytes.Buffer
	if err := writeMemoryList(&buf, entries); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"claude", "-Users-test-proj", "MEMORY.md", "1.0KB", "2026-05-15 10:30"} {
		if !strings.Contains(out, want) {
			t.Errorf("memory list missing %q in:\n%s", want, out)
		}
	}
}

// TestWriteTrashList_emptyShowsExplicitMessage confirms the
// empty-trash case prints "Trash is empty." instead of just
// the count line, so the user sees an unambiguous signal
// rather than a header with nothing under it.
func TestWriteTrashList_emptyShowsExplicitMessage(t *testing.T) {
	var buf bytes.Buffer
	if err := writeTrashList(&buf, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Trash is empty") {
		t.Errorf("empty trash should say so, got: %q", buf.String())
	}
}

// TestWriteTrashList_rendersOneLinePerEntry pins the format
// for trash entries. Each entry takes one line built by
// TrashEntry.String, which the trash list writer relies on
// without rewriting the format itself.
func TestWriteTrashList_rendersOneLinePerEntry(t *testing.T) {
	entries := []composition.TrashEntry{{
		ID:        "20260515-103045-abcdef00",
		Provider:  "claude",
		SessionID: "abcdef12-3456",
		SizeBytes: 2048,
		TrashedAt: time.Now().Add(-2 * time.Hour),
	}}
	var buf bytes.Buffer
	if err := writeTrashList(&buf, entries); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{entries[0].ID, "claude", "2.0KB"} {
		if !strings.Contains(out, want) {
			t.Errorf("trash list missing %q in:\n%s", want, out)
		}
	}
}

// TestRunClean_dryRunPrintsPlanAndReminder confirms the
// default-dry-run behaviour. With apply=false, runClean
// prints every item that would move and ends with a
// "pass --apply" hint. The user reads this output to
// decide whether to confirm the cleanup.
func TestRunClean_dryRunPrintsPlanAndReminder(t *testing.T) {
	planned := []composition.PlannedDeletion{{
		Plan: contracts.DeletePlan{
			SessionID: "s1",
			SizeBytes: 500,
			Items: []contracts.DeleteItem{{
				Path:      "projects/p/s1.jsonl",
				Reason:    "session file",
				SizeBytes: 500,
			}},
		},
	}}
	var buf bytes.Buffer
	// We pass a nil App because the dry-run path never
	// touches it; the apply path would, and that branch is
	// covered by the composition-layer tests against a real
	// App.
	if err := runClean(nil, planned, false, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"Found 1 session", "projects/p/s1.jsonl", "session file", "dry-run", "--apply"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q in:\n%s", want, out)
		}
	}
}

// TestRunClean_emptyPlanPrintsNothingToCleanMessage covers
// the "nothing to do" branch. A clean command that finds no
// matching sessions should print a short, unambiguous
// message instead of an empty table.
func TestRunClean_emptyPlanPrintsNothingToCleanMessage(t *testing.T) {
	var buf bytes.Buffer
	if err := runClean(nil, nil, false, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Nothing to clean") {
		t.Errorf("empty plan should say so, got: %q", buf.String())
	}
}
