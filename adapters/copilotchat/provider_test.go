package copilotchat

import (
	"os"
	"testing"
	"testing/fstest"

	"github.com/danieljbfz/chronicle/contracts"
)

// fixturePair returns the bytes of one fixture file under
// testdata/v3. The little helper exists because most provider
// tests below need to load several fixtures at once.
func fixturePair(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/v3/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// buildFS sets up a fake VS Code install with two workspaces and
// one empty-window chat. One workspace has both a small session
// and an empty session. The other workspace has just the small
// session. That gives us enough material to exercise every
// Provider method in one go.
func buildFS(t *testing.T) fstest.MapFS {
	t.Helper()
	small := fixturePair(t, "small_session.jsonl")
	empty := fixturePair(t, "empty_session.jsonl")
	return fstest.MapFS{
		"workspaceStorage/abc123/workspace.json": &fstest.MapFile{
			Data: []byte(`{"folder":"file:///Users/test/proj-a"}`),
		},
		"workspaceStorage/abc123/chatSessions/small-session-1.jsonl": &fstest.MapFile{Data: small},
		"workspaceStorage/abc123/chatSessions/empty-session-1.jsonl": &fstest.MapFile{Data: empty},
		"workspaceStorage/def456/workspace.json": &fstest.MapFile{
			Data: []byte(`{"folder":"file:///Users/test/proj-b"}`),
		},
		"workspaceStorage/def456/chatSessions/small-session-1-other.jsonl": &fstest.MapFile{Data: small},
		"globalStorage/emptyWindowChatSessions/lonely-session.jsonl":       &fstest.MapFile{Data: small},
	}
}

// TestProvider_ListProjects_combinesWorkspacesAndEmptyBucket
// confirms the listing groups things the way we expect. Two real
// workspaces show up plus the synthetic "(no workspace)" bucket.
func TestProvider_ListProjects_combinesWorkspacesAndEmptyBucket(t *testing.T) {
	p := New()
	projects, err := p.ListProjects(buildFS(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 3 {
		t.Fatalf("got %d projects, want 3", len(projects))
	}
	names := map[string]bool{}
	for _, pr := range projects {
		names[pr.DisplayName] = true
	}
	for _, want := range []string{"proj-a", "proj-b", emptyWindowDisplayName} {
		if !names[want] {
			t.Errorf("missing project %q in %v", want, names)
		}
	}
}

// TestProvider_ListSessionRefs_workspaceProject confirms we can list
// the sessions of a real workspace. The fixture has two sessions
// in workspace abc123, both should come back.
func TestProvider_ListSessionRefs_workspaceProject(t *testing.T) {
	p := New()
	refs, err := p.ListSessionRefs(buildFS(t), "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 {
		t.Fatalf("refs = %d, want 2", len(refs))
	}
}

// TestProvider_ListSessionRefs_emptyWindowBucket confirms the
// synthetic project routes to globalStorage/emptyWindowChatSessions
// instead of into workspaceStorage. One session lives there.
func TestProvider_ListSessionRefs_emptyWindowBucket(t *testing.T) {
	p := New()
	refs, err := p.ListSessionRefs(buildFS(t), emptyWindowProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 {
		t.Fatalf("refs = %d, want 1", len(refs))
	}
	if refs[0].ID != "lonely-session" {
		t.Errorf("session id = %q, want lonely-session", refs[0].ID)
	}
}

// TestProvider_ReadSession_findsSessionAcrossWorkspaces proves the
// linear-scan lookup walks every workspace. The session lives in
// workspace def456, but the caller does not have to know that.
func TestProvider_ReadSession_findsSessionAcrossWorkspaces(t *testing.T) {
	p := New()
	conv, err := p.ReadSession(buildFS(t), "small-session-1-other")
	if err != nil {
		t.Fatal(err)
	}
	if conv.SessionID != "small-session-1" {
		t.Errorf("SessionID = %q", conv.SessionID)
	}
}

// TestProvider_ReadSession_findsEmptyWindowSession proves the
// fallback to the empty-window directory. The session is not under
// workspaceStorage, so the workspace walk has to fail before the
// empty-window path is tried.
func TestProvider_ReadSession_findsEmptyWindowSession(t *testing.T) {
	p := New()
	conv, err := p.ReadSession(buildFS(t), "lonely-session")
	if err != nil {
		t.Fatal(err)
	}
	if conv.Project != emptyWindowProjectID {
		t.Errorf("Project = %q, want %q", conv.Project, emptyWindowProjectID)
	}
}

// TestProvider_implementsCleaner pins the fact that the
// Copilot Chat adapter supports cascade-aware cleanup. If
// anyone ever removes one of the Cleaner methods, the
// type assertion fails and the test catches the
// regression before any destructive code ships.
func TestProvider_implementsCleaner(t *testing.T) {
	var p any = New()
	if _, ok := p.(contracts.Cleaner); !ok {
		t.Error("*Provider should satisfy contracts.Cleaner")
	}
}
