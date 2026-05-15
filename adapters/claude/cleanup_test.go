package claude

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

// validUUID and otherUUID are the two session identifiers the
// fixtures use. Both have the canonical lowercase-UUID shape so
// the orphan scan accepts them. Using real-shaped UUIDs in
// fixtures keeps them consistent with the regex filter in the
// production code.
const (
	validUUID = "11111111-1111-1111-1111-111111111111"
	otherUUID = "22222222-2222-2222-2222-222222222222"
)

// buildClaudeFSWithCascade lays out a fake Claude install where
// one session has every per-session sibling (the .jsonl, the
// companion directory with subagents/ and tool-results/ inside,
// file-history, tasks, and session-env) and another session has
// only the bare .jsonl. The two together let one fixture cover
// both the "everything cascades" path and the "missing siblings
// drop silently" path.
func buildClaudeFSWithCascade(t *testing.T) fstest.MapFS {
	t.Helper()
	return fstest.MapFS{
		"projects/-Users-test/" + validUUID + ".jsonl":                         {Data: []byte(`{"type":"user"}`)},
		"projects/-Users-test/" + validUUID + "/subagents/sub1/file.jsonl":     {Data: []byte("subagent")},
		"projects/-Users-test/" + validUUID + "/tool-results/large-output.txt": {Data: []byte("big tool output")},
		"file-history/" + validUUID + "/snap1@v1":                              {Data: []byte("v1")},
		"file-history/" + validUUID + "/snap1@v2":                              {Data: []byte("v2 longer")},
		"tasks/" + validUUID + "/state.json":                                   {Data: []byte(`{"task":"x"}`)},
		"session-env/" + validUUID:                                             {Data: []byte("ENV=value")},
		"projects/-Users-test/" + otherUUID + ".jsonl":                         {Data: []byte(`{"type":"user"}`)},
	}
}

// TestPlanDelete_includesEverySibling proves the cascade map
// catches every per-session sibling for a session that has
// them all. With the fixture above we expect five plan items:
// the .jsonl, the companion directory, the file-history dir,
// the tasks dir, and the session-env file.
//
// The sessions/ directory at the Claude root deliberately
// stays out of the cascade. Files there describe live Claude
// processes and are keyed by PID, so they have nothing to do
// with chronicle's per-session deletes.
func TestPlanDelete_includesEverySibling(t *testing.T) {
	p := New()
	plan, err := p.PlanDelete(buildClaudeFSWithCascade(t), validUUID)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Category != categoryClaudeSession {
		t.Errorf("category = %q, want %q", plan.Category, categoryClaudeSession)
	}
	if string(plan.SessionID) != validUUID {
		t.Errorf("session id = %q, want %q", plan.SessionID, validUUID)
	}
	if len(plan.Items) != 5 {
		t.Fatalf("plan items = %d, want 5; got %#v", len(plan.Items), plan.Items)
	}

	// One of the items must be the per-session companion
	// directory. The check protects us against a regression
	// where someone removes the cascade entry for it. The
	// companion is the most common kind of orphan in practice
	// (per upstream issue #59248), so leaving it out would
	// silently bring back the bug.
	var sawCompanion bool
	for _, item := range plan.Items {
		if item.Reason == "session companion (subagents, tool results)" {
			sawCompanion = true
		}
	}
	if !sawCompanion {
		t.Error("plan should include the per-session companion directory")
	}
}

// TestPlanDelete_omitsMissingSiblings proves the dry-run output
// stays focused when a session has no siblings on disk. The
// user reading "session file (1 KB)" should not also see
// bogus zero-byte entries for siblings that do not exist.
func TestPlanDelete_omitsMissingSiblings(t *testing.T) {
	p := New()
	plan, err := p.PlanDelete(buildClaudeFSWithCascade(t), otherUUID)
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
// missing-session error can be detected with errors.Is. The
// CLI uses this to print a clean "no such session" message
// instead of dumping the wrapped error string at the user.
func TestPlanDelete_unknownSessionWrapsErrNotExist(t *testing.T) {
	p := New()
	_, err := p.PlanDelete(buildClaudeFSWithCascade(t), "no-such-id")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want one wrapping fs.ErrNotExist", err)
	}
}

// TestPlanOrphanScan_findsSiblingOrphans confirms the basic
// orphan-sibling case. The fixture has one alive session and
// three sibling entries belonging to gone sessions, one in
// each of file-history, tasks, and session-env. The scan
// should flag those three and leave the alive session's
// siblings alone.
func TestPlanOrphanScan_findsSiblingOrphans(t *testing.T) {
	fsys := fstest.MapFS{
		"projects/-Users-test/" + validUUID + ".jsonl":           {Data: []byte(`{"type":"user"}`)},
		"file-history/" + validUUID + "/x":                       {Data: []byte("alive")},
		"file-history/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/snap": {Data: []byte("ghost data")},
		"tasks/bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb/state.json":  {Data: []byte("{}")},
		"session-env/cccccccc-cccc-cccc-cccc-cccccccccccc":       {Data: []byte("env")},
	}
	p := New()
	plan, err := p.PlanOrphanScan(fsys)
	if err != nil {
		t.Fatal(err)
	}
	siblings := filterByReason(plan.Items, "orphaned file history")
	siblings = append(siblings, filterByReason(plan.Items, "orphaned task state")...)
	siblings = append(siblings, filterByReason(plan.Items, "orphaned environment capture")...)
	if len(siblings) != 3 {
		t.Errorf("sibling orphans = %d, want 3; got %#v", len(siblings), siblings)
	}
	if plan.Category != "claude-orphans" {
		t.Errorf("category = %q, want claude-orphans", plan.Category)
	}
}

// TestPlanOrphanScan_findsCompanionOrphans confirms the
// companion-directory case, which is the most common form of
// orphan in practice (per upstream issue #59248). The fixture
// has one alive session whose companion is intact, plus a
// companion directory whose .jsonl was already deleted. The
// scan should flag the second one.
//
// We also include a per-project memory directory and a
// non-UUID subdirectory to confirm the scan filters them out.
// Both are protected by the safety checks in the production
// code: memory because it is user-facing content, and the
// non-UUID name because chronicle never touches a directory
// it cannot positively identify as a session companion.
func TestPlanOrphanScan_findsCompanionOrphans(t *testing.T) {
	deadUUID := "dddddddd-dddd-dddd-dddd-dddddddddddd"
	fsys := fstest.MapFS{
		"projects/-Users-test/" + validUUID + ".jsonl":           {Data: []byte(`{"type":"user"}`)},
		"projects/-Users-test/" + validUUID + "/subagents/sub/x": {Data: []byte("alive")},
		"projects/-Users-test/" + deadUUID + "/subagents/sub/x":  {Data: []byte("orphan")},
		"projects/-Users-test/memory/MEMORY.md":                  {Data: []byte("# project memory")},
		"projects/-Users-test/.claude-history/notes.md":          {Data: []byte("third party")},
	}
	p := New()
	plan, err := p.PlanOrphanScan(fsys)
	if err != nil {
		t.Fatal(err)
	}
	companions := filterByReason(plan.Items, "orphaned session companion")
	if len(companions) != 1 {
		t.Fatalf("companion orphans = %d, want 1 (just the deadUUID one); got %#v", len(companions), companions)
	}
	if !strings.Contains(companions[0].Path, deadUUID) {
		t.Errorf("companion path = %q, want one containing %q", companions[0].Path, deadUUID)
	}
}

// TestPlanOrphanScan_emptyTreeReturnsEmptyPlan is the harmless
// case: a brand-new Claude install with no projects directory
// should produce a plan with zero items and no error.
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

// TestPlanDelete_returnedPathIsRelative pins the contract that
// adapter-produced paths are relative to the fs.FS root.
// Composition relies on this when it combines the path with
// the provider's absolute root before moving anything.
func TestPlanDelete_returnedPathIsRelative(t *testing.T) {
	plan, err := New().PlanDelete(buildClaudeFSWithCascade(t), validUUID)
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
	// Sanity: every item path should match the fs.FS-rooted
	// path we used to populate the fixture, so a re-stat
	// through the same fs.FS finds them.
	fsys := buildClaudeFSWithCascade(t)
	for _, item := range plan.Items {
		if _, err := fs.Stat(fsys, item.Path); err != nil {
			t.Errorf("item %q does not stat through the same fs.FS: %v", item.Path, err)
		}
	}
}
