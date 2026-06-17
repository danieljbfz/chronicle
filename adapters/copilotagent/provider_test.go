package copilotagent

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/danieljbfz/chronicle/contracts"
)

// realSessionUUID is the UUID we stamp on every test
// session. It just needs to be a stable string the tests
// can reference. The agent runtime uses real UUIDs in
// production, but nothing in the parser cares about the
// format.
const realSessionUUID = "11111111-1111-1111-1111-111111111111"

// minimalEvents is the smallest event stream that exercises
// the happy path: session.start, one user message, one
// assistant message, session.shutdown. Bytes are crafted by
// hand rather than fixture-loaded so the test reads
// top-to-bottom without context-switching to a fixture
// file.
const minimalEvents = `{"type":"session.start","data":{"sessionId":"` + realSessionUUID + `","startTime":"2026-04-19T17:37:18.001Z","selectedModel":"claude-sonnet-4.6","context":{"cwd":"/Users/me/proj"}},"timestamp":"2026-04-19T17:37:18.001Z","id":"e1","parentId":""}
{"type":"user.message","data":{"content":"Hello agent"},"timestamp":"2026-04-19T17:37:19.000Z","id":"e2","parentId":"e1"}
{"type":"assistant.turn_start","data":{"turnId":"0"},"timestamp":"2026-04-19T17:37:19.500Z","id":"e3","parentId":"e2"}
{"type":"assistant.message","data":{"messageId":"m1","content":"Hi back","toolRequests":[]},"timestamp":"2026-04-19T17:37:20.000Z","id":"e4","parentId":"e3"}
{"type":"assistant.turn_end","data":{"turnId":"0"},"timestamp":"2026-04-19T17:37:21.000Z","id":"e5","parentId":"e4"}
{"type":"session.shutdown","data":{},"timestamp":"2026-04-19T17:37:22.000Z","id":"e6","parentId":"e5"}
`

// vscodeMetadataJSON is the optional sidecar VS Code
// writes when it launches the agent. The parser uses it
// only for the session title.
const vscodeMetadataJSON = `{
  "workspaceFolder": {"folderPath": "/Users/me/proj", "timestamp": 1776620238016},
  "writtenToDisc": true,
  "customTitle": "Agent test session"
}`

// buildAgentFS produces a one-session fixture filesystem
// the read-path tests share.
func buildAgentFS() fstest.MapFS {
	base := "session-state/" + realSessionUUID
	return fstest.MapFS{
		base + "/events.jsonl":         &fstest.MapFile{Data: []byte(minimalEvents)},
		base + "/vscode.metadata.json": &fstest.MapFile{Data: []byte(vscodeMetadataJSON)},
	}
}

// TestDetect_emptyTreeReturnsUnknown pins the fresh-install
// behaviour. A user with no agent activity yet should get
// a clean "unknown" version, not an error.
func TestDetect_emptyTreeReturnsUnknown(t *testing.T) {
	sv, err := detectInDir(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Adapter != adapterName {
		t.Errorf("Adapter = %q, want %q", sv.Adapter, adapterName)
	}
	if sv.Version != "unknown" {
		t.Errorf("Version = %q, want unknown", sv.Version)
	}
}

// TestDetect_realFixtureReportsCurrentVersion proves the
// happy-path detection. With a session-state directory
// present, Detect reports the current known version.
func TestDetect_realFixtureReportsCurrentVersion(t *testing.T) {
	sv, err := detectInDir(buildAgentFS())
	if err != nil {
		t.Fatal(err)
	}
	if sv.Version != currentVersion {
		t.Errorf("Version = %q, want %q", sv.Version, currentVersion)
	}
}

// TestProvider_ListProjectsReturnsSyntheticBucket pins the
// adapter's choice to expose one synthetic project. The
// agent runtime stores sessions flat, so the listing has
// one Project covering everything.
func TestProvider_ListProjectsReturnsSyntheticBucket(t *testing.T) {
	p := New()
	projects, err := p.ListProjects(buildAgentFS())
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("projects = %d, want 1", len(projects))
	}
	if projects[0].ID != agentProjectID {
		t.Errorf("project ID = %q, want %q", projects[0].ID, agentProjectID)
	}
	if projects[0].SessionCount != 1 {
		t.Errorf("session count = %d, want 1", projects[0].SessionCount)
	}
}

// TestProvider_ListProjectsEmptyTreeReturnsNothing covers
// the fresh-install path. No session-state directory
// means no project, no error.
func TestProvider_ListProjectsEmptyTreeReturnsNothing(t *testing.T) {
	p := New()
	projects, err := p.ListProjects(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 0 {
		t.Errorf("projects = %d, want 0", len(projects))
	}
}

// TestProvider_SummarizeSession_carriesTitleAndTurnCount confirms
// the summary picks up the title from the VS Code metadata sidecar
// and counts the turns from the session's events. Both pieces of
// information are critical for the user to recognise which session
// they are looking at.
func TestProvider_SummarizeSession_carriesTitleAndTurnCount(t *testing.T) {
	p := New()
	fsys := buildAgentFS()
	if _, err := p.Detect(fsys); err != nil {
		t.Fatal(err)
	}
	refs, err := p.ListSessionRefs(fsys, agentProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 {
		t.Fatalf("refs = %d, want 1", len(refs))
	}
	got, err := p.SummarizeSession(fsys, refs[0])
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != realSessionUUID {
		t.Errorf("ID = %q, want %q", got.ID, realSessionUUID)
	}
	if got.Title != "Agent test session" {
		t.Errorf("Title = %q, want the customTitle from vscode.metadata.json", got.Title)
	}
	if got.TurnCount != 2 {
		t.Errorf("TurnCount = %d, want 2 (one user, one assistant)", got.TurnCount)
	}
}

// TestProvider_ReadSessionParsesContent confirms the full
// read path: open the events stream, fold each event into
// the right shape, return a Conversation with both messages
// rendered correctly.
func TestProvider_ReadSessionParsesContent(t *testing.T) {
	p := New()
	if _, err := p.Detect(buildAgentFS()); err != nil {
		t.Fatal(err)
	}
	conv, err := p.ReadSession(buildAgentFS(), realSessionUUID)
	if err != nil {
		t.Fatal(err)
	}
	if conv.SessionID != realSessionUUID {
		t.Errorf("SessionID = %q, want %q", conv.SessionID, realSessionUUID)
	}
	if conv.Cwd != "/Users/me/proj" {
		t.Errorf("Cwd = %q, want the cwd from session.start", conv.Cwd)
	}
	if len(conv.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(conv.Messages))
	}
	if conv.Messages[0].Role != contracts.RoleUser {
		t.Errorf("first role = %q, want user", conv.Messages[0].Role)
	}
	if conv.Messages[1].Role != contracts.RoleAssistant {
		t.Errorf("second role = %q, want assistant", conv.Messages[1].Role)
	}
}

// TestProvider_ReadSessionUnknownIDReturnsErrNotExist pins
// the not-found contract so chronicle can use errors.Is
// to recognise "no such session" without parsing strings.
func TestProvider_ReadSessionUnknownIDReturnsErrNotExist(t *testing.T) {
	p := New()
	_, err := p.ReadSession(buildAgentFS(), "no-such-session")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want one wrapping fs.ErrNotExist", err)
	}
}

// TestParse_unknownEventTypeBecomesUnknownBlock pins the
// resilience contract. A future agent runtime that adds a
// new event type must not lose data. Chronicle wraps the
// raw line in an UnknownBlock and surfaces it through the
// renderer.
func TestParse_unknownEventTypeBecomesUnknownBlock(t *testing.T) {
	body := minimalEvents +
		`{"type":"future.kind","data":{"hello":"world"},"timestamp":"2026-04-19T17:38:00.000Z","id":"f1","parentId":"e6"}` + "\n"
	fsys := fstest.MapFS{
		"session-state/" + realSessionUUID + "/events.jsonl": &fstest.MapFile{Data: []byte(body)},
	}
	p := New()
	if _, err := p.Detect(fsys); err != nil {
		t.Fatal(err)
	}
	conv, err := p.ReadSession(fsys, realSessionUUID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range conv.Messages {
		for _, b := range m.Blocks {
			if u, ok := b.(contracts.UnknownBlock); ok && u.Kind == "future.kind" {
				found = true
				if !strings.Contains(string(u.Raw), "future.kind") {
					t.Errorf("UnknownBlock raw payload missing the type marker: %q", u.Raw)
				}
			}
		}
	}
	if !found {
		t.Error("expected an UnknownBlock for the future.kind event")
	}
}

// TestParse_assistantMessageProducesToolUseBlocks
// confirms tool requests inside an assistant.message land
// as ToolUseBlock values. The agent inlines tool requests
// rather than emitting separate request events, so the
// parser has to lift them out.
func TestParse_assistantMessageProducesToolUseBlocks(t *testing.T) {
	body := `{"type":"session.start","data":{"sessionId":"` + realSessionUUID + `","startTime":"2026-04-19T17:37:18.001Z","context":{"cwd":"/p"}},"timestamp":"2026-04-19T17:37:18.001Z","id":"e1"}
{"type":"assistant.message","data":{"messageId":"m1","content":"Looking up","toolRequests":[{"toolCallId":"call-1","name":"view","arguments":{"path":"/p/file.txt"}}]},"timestamp":"2026-04-19T17:37:20.000Z","id":"e2","parentId":"e1"}
`
	fsys := fstest.MapFS{
		"session-state/" + realSessionUUID + "/events.jsonl": &fstest.MapFile{Data: []byte(body)},
	}
	p := New()
	if _, err := p.Detect(fsys); err != nil {
		t.Fatal(err)
	}
	conv, err := p.ReadSession(fsys, realSessionUUID)
	if err != nil {
		t.Fatal(err)
	}
	if len(conv.Messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(conv.Messages))
	}
	blocks := conv.Messages[0].Blocks
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2 (text + tool use)", len(blocks))
	}
	tu, ok := blocks[1].(contracts.ToolUseBlock)
	if !ok {
		t.Fatalf("second block = %T, want ToolUseBlock", blocks[1])
	}
	if tu.Tool != "view" {
		t.Errorf("tool name = %q, want view", tu.Tool)
	}
	if tu.CallID != "call-1" {
		t.Errorf("call id = %q, want call-1", tu.CallID)
	}
}

// TestParse_toolStartCompletePairProducesResultBlock pins
// the start/complete join. The parser correlates a
// tool.execution_start with the matching
// tool.execution_complete and emits a single
// ToolResultBlock.
//
// The result payload uses the real shape the runtime writes —
// an object with content and detailedContent — so the test
// also pins the flatten: the block's Output must be exactly the
// content text, not the raw JSON and not the detailedContent.
func TestParse_toolStartCompletePairProducesResultBlock(t *testing.T) {
	body := `{"type":"session.start","data":{"sessionId":"` + realSessionUUID + `","startTime":"2026-04-19T17:37:18.001Z","context":{"cwd":"/p"}},"timestamp":"2026-04-19T17:37:18.001Z","id":"e1"}
{"type":"tool.execution_start","data":{"toolCallId":"call-1","toolName":"view","arguments":{"path":"/p/file.txt"}},"timestamp":"2026-04-19T17:37:20.000Z","id":"e2"}
{"type":"tool.execution_complete","data":{"toolCallId":"call-1","success":true,"result":{"content":"file body","detailedContent":"diff --git a/file.txt"}},"timestamp":"2026-04-19T17:37:20.500Z","id":"e3"}
`
	fsys := fstest.MapFS{
		"session-state/" + realSessionUUID + "/events.jsonl": &fstest.MapFile{Data: []byte(body)},
	}
	p := New()
	if _, err := p.Detect(fsys); err != nil {
		t.Fatal(err)
	}
	conv, err := p.ReadSession(fsys, realSessionUUID)
	if err != nil {
		t.Fatal(err)
	}
	var found *contracts.ToolResultBlock
	for _, m := range conv.Messages {
		for _, b := range m.Blocks {
			if r, ok := b.(contracts.ToolResultBlock); ok {
				rcopy := r
				found = &rcopy
			}
		}
	}
	if found == nil {
		t.Fatal("expected a ToolResultBlock from the start/complete pair")
	}
	if found.CallID != "call-1" {
		t.Errorf("call id = %q, want call-1", found.CallID)
	}
	if found.IsError {
		t.Errorf("IsError = true, want false (the complete event reported success)")
	}
	if found.Output != "file body" {
		t.Errorf("Output = %q, want exactly the flattened content (not the raw JSON or detailedContent)", found.Output)
	}
}

// TestParse_sessionStartCarriesSelectedModel pins the
// Model wiring on the agent side. The runtime records the
// model once per session in session.start.selectedModel,
// and parseEventStream surfaces that value on the resulting
// Conversation so the stats renderer can roll it into the
// by-model breakdown. SummarizeSession then carries the same
// value onto the SessionSummary without paying the read
// cost twice.
func TestParse_sessionStartCarriesSelectedModel(t *testing.T) {
	p := New()
	fsys := buildAgentFS()
	if _, err := p.Detect(fsys); err != nil {
		t.Fatal(err)
	}
	conv, err := p.ReadSession(fsys, realSessionUUID)
	if err != nil {
		t.Fatalf("ReadSession: %v", err)
	}
	if conv.Model != "claude-sonnet-4.6" {
		t.Errorf("Conversation.Model = %q, want claude-sonnet-4.6", conv.Model)
	}

	refs, err := p.ListSessionRefs(fsys, agentProjectID)
	if err != nil {
		t.Fatalf("ListSessionRefs: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("got %d refs, want 1", len(refs))
	}
	summary, err := p.SummarizeSession(fsys, refs[0])
	if err != nil {
		t.Fatalf("SummarizeSession: %v", err)
	}
	if summary.Model != "claude-sonnet-4.6" {
		t.Errorf("SessionSummary.Model = %q, want claude-sonnet-4.6", summary.Model)
	}
}

// TestParse_missingSelectedModelLeavesModelEmpty covers the
// fallback case. When the agent did not record a model on
// session.start, the Conversation reports the empty string
// and the stats renderer groups those sessions under
// "(unknown)".
func TestParse_missingSelectedModelLeavesModelEmpty(t *testing.T) {
	const noModelEvents = `{"type":"session.start","data":{"sessionId":"abc","startTime":"2026-04-19T17:37:18.001Z","context":{"cwd":"/x"}},"timestamp":"2026-04-19T17:37:18.001Z","id":"e1","parentId":""}
`
	fsys := fstest.MapFS{
		"session-state/abc/events.jsonl": &fstest.MapFile{Data: []byte(noModelEvents)},
	}
	p := New()
	if _, err := p.Detect(fsys); err != nil {
		t.Fatal(err)
	}
	conv, err := p.ReadSession(fsys, "abc")
	if err != nil {
		t.Fatalf("ReadSession: %v", err)
	}
	if conv.Model != "" {
		t.Errorf("Conversation.Model = %q, want empty when selectedModel is missing", conv.Model)
	}
}
