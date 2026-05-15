package claude

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/danieljbfz/chronicle/contracts"
)

// readSession is a small helper that loads a fixture file, places it
// inside an in-memory MapFS at the path the parser expects, and runs
// readSessionFile against it. The test is therefore exercising the
// real production code path from "open this file" through
// "produce a Conversation," but without touching the real
// filesystem at all.
func readSession(t *testing.T, fixture string) contracts.Conversation {
	t.Helper()
	data, err := os.ReadFile("testdata/v1_0/" + fixture)
	if err != nil {
		t.Fatalf("read %s: %v", fixture, err)
	}
	fsys := fstest.MapFS{
		"projects/-p/s.jsonl": &fstest.MapFile{Data: data},
	}
	c, err := readSessionFile(fsys, "projects/-p/s.jsonl", contracts.StorageVersion{Adapter: "claude"})
	if err != nil {
		t.Fatalf("readSessionFile: %v", err)
	}
	return c
}

// TestParse_smallSessionShape walks the small fixture and confirms
// the parser produced the right kinds of blocks in the right places.
// The fixture has four conversation turns: a user prompt, an
// assistant turn that contains both text and a tool_use, a user
// turn that carries a tool_result, and an assistant text reply. We
// look for both the ToolUseBlock and the ToolResultBlock to confirm
// the tool round-trip survived the parsing pass intact.
func TestParse_smallSessionShape(t *testing.T) {
	c := readSession(t, "small_session.jsonl")
	if c.SessionID != "small-session-1" {
		t.Errorf("SessionID = %q, want %q", c.SessionID, "small-session-1")
	}
	if c.Messages[0].Role != contracts.RoleUser {
		t.Errorf("first message role = %q, want user", c.Messages[0].Role)
	}
	foundToolUse := false
	foundToolResult := false
	for _, m := range c.Messages {
		for _, b := range m.Blocks {
			if _, ok := b.(contracts.ToolUseBlock); ok {
				foundToolUse = true
			}
			if _, ok := b.(contracts.ToolResultBlock); ok {
				foundToolResult = true
			}
		}
	}
	if !foundToolUse {
		t.Error("expected a ToolUseBlock in the small fixture")
	}
	if !foundToolResult {
		t.Error("expected a ToolResultBlock in the small fixture")
	}
}

// TestParse_emptySessionIsAbandoned uses the predicate side of the
// Conversation type to confirm the empty fixture really does count
// as abandoned. The fixture contains only synthetic meta records
// (the session-start hook, a /clear command echo) and no real user
// prompts. This is the canonical shape of the sessions the cleanup
// command will surface for one-key removal.
func TestParse_emptySessionIsAbandoned(t *testing.T) {
	c := readSession(t, "empty_session.jsonl")
	if !c.IsAbandoned() {
		t.Error("empty fixture should be abandoned")
	}
	if c.FirstUserPrompt() != "" {
		t.Errorf("FirstUserPrompt = %q, want empty", c.FirstUserPrompt())
	}
}

// TestParse_projectIsEncodedFolderAndCwdIsRawPath pins the new
// contract for the two project-shaped fields on Conversation.
// Project must be the encoded folder name the file lived under,
// matching what SessionSummary already returns. Cwd must be the
// raw working-directory string Claude wrote into the JSONL
// records. Keeping these distinct removes the latent bug where
// the same field carried different values depending on whether
// the caller arrived through ListSessions or through
// ReadSession.
func TestParse_projectIsEncodedFolderAndCwdIsRawPath(t *testing.T) {
	c := readSession(t, "small_session.jsonl")
	if c.Project != "-p" {
		t.Errorf("Project = %q, want %q (encoded folder name from the path)", c.Project, "-p")
	}
	if c.Cwd != "/Users/test/proj" {
		t.Errorf("Cwd = %q, want %q (raw cwd from the JSONL records)", c.Cwd, "/Users/test/proj")
	}
}

// TestParse_thinkingBlockSurvives proves the parser holds on to the
// assistant's internal reasoning instead of discarding it. Hiding
// the thinking block at render time is a UI choice. Dropping it at
// parse time would break the resilience contract, because we would
// be losing content the upstream tool wrote.
func TestParse_thinkingBlockSurvives(t *testing.T) {
	c := readSession(t, "thinking_session.jsonl")
	found := false
	for _, m := range c.Messages {
		for _, b := range m.Blocks {
			if tb, ok := b.(contracts.ThinkingBlock); ok && strings.Contains(tb.Text, "refactor") {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected ThinkingBlock with the fixture content")
	}
}

// TestParse_syntheticFutureKeepsUnknowns is the canary test for the
// resilience contract. The fixture contains a fabricated record type
// that no version of chronicle has ever seen ("future-event-from-tomorrow")
// and a fabricated assistant content kind ("galaxy_brain"). The
// parser must keep both as UnknownBlock entries, neither as errors
// nor as silent drops. If anyone ever changes the parser in a way
// that loses unknowns, this test fails immediately and loud — which
// is exactly the safety net the contract requires.
func TestParse_syntheticFutureKeepsUnknowns(t *testing.T) {
	data, err := os.ReadFile("testdata/synthetic_future.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	fsys := fstest.MapFS{"projects/-p/s.jsonl": &fstest.MapFile{Data: data}}
	c, err := readSessionFile(fsys, "projects/-p/s.jsonl", contracts.StorageVersion{Adapter: "claude", Version: "unknown"})
	if err != nil {
		t.Fatalf("parse must not error on synthetic future: %v", err)
	}
	var sawUnknownRecord, sawUnknownContent bool
	for _, m := range c.Messages {
		for _, b := range m.Blocks {
			if u, ok := b.(contracts.UnknownBlock); ok {
				if u.Kind == "future-event-from-tomorrow" {
					sawUnknownRecord = true
				}
				if u.Kind == "galaxy_brain" {
					sawUnknownContent = true
				}
			}
		}
	}
	if !sawUnknownRecord {
		t.Error("unknown record type must surface as UnknownBlock — the resilience canary")
	}
	if !sawUnknownContent {
		t.Error("unknown content kind must surface as UnknownBlock — the resilience canary")
	}
}

// TestMostFrequentModel_picksTheValueWithTheHighestCount confirms
// the simple majority case. Three assistant messages on model A and
// one on model B mean A is the session-level summary.
func TestMostFrequentModel_picksTheValueWithTheHighestCount(t *testing.T) {
	messages := []contracts.Message{
		{Role: contracts.RoleAssistant, Model: "sonnet"},
		{Role: contracts.RoleAssistant, Model: "sonnet"},
		{Role: contracts.RoleAssistant, Model: "sonnet"},
		{Role: contracts.RoleAssistant, Model: "opus"},
	}
	if got := mostFrequentModel(messages); got != "sonnet" {
		t.Errorf("model = %q, want sonnet", got)
	}
}

// TestMostFrequentModel_breaksTiesByFirstAppearance pins the
// tie-breaking rule. When two models are tied on count, the model
// that appeared first in the conversation wins, which gives a
// deterministic answer the user can predict.
func TestMostFrequentModel_breaksTiesByFirstAppearance(t *testing.T) {
	messages := []contracts.Message{
		{Role: contracts.RoleAssistant, Model: "opus"},
		{Role: contracts.RoleAssistant, Model: "sonnet"},
		{Role: contracts.RoleAssistant, Model: "opus"},
		{Role: contracts.RoleAssistant, Model: "sonnet"},
	}
	if got := mostFrequentModel(messages); got != "opus" {
		t.Errorf("model = %q, want opus (it appeared first)", got)
	}
}

// TestMostFrequentModel_returnsEmptyWhenNoModelsRecorded covers the
// shape we feed back to the stats renderer. A session with no
// assistant messages, or with assistant messages whose Model field
// is empty, should produce the empty string so the renderer can
// group it under "(unknown)".
func TestMostFrequentModel_returnsEmptyWhenNoModelsRecorded(t *testing.T) {
	if got := mostFrequentModel(nil); got != "" {
		t.Errorf("model = %q, want empty for nil messages", got)
	}
	messages := []contracts.Message{
		{Role: contracts.RoleUser, Model: ""},
		{Role: contracts.RoleAssistant, Model: ""},
	}
	if got := mostFrequentModel(messages); got != "" {
		t.Errorf("model = %q, want empty for messages without models", got)
	}
}

// TestParse_slashCommandUserRecordIsTreatedAsMeta pins the
// behavior that lets the listing view show a useful title
// for sessions that begin with /clear or /compact. Claude
// writes those records as ordinary user messages without
// isMeta=true, and FirstUserPrompt would otherwise pick the
// "<command-name>" markup as the session's title. The
// parser marks them IsMeta itself so the title fallback
// looks past them to the next real prompt.
func TestParse_slashCommandUserRecordIsTreatedAsMeta(t *testing.T) {
	jsonl := []byte(`{"type":"user","uuid":"u1","timestamp":"2026-05-15T10:00:00Z","message":{"role":"user","content":"<command-name>/clear</command-name>\n<command-message>clear</command-message>\n<command-args></command-args>"}}
{"type":"user","uuid":"u2","timestamp":"2026-05-15T10:00:01Z","message":{"role":"user","content":"actual question from the user"}}
{"type":"assistant","uuid":"a1","timestamp":"2026-05-15T10:00:02Z","message":{"role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"text","text":"reply"}]}}
`)
	fsys := fstest.MapFS{"projects/-p/s.jsonl": &fstest.MapFile{Data: jsonl}}
	c, err := readSessionFile(fsys, "projects/-p/s.jsonl",
		contracts.StorageVersion{Adapter: "claude", Version: "claude-1.0"})
	if err != nil {
		t.Fatalf("readSessionFile: %v", err)
	}
	if len(c.Messages) < 1 {
		t.Fatalf("expected at least one user message, got %d", len(c.Messages))
	}
	if !c.Messages[0].IsMeta {
		t.Error("the /clear record must be marked IsMeta so the title fallback skips it")
	}
	if got := c.FirstUserPrompt(); got != "actual question from the user" {
		t.Errorf("FirstUserPrompt = %q, want the next real user message", got)
	}
}

// TestParse_slashCommandOnlySessionIsAbandoned covers the
// edge case where a user opens a session, runs only slash
// commands, and never asks a real question. With the
// detection in place, FirstUserPrompt returns the empty
// string and IsAbandoned reports true so the cleanup
// commands can offer to prune the session.
func TestParse_slashCommandOnlySessionIsAbandoned(t *testing.T) {
	jsonl := []byte(`{"type":"user","uuid":"u1","timestamp":"2026-05-15T10:00:00Z","message":{"role":"user","content":"<command-name>/clear</command-name>"}}
`)
	fsys := fstest.MapFS{"projects/-p/s.jsonl": &fstest.MapFile{Data: jsonl}}
	c, err := readSessionFile(fsys, "projects/-p/s.jsonl",
		contracts.StorageVersion{Adapter: "claude", Version: "claude-1.0"})
	if err != nil {
		t.Fatalf("readSessionFile: %v", err)
	}
	if !c.IsAbandoned() {
		t.Error("a session whose only user input is a slash command must read as abandoned")
	}
	if got := c.FirstUserPrompt(); got != "" {
		t.Errorf("FirstUserPrompt = %q, want empty for a slash-command-only session", got)
	}
}

// TestParse_assistantRecordCarriesModelOntoMessage confirms
// the per-message Model wiring the by-model summary depends
// on. Claude records the model identifier on every
// assistant record, the parser copies it through, and the
// session-level summary is then the most-frequent value.
func TestParse_assistantRecordCarriesModelOntoMessage(t *testing.T) {
	jsonl := []byte(`{"type":"user","uuid":"u1","timestamp":"2026-05-15T10:00:00Z","message":{"role":"user","content":"hi"}}
{"type":"assistant","uuid":"a1","timestamp":"2026-05-15T10:00:01Z","message":{"role":"assistant","model":"claude-opus-4-7","content":[{"type":"text","text":"reply"}]}}
{"type":"assistant","uuid":"a2","timestamp":"2026-05-15T10:00:02Z","message":{"role":"assistant","model":"claude-opus-4-7","content":[{"type":"text","text":"again"}]}}
{"type":"assistant","uuid":"a3","timestamp":"2026-05-15T10:00:03Z","message":{"role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"text","text":"once"}]}}
`)
	fsys := fstest.MapFS{"projects/-p/s.jsonl": &fstest.MapFile{Data: jsonl}}
	c, err := readSessionFile(fsys, "projects/-p/s.jsonl",
		contracts.StorageVersion{Adapter: "claude", Version: "claude-1.0"})
	if err != nil {
		t.Fatalf("readSessionFile: %v", err)
	}
	if c.Model != "claude-opus-4-7" {
		t.Errorf("Conversation.Model = %q, want claude-opus-4-7 (the most frequent)", c.Model)
	}
	var assistantModels []string
	for _, m := range c.Messages {
		if m.Role == contracts.RoleAssistant {
			assistantModels = append(assistantModels, m.Model)
		}
	}
	want := []string{"claude-opus-4-7", "claude-opus-4-7", "claude-sonnet-4-6"}
	if len(assistantModels) != len(want) {
		t.Fatalf("got %d assistant messages, want %d", len(assistantModels), len(want))
	}
	for i, w := range want {
		if assistantModels[i] != w {
			t.Errorf("assistant[%d].Model = %q, want %q", i, assistantModels[i], w)
		}
	}
}
