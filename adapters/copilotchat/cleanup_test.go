package copilotchat

import (
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"
)

// buildCopilotFSWithCascade lays out a fake VS Code install
// where one workspace has a session with edit snapshots, the
// same workspace has a session without any edit snapshots, and
// an empty-window chat lives under globalStorage. The fixture
// covers the three cascade shapes PlanDelete handles.
func buildCopilotFSWithCascade(t *testing.T) fstest.MapFS {
	t.Helper()
	return fstest.MapFS{
		// Session abc has a workspace and edit snapshots.
		"workspaceStorage/ws1/workspace.json":                        {Data: []byte(`{"folder":"file:///proj"}`)},
		"workspaceStorage/ws1/chatSessions/abc.jsonl":                {Data: []byte(`{"kind":0,"v":{"sessionId":"abc"}}`)},
		"workspaceStorage/ws1/chatEditingSessions/abc/state.json":    {Data: []byte("{}")},
		"workspaceStorage/ws1/chatEditingSessions/abc/contents/snap": {Data: []byte("snap data")},
		// Session def is in the same workspace but has no edit snapshots.
		"workspaceStorage/ws1/chatSessions/def.jsonl": {Data: []byte(`{"kind":0,"v":{"sessionId":"def"}}`)},
		// Session lonely lives in a folder-less window.
		"globalStorage/emptyWindowChatSessions/lonely.jsonl": {Data: []byte(`{"kind":0,"v":{"sessionId":"lonely"}}`)},
	}
}

// TestPlanDelete_workspaceSessionWithEditSnapshots checks the
// shape we expect in the most common case: a session that lives
// in a real workspace and has edit snapshots. Both items should
// land in the plan, with the session file first and the
// snapshot directory second.
func TestPlanDelete_workspaceSessionWithEditSnapshots(t *testing.T) {
	plan, err := New().PlanDelete(buildCopilotFSWithCascade(t), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Category != categoryCopilotSession {
		t.Errorf("category = %q, want %q", plan.Category, categoryCopilotSession)
	}
	if len(plan.Items) != 2 {
		t.Fatalf("plan items = %d, want 2 (session + edit snapshots); got %#v", len(plan.Items), plan.Items)
	}
	reasons := []string{plan.Items[0].Reason, plan.Items[1].Reason}
	if reasons[0] != "session file" || reasons[1] != "edit snapshots" {
		t.Errorf("reasons = %v, want [session file edit snapshots]", reasons)
	}
}

// TestPlanDelete_workspaceSessionWithoutEditSnapshots proves the
// dry-run output stays clean when a session has no snapshots.
// The user should see one item, not one plus a misleading empty
// snapshot directory entry.
func TestPlanDelete_workspaceSessionWithoutEditSnapshots(t *testing.T) {
	plan, err := New().PlanDelete(buildCopilotFSWithCascade(t), "def")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Items) != 1 {
		t.Errorf("plan items = %d, want 1 (just the session)", len(plan.Items))
	}
}

// TestPlanDelete_emptyWindowSession routes through the
// folder-less branch. The session lives under globalStorage,
// not under any workspace, so the cascade has just the one
// .jsonl file with a Reason that names the folder-less origin.
func TestPlanDelete_emptyWindowSession(t *testing.T) {
	plan, err := New().PlanDelete(buildCopilotFSWithCascade(t), "lonely")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Items) != 1 {
		t.Fatalf("plan items = %d, want 1", len(plan.Items))
	}
	if plan.Items[0].Reason != reasonSessionFileEmptyWin {
		t.Errorf("reason = %q, want %q", plan.Items[0].Reason, reasonSessionFileEmptyWin)
	}
}

// TestPlanDelete_unknownSessionWrapsErrNotExist proves the
// missing-session error can be detected with errors.Is. The CLI
// uses this to print a clean "no such session" message instead
// of the wrapped error string.
func TestPlanDelete_unknownSessionWrapsErrNotExist(t *testing.T) {
	_, err := New().PlanDelete(buildCopilotFSWithCascade(t), "no-such-id")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want one wrapping fs.ErrNotExist", err)
	}
}

// TestPlanOrphanScan_findsEditingDirsWithoutSessions lays out a
// workspace where chatEditingSessions has subdirectories for
// sessions ghost (gone) and abc (alive). The scan should
// surface ghost as an orphan and leave abc alone.
func TestPlanOrphanScan_findsEditingDirsWithoutSessions(t *testing.T) {
	fsys := fstest.MapFS{
		"workspaceStorage/ws1/chatSessions/abc.jsonl":                         {Data: []byte("{}")},
		"workspaceStorage/ws1/chatEditingSessions/abc/state.json":             {Data: []byte("{}")},
		"workspaceStorage/ws1/chatEditingSessions/ghost/state.json":           {Data: []byte("ghost")},
		"workspaceStorage/ws1/chatEditingSessions/vanished/contents/data.txt": {Data: []byte("more ghost")},
	}
	plan, err := New().PlanOrphanScan(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Category != "copilot-orphans" {
		t.Errorf("category = %q, want copilot-orphans", plan.Category)
	}
	if len(plan.Items) != 2 {
		t.Errorf("orphan items = %d, want 2 (ghost and vanished); got %#v", len(plan.Items), plan.Items)
	}
}

// TestPlanOrphanScan_emptyTreeReturnsEmptyPlan covers the
// fresh-install case. No workspaceStorage at all means no
// orphans, no error, and a clean empty plan ready for the
// caller to render.
func TestPlanOrphanScan_emptyTreeReturnsEmptyPlan(t *testing.T) {
	plan, err := New().PlanOrphanScan(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Items) != 0 {
		t.Errorf("empty install should produce zero orphans, got %d", len(plan.Items))
	}
}
