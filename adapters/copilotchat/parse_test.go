package copilotchat

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

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

// TestParse_inputStateSelectedModelLandsOnConversation pins
// the Model wiring on the copilot-chat side. VS Code stores
// the user's per-session model pick at
// inputState.selectedModel.identifier, and parseSnapshot
// surfaces that value on the resulting Conversation so the
// stats renderer can roll it into the by-model breakdown.
func TestParse_inputStateSelectedModelLandsOnConversation(t *testing.T) {
	state := map[string]any{
		"sessionId":       "s1",
		"creationDate":    float64(1700000000000),
		"lastMessageDate": float64(1700000000000),
		"customTitle":     "",
		"inputState": map[string]any{
			"selectedModel": map[string]any{
				"identifier": "copilot/claude-sonnet-4.6",
			},
		},
	}
	conv := parseSnapshot(state, "ws", contracts.StorageVersion{Adapter: "copilot"})
	if conv.Model != "copilot/claude-sonnet-4.6" {
		t.Errorf("Conversation.Model = %q, want copilot/claude-sonnet-4.6", conv.Model)
	}
}

// TestParse_missingSelectedModelLeavesModelEmpty covers the
// fallback case. When the snapshot does not carry the
// nested identifier, the Conversation reports the empty
// string and the stats renderer groups the session under
// "(unknown)".
func TestParse_missingSelectedModelLeavesModelEmpty(t *testing.T) {
	state := map[string]any{
		"sessionId":       "s1",
		"creationDate":    float64(1700000000000),
		"lastMessageDate": float64(1700000000000),
	}
	conv := parseSnapshot(state, "ws", contracts.StorageVersion{Adapter: "copilot"})
	if conv.Model != "" {
		t.Errorf("Conversation.Model = %q, want empty when inputState is missing", conv.Model)
	}
}

// TestParse_endedAtComesFromLatestRequest pins the
// real-shape behaviour current VS Code builds depend on:
// the snapshot has no lastMessageDate field, and each
// request inside the requests array carries its own
// timestamp in Unix milliseconds. The parser walks the
// requests and reports the latest of those as the
// conversation's EndedAt, so the session list's "ago"
// reading reflects when the user actually last interacted
// rather than the zero value of time.Time (which an
// earlier version of the parser propagated and the TUI
// rendered as the days since Go's zero time).
func TestParse_endedAtComesFromLatestRequest(t *testing.T) {
	creation := int64(1700000000000)
	earlier := int64(1700000060000) // creation + 60s
	latest := int64(1700000120000)  // creation + 120s
	state := map[string]any{
		"sessionId":    "s1",
		"creationDate": float64(creation),
		// No lastMessageDate. Real VS Code builds omit it.
		"requests": []any{
			map[string]any{
				"requestId": "r1",
				"timestamp": float64(earlier),
				"message":   map[string]any{"parts": []any{map[string]any{"kind": "text", "text": "hello"}}},
			},
			map[string]any{
				"requestId": "r2",
				"timestamp": float64(latest),
				"message":   map[string]any{"parts": []any{map[string]any{"kind": "text", "text": "world"}}},
			},
		},
	}
	conv := parseSnapshot(state, "ws", contracts.StorageVersion{Adapter: "copilot"})
	wantEnded := time.UnixMilli(latest)
	if !conv.EndedAt.Equal(wantEnded) {
		t.Errorf("EndedAt = %v, want %v (latest request timestamp)", conv.EndedAt, wantEnded)
	}
	wantStarted := time.UnixMilli(creation)
	if !conv.StartedAt.Equal(wantStarted) {
		t.Errorf("StartedAt = %v, want %v (creationDate)", conv.StartedAt, wantStarted)
	}
}

// TestParse_endedAtFallsBackToCreationDate pins the
// fallback path for a session that exists on disk but
// has no requests yet. The snapshot writes creationDate
// the moment the user opens a new chat panel, so the
// session shows up in the listing even before the user
// sends anything; the parser uses creationDate as EndedAt
// in that case so the listing's "ago" reading reflects
// when the session was created rather than the zero value.
func TestParse_endedAtFallsBackToCreationDate(t *testing.T) {
	creation := int64(1700000000000)
	state := map[string]any{
		"sessionId":    "s1",
		"creationDate": float64(creation),
		"requests":     []any{},
	}
	conv := parseSnapshot(state, "ws", contracts.StorageVersion{Adapter: "copilot"})
	want := time.UnixMilli(creation)
	if !conv.EndedAt.Equal(want) {
		t.Errorf("EndedAt = %v, want %v (creationDate fallback)", conv.EndedAt, want)
	}
}
