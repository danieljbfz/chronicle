package claude

import (
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/danieljbfz/chronicle/contracts"
)

// buildClaudeFSWithCascade lays out a fake Claude install where
// one session has every sibling artifact (file-history, tasks,
// session-env, sessions metadata) and another session has only
// the bare .jsonl. The two together let one fixture test both
// the "everything cascades" path and the "missing siblings get
// silently dropped" path.
func buildClaudeFSWithCascade(t *testing.T) fstest.MapFS {
	t.Helper()
	return fstest.MapFS{
		// Session abc has every sibling.
		"projects/-Users-test/abc.jsonl": {Data: []byte(`{"type":"user","sessionId":"abc"}`)},
		"file-history/abc/snap1@v1":      {Data: []byte("v1")},
		"file-history/abc/snap1@v2":      {Data: []byte("v2 longer")},
		"tasks/abc/state.json":           {Data: []byte(`{"task":"x"}`)},
		"session-env/abc":                {Data: []byte("ENV=value")},
		// Session def has only the .jsonl, no siblings.
		"projects/-Users-test/def.jsonl": {Data: []byte(`{"type":"user","sessionId":"def"}`)},
	}
}

// TestPlanDelete_includesEverySibling proves the cascade map
// catches every per-session sibling for a session that has
// them all. The fixture has the .jsonl, two file-history
// files, the tasks/ directory, and the session-env file. After
// the plan groups them by their top-level paths, we expect
// four plan items: the .jsonl, the file-history directory
// (counted once with its inner files summed), tasks, and
// session-env.
//
// The sessions/ directory deliberately stays out of the
// cascade. Files there describe live Claude processes and are
// keyed by PID, not by session UUID, so they have nothing to
// do with chronicle's per-session deletes.
func TestPlanDelete_includesEverySibling(t *testing.T) {
	p := New()
	plan, err := p.PlanDelete(buildClaudeFSWithCascade(t), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Category != categoryClaudeSession {
		t.Errorf("category = %q, want %q", plan.Category, categoryClaudeSession)
	}
	if plan.SessionID != "abc" {
		t.Errorf("session id = %q, want abc", plan.SessionID)
	}

	// Four top-level items: .jsonl, file-history/abc, tasks/abc,
	// session-env/abc.
	if len(plan.Items) != 4 {
		t.Fatalf("plan items = %d, want 4; got %#v", len(plan.Items), plan.Items)
	}

	// The total size should at least cover the file-history
	// directory's two inner files (sized 2 + 9 = 11) plus the
	// other small files.
	if plan.SizeBytes < 11 {
		t.Errorf("size = %d, want at least 11 (the file-history inner files)", plan.SizeBytes)
	}
}

// TestPlanDelete_omitsMissingSiblings proves the dry-run output
// stays clean when a session has no siblings. The user reading
// "session file (1 KB)" should not also see four bogus zero-byte
// entries for siblings that do not exist.
func TestPlanDelete_omitsMissingSiblings(t *testing.T) {
	p := New()
	plan, err := p.PlanDelete(buildClaudeFSWithCascade(t), "def")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Items) != 1 {
		t.Errorf("plan items = %d, want 1 (just the .jsonl)", len(plan.Items))
	}
	if plan.Items[0].Reason != "session file" {
		t.Errorf("reason = %q, want session file", plan.Items[0].Reason)
	}
}

// TestPlanDelete_unknownSessionWrapsErrNotExist proves the
// error returned for a missing session can be detected with
// errors.Is. The CLI uses this to print a clean "no such
// session" message instead of dumping the wrapped error string.
func TestPlanDelete_unknownSessionWrapsErrNotExist(t *testing.T) {
	p := New()
	_, err := p.PlanDelete(buildClaudeFSWithCascade(t), "no-such-id")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want one wrapping fs.ErrNotExist", err)
	}
}

// TestPlanOrphanScan_findsOrphans builds a fake install where
// the projects/ directory has session abc but file-history,
// tasks, and session-env have entries for sessions ghost,
// vanished, and longgone. The orphan scan should produce one
// orphan item per stale entry and zero items for abc which is
// still alive.
//
// The sessions/ directory is intentionally absent from the
// fixture. Files there describe live Claude processes and
// chronicle never touches them.
func TestPlanOrphanScan_findsOrphans(t *testing.T) {
	fsys := fstest.MapFS{
		"projects/-Users-test/abc.jsonl": {Data: []byte(`{"type":"user","sessionId":"abc"}`)},
		// Live session's siblings, should not be in the plan.
		"file-history/abc/x": {Data: []byte("alive")},
		// Orphan siblings from sessions that no longer exist.
		"file-history/ghost/snap":   {Data: []byte("ghost data")},
		"tasks/vanished/state.json": {Data: []byte("{}")},
		"session-env/longgone":      {Data: []byte("env")},
	}
	p := New()
	plan, err := p.PlanOrphanScan(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Items) != 3 {
		t.Errorf("orphan items = %d, want 3 (ghost, vanished, longgone); got %#v", len(plan.Items), plan.Items)
	}
	if plan.Category != "claude-orphans" {
		t.Errorf("category = %q, want claude-orphans", plan.Category)
	}
}

// TestPlanOrphanScan_emptyTreeReturnsEmptyPlan is the harmless
// case: a brand-new Claude install with no projects directory
// should produce a plan with zero items and no error. Returning
// a plan keeps the contract uniform and lets the caller render
// an empty result without special-casing.
func TestPlanOrphanScan_emptyTreeReturnsEmptyPlan(t *testing.T) {
	p := New()
	plan, err := p.PlanOrphanScan(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Items) != 0 {
		t.Errorf("empty install should produce zero orphans, got %d", len(plan.Items))
	}
}

// TestPlanDelete_returnedPathIsRelative pins down the contract
// that adapter-produced paths are relative to the fs.FS root.
// Composition relies on this when it joins the path with the
// provider's absolute root before moving anything.
func TestPlanDelete_returnedPathIsRelative(t *testing.T) {
	plan, err := New().PlanDelete(buildClaudeFSWithCascade(t), "abc")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range plan.Items {
		if item.Path == "" {
			t.Error("item path should not be empty")
		}
		if item.Path[0] == '/' {
			t.Errorf("item path %q should be relative, not absolute", item.Path)
		}
	}
	// Sanity: every item path should be the same as the
	// fs.FS-rooted path we used to populate the fixture, so a
	// re-stat through the same fs.FS finds them.
	fsys := buildClaudeFSWithCascade(t)
	for _, item := range plan.Items {
		if _, err := fs.Stat(fsys, item.Path); err != nil {
			t.Errorf("item %q does not stat through the same fs.FS: %v", item.Path, err)
		}
	}

	// Silence the unused-import warning in case we ever drop
	// the contracts package use above.
	_ = contracts.SessionID("")
}
