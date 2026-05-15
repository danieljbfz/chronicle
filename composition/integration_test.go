package composition

import (
	"os"
	"path/filepath"
	"testing"
)

// TestNew_integrationAgainstFakeHome exercises composition.New on
// a real (temporary) home directory. The other tests in this
// package use NewForTest with fake providers, but that path skips
// the actual wiring code: config loading, registry walking, and
// Detect calls on real adapters. This test exercises that full
// path against a hand-built fake home, so the production code in
// New() is no longer untested.
//
// We point CHRONICLE_HOME at a fresh temp directory, drop one
// real-shape Claude session into it, and assert that the
// resulting App can list and read it. If anyone breaks the wiring
// in New (forgets to call Detect, misroutes the config, drops a
// provider on the floor) the test fails right here.
func TestNew_integrationAgainstFakeHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CHRONICLE_HOME", home)

	// Drop a real Claude session into the fake home. The fixture
	// is the same one the claude package uses in its own tests,
	// so we know the shape is valid.
	claudeProject := filepath.Join(home, ".claude", "projects", "-tmp-fakeproject")
	if err := os.MkdirAll(claudeProject, 0o755); err != nil {
		t.Fatal(err)
	}
	session := filepath.Join(claudeProject, "fake-session-1.jsonl")
	body := `{"parentUuid":null,"isSidechain":false,"promptId":"p1","type":"user","message":{"role":"user","content":"hello"},"uuid":"u1","timestamp":"2026-05-15T09:00:00.000Z","sessionId":"fake-session-1","cwd":"/tmp/fake","version":"2.1.0","gitBranch":"main"}
{"parentUuid":"u1","isSidechain":false,"type":"assistant","message":{"id":"m1","role":"assistant","content":[{"type":"text","text":"hi"}]},"uuid":"u2","timestamp":"2026-05-15T09:00:01.000Z","sessionId":"fake-session-1","cwd":"/tmp/fake","version":"2.1.0","gitBranch":"main"}
`
	if err := os.WriteFile(session, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	app, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// The fake home has no Copilot directory, so only Claude
	// should come out of the registry walk.
	if len(app.providers) < 1 {
		t.Fatalf("got %d providers, want at least 1", len(app.providers))
	}

	// ListSessionsAll should return our one session. Going through
	// the full path proves the registry, Detect, ListProjects, and
	// ListSessions all wire together correctly.
	listings, err := app.ListSessionsAll("claude")
	if err != nil {
		t.Fatalf("ListSessionsAll: %v", err)
	}
	if len(listings) != 1 {
		t.Fatalf("listings = %d, want 1", len(listings))
	}
	if listings[0].Summary.ID != "fake-session-1" {
		t.Errorf("session id = %q, want fake-session-1", listings[0].Summary.ID)
	}

	// ReadSession should round-trip the same session by id.
	conv, err := app.ReadSession("fake-session-1")
	if err != nil {
		t.Fatalf("ReadSession: %v", err)
	}
	if conv.SessionID != "fake-session-1" {
		t.Errorf("SessionID = %q", conv.SessionID)
	}
	if conv.FirstUserPrompt() != "hello" {
		t.Errorf("first user prompt = %q, want hello", conv.FirstUserPrompt())
	}
}

// TestDoctor_integrationAgainstEmptyHome confirms the doctor view
// works against a fake home with no provider data at all. The
// providers should detect, report Reachable=true, and show zero
// sessions. This is the path a brand-new chronicle install would
// take on a developer's machine where neither tool has been used
// yet.
func TestDoctor_integrationAgainstEmptyHome(t *testing.T) {
	t.Setenv("CHRONICLE_HOME", t.TempDir())

	app, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	healths := app.Doctor()
	if len(healths) < 1 {
		t.Fatalf("got %d health entries, want at least 1", len(healths))
	}
	for _, h := range healths {
		if h.SessionCount != 0 {
			t.Errorf("%s session count = %d, want 0 on empty home", h.Name, h.SessionCount)
		}
	}
}
