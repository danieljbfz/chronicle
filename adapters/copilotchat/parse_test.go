package copilotchat

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/danieljbfz/chronicle/contracts"
)

// readSession is the test helper. It loads a fixture, drops it
// into a fake filesystem at a path the parser expects, and runs
// readSessionFile against it. The test gets back a fully parsed
// Conversation without ever touching the real disk.
func readSession(t *testing.T, fixture string, project contracts.ProjectID) contracts.Conversation {
	t.Helper()
	data, err := os.ReadFile("testdata/v3/" + fixture)
	if err != nil {
		t.Fatalf("read %s: %v", fixture, err)
	}
	fsys := fstest.MapFS{
		"workspaceStorage/abc/chatSessions/s.jsonl": &fstest.MapFile{Data: data},
	}
	conv, err := readSessionFile(fsys, "workspaceStorage/abc/chatSessions/s.jsonl", project, contracts.StorageVersion{Adapter: "copilot"})
	if err != nil {
		t.Fatalf("readSessionFile: %v", err)
	}
	return conv
}

// TestParse_smallSessionShape walks the small fixture and confirms
// the parser produced the expected Conversation shape. The fixture
// has one user message ("How do I read a file in Go?") and one
// assistant reply that contains both a markdown text block and a
// tool invocation.
func TestParse_smallSessionShape(t *testing.T) {
	conv := readSession(t, "small_session.jsonl", "abc")
	if conv.SessionID != "small-session-1" {
		t.Errorf("SessionID = %q", conv.SessionID)
	}
	// One user message, one assistant message.
	if len(conv.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(conv.Messages))
	}
	if conv.Messages[0].Role != contracts.RoleUser {
		t.Errorf("first message role = %q, want user", conv.Messages[0].Role)
	}
	if conv.Messages[1].Role != contracts.RoleAssistant {
		t.Errorf("second message role = %q, want assistant", conv.Messages[1].Role)
	}

	// User text should round-trip the original prompt.
	userText, ok := conv.Messages[0].Blocks[0].(contracts.TextBlock)
	if !ok || !strings.Contains(userText.Text, "How do I read a file") {
		t.Errorf("user prompt missing or wrong shape: %#v", conv.Messages[0].Blocks[0])
	}

	// Assistant should have at least a TextBlock and a ToolUseBlock.
	var sawText, sawTool bool
	for _, b := range conv.Messages[1].Blocks {
		switch b.(type) {
		case contracts.TextBlock:
			sawText = true
		case contracts.ToolUseBlock:
			sawTool = true
		}
	}
	if !sawText {
		t.Error("assistant should have a TextBlock from the markdown response")
	}
	if !sawTool {
		t.Error("assistant should have a ToolUseBlock from the tool invocation")
	}
}

// TestParse_emptySessionIsAbandoned proves the empty fixture comes
// back as abandoned. A session with zero requests in the snapshot
// has nothing the user typed, which is the chronicle definition of
// abandoned.
func TestParse_emptySessionIsAbandoned(t *testing.T) {
	conv := readSession(t, "empty_session.jsonl", "abc")
	if !conv.IsAbandoned() {
		t.Error("empty fixture should be abandoned")
	}
}

// TestParse_syntheticFutureKeepsUnknowns is the canary for the
// resilience contract. The synthetic-future fixture has an unknown
// event kind in the middle of the event log AND an unknown
// response part kind inside the request that does survive. Both
// have to come through alive: the event-log replay must keep going,
// and the unknown response part must turn into an UnknownBlock so
// the renderer can show it.
func TestParse_syntheticFutureKeepsUnknowns(t *testing.T) {
	data, err := os.ReadFile("testdata/synthetic_future.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	fsys := fstest.MapFS{
		"workspaceStorage/abc/chatSessions/s.jsonl": &fstest.MapFile{Data: data},
	}
	conv, err := readSessionFile(fsys, "workspaceStorage/abc/chatSessions/s.jsonl", "abc", contracts.StorageVersion{Adapter: "copilot", Version: "unknown"})
	if err != nil {
		t.Fatalf("parse must not error on synthetic future: %v", err)
	}
	if conv.Title != "A title from the future" {
		t.Errorf("title = %q, want a title from the future event", conv.Title)
	}
	var sawUnknownContent bool
	for _, m := range conv.Messages {
		for _, b := range m.Blocks {
			if u, ok := b.(contracts.UnknownBlock); ok && u.Kind == "galaxy_brain" {
				sawUnknownContent = true
			}
		}
	}
	if !sawUnknownContent {
		t.Error("unknown response part must surface as an UnknownBlock")
	}
}
