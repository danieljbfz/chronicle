package claude

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/danieljbfz/chronicle/contracts"
)

// TestPlanOrphanScan_findsPasteCacheOrphans confirms we flag
// paste-cache files whose hash is not referenced anywhere in
// history.jsonl. The fixture has two cached pastes, only one
// of which is mentioned in the history. The other one should
// land in the plan with the matching reason.
func TestPlanOrphanScan_findsPasteCacheOrphans(t *testing.T) {
	fsys := fstest.MapFS{
		"history.jsonl":          {Data: []byte(`{"display":"hi","pastedContents":{"1":{"contentHash":"alive"}}}` + "\n")},
		"paste-cache/alive.txt":  {Data: []byte("still in use")},
		"paste-cache/orphan.txt": {Data: []byte("nobody references me")},
	}
	plan, err := New().PlanOrphanScan(fsys)
	if err != nil {
		t.Fatal(err)
	}
	pasteOrphans := filterByReason(plan.Items, reasonOrphanPaste)
	if len(pasteOrphans) != 1 {
		t.Fatalf("got %d paste orphans, want 1", len(pasteOrphans))
	}
	if !strings.HasSuffix(pasteOrphans[0].Path, "orphan.txt") {
		t.Errorf("orphan path = %q, want one ending in orphan.txt", pasteOrphans[0].Path)
	}
}

// TestPlanOrphanScan_skipsPasteCacheWhenHistoryMissing
// confirms the safety property: with no history.jsonl on disk,
// we cannot tell which caches are referenced, so we leave the
// whole paste-cache directory alone. Removing files based on
// missing reference data would risk deleting active caches.
func TestPlanOrphanScan_skipsPasteCacheWhenHistoryMissing(t *testing.T) {
	fsys := fstest.MapFS{
		"paste-cache/maybe-alive.txt": {Data: []byte("we cannot tell")},
	}
	plan, err := New().PlanOrphanScan(fsys)
	if err != nil {
		t.Fatal(err)
	}
	pasteOrphans := filterByReason(plan.Items, reasonOrphanPaste)
	if len(pasteOrphans) != 0 {
		t.Errorf("got %d paste orphans, want 0 when history.jsonl is missing", len(pasteOrphans))
	}
}

// TestPlanOrphanScan_findsSecurityWarningOrphans confirms we
// flag security_warnings_state files whose session UUID does
// not match a live session. The fixture has one alive session
// and two stale state files, so we expect two orphans.
func TestPlanOrphanScan_findsSecurityWarningOrphans(t *testing.T) {
	fsys := fstest.MapFS{
		"projects/-Users-test/abc.jsonl":              {Data: []byte(`{"type":"user"}`)},
		"security_warnings_state_abc.json":            {Data: []byte("{}")},
		"security_warnings_state_dead-session-1.json": {Data: []byte("{}")},
		"security_warnings_state_dead-session-2.json": {Data: []byte("{}")},
	}
	plan, err := New().PlanOrphanScan(fsys)
	if err != nil {
		t.Fatal(err)
	}
	warningOrphans := filterByReason(plan.Items, reasonOrphanWarning)
	if len(warningOrphans) != 2 {
		t.Errorf("got %d warning orphans, want 2", len(warningOrphans))
	}
}

// TestPlanOrphanScan_keepsRecentShellSnapshots confirms we
// keep the most recent N snapshots and only flag the older
// ones. The fixture has seven snapshots, and the keep count is
// five, so we expect the two oldest to be flagged.
//
// Snapshot names embed an epoch-millis timestamp in their
// prefix, so we use string sort (newest first) to determine
// which to keep. The fixture names are chosen to make the
// expected order obvious.
func TestPlanOrphanScan_keepsRecentShellSnapshots(t *testing.T) {
	fsys := fstest.MapFS{}
	for i, ts := range []string{
		"snapshot-zsh-1700000001000-a.sh", // oldest
		"snapshot-zsh-1700000002000-b.sh",
		"snapshot-zsh-1700000003000-c.sh",
		"snapshot-zsh-1700000004000-d.sh",
		"snapshot-zsh-1700000005000-e.sh",
		"snapshot-zsh-1700000006000-f.sh",
		"snapshot-zsh-1700000007000-g.sh", // newest
	} {
		fsys["shell-snapshots/"+ts] = &fstest.MapFile{Data: []byte(string(rune('A' + i)))}
	}
	plan, err := New().PlanOrphanScan(fsys)
	if err != nil {
		t.Fatal(err)
	}
	snapshotOrphans := filterByReason(plan.Items, reasonOrphanShellSnap)
	if len(snapshotOrphans) != 2 {
		t.Fatalf("got %d snapshot orphans, want 2", len(snapshotOrphans))
	}
	// The two oldest are the ones flagged.
	for _, item := range snapshotOrphans {
		if !strings.Contains(item.Path, "1700000001000") && !strings.Contains(item.Path, "1700000002000") {
			t.Errorf("unexpected snapshot orphan: %s", item.Path)
		}
	}
}

// TestPlanOrphanScan_skipsSnapshotsBelowKeepCount confirms we
// do not flag anything when the directory has fewer snapshots
// than the keep count. A user with three snapshots should not
// see them in the orphan plan.
func TestPlanOrphanScan_skipsSnapshotsBelowKeepCount(t *testing.T) {
	fsys := fstest.MapFS{
		"shell-snapshots/snapshot-zsh-1.sh": {Data: []byte("a")},
		"shell-snapshots/snapshot-zsh-2.sh": {Data: []byte("b")},
		"shell-snapshots/snapshot-zsh-3.sh": {Data: []byte("c")},
	}
	plan, err := New().PlanOrphanScan(fsys)
	if err != nil {
		t.Fatal(err)
	}
	snapshotOrphans := filterByReason(plan.Items, reasonOrphanShellSnap)
	if len(snapshotOrphans) != 0 {
		t.Errorf("got %d snapshot orphans with three files, want 0", len(snapshotOrphans))
	}
}

// TestPlanOrphanScan_keepsRecentBackups mirrors the snapshot
// test for the backups directory. Same shape: we keep the
// newest five backups and flag everything older.
func TestPlanOrphanScan_keepsRecentBackups(t *testing.T) {
	fsys := fstest.MapFS{}
	for _, ts := range []string{
		".claude.json.backup.1700000001000",
		".claude.json.backup.1700000002000",
		".claude.json.backup.1700000003000",
		".claude.json.backup.1700000004000",
		".claude.json.backup.1700000005000",
		".claude.json.backup.1700000006000",
		".claude.json.backup.1700000007000",
	} {
		fsys["backups/"+ts] = &fstest.MapFile{Data: []byte("backup")}
	}
	plan, err := New().PlanOrphanScan(fsys)
	if err != nil {
		t.Fatal(err)
	}
	backupOrphans := filterByReason(plan.Items, reasonOrphanBackup)
	if len(backupOrphans) != 2 {
		t.Fatalf("got %d backup orphans, want 2", len(backupOrphans))
	}
}

// TestPlanOrphanScan_ignoresNonBackupFilesInBackupsDir
// confirms we only touch files whose name matches the backup
// pattern. If the user has put their own files in the backups
// directory, we leave them alone.
func TestPlanOrphanScan_ignoresNonBackupFilesInBackupsDir(t *testing.T) {
	fsys := fstest.MapFS{
		// Six real backups, plus one user file we should ignore.
		".claude.json.backup.1700000001000":         nil, // path inside fsys, see below
		"backups/.claude.json.backup.1700000001000": {Data: []byte("a")},
		"backups/.claude.json.backup.1700000002000": {Data: []byte("b")},
		"backups/.claude.json.backup.1700000003000": {Data: []byte("c")},
		"backups/.claude.json.backup.1700000004000": {Data: []byte("d")},
		"backups/.claude.json.backup.1700000005000": {Data: []byte("e")},
		"backups/.claude.json.backup.1700000006000": {Data: []byte("f")},
		"backups/my-personal-notes.txt":             {Data: []byte("do not touch")},
	}
	delete(fsys, ".claude.json.backup.1700000001000")
	plan, err := New().PlanOrphanScan(fsys)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range plan.Items {
		if strings.Contains(item.Path, "my-personal-notes.txt") {
			t.Error("the personal-notes file should be left alone")
		}
	}
}

// filterByReason returns the items in the plan whose Reason
// matches the given string. The orphan scan returns one
// combined plan with every kind of orphan in a single slice,
// so the tests use this helper to pick out just the items
// they care about.
func filterByReason(items []contracts.DeleteItem, reason string) []contracts.DeleteItem {
	var out []contracts.DeleteItem
	for _, item := range items {
		if item.Reason == reason {
			out = append(out, item)
		}
	}
	return out
}
