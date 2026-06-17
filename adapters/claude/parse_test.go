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

// TestDecodePart_thinkingShapes pins the three shapes a thinking
// part can take on disk. Claude writes the reasoning as readable
// text in most blocks, as an empty text plus an encrypted signature
// when extended thinking runs with its display omitted, and — in
// principle — as a wholly empty block. The first two are kept (the
// second flagged Encrypted so the renderer marks it), and the third
// is dropped because it carries no reasoning to show.
func TestDecodePart_thinkingShapes(t *testing.T) {
	t.Run("readable text is kept and not flagged encrypted", func(t *testing.T) {
		block, ok := decodePart([]byte(`{"type":"thinking","thinking":"I will refactor","signature":"sig"}`))
		if !ok {
			t.Fatal("a thinking block with readable text should be kept")
		}
		tb, isThinking := block.(contracts.ThinkingBlock)
		if !isThinking {
			t.Fatalf("block = %T, want ThinkingBlock", block)
		}
		if tb.Text != "I will refactor" {
			t.Errorf("Text = %q, want the reasoning text", tb.Text)
		}
		if tb.Encrypted {
			t.Error("a block with readable text should not be flagged encrypted")
		}
	})

	t.Run("empty text with a signature is kept and flagged encrypted", func(t *testing.T) {
		block, ok := decodePart([]byte(`{"type":"thinking","thinking":"","signature":"Eo0JCmMIDRgCKkA"}`))
		if !ok {
			t.Fatal("an encrypted thinking block should be kept, not dropped")
		}
		tb, isThinking := block.(contracts.ThinkingBlock)
		if !isThinking {
			t.Fatalf("block = %T, want ThinkingBlock", block)
		}
		if tb.Text != "" {
			t.Errorf("Text = %q, want empty for an encrypted block", tb.Text)
		}
		if !tb.Encrypted {
			t.Error("empty text with a signature should be flagged encrypted")
		}
	})

	t.Run("empty text and no signature is dropped", func(t *testing.T) {
		if _, ok := decodePart([]byte(`{"type":"thinking","thinking":"","signature":""}`)); ok {
			t.Error("a thinking block with no text and no signature should be dropped")
		}
	})
}

// TestParse_blocklessMessageIsDropped proves a user or assistant
// record whose content decodes to no blocks is left out of the
// conversation rather than rendered as a bare role heading. The
// first user record carries an empty content array, the assistant
// record carries only a wholly empty thinking part the decoder
// drops, and only the real user turn survives.
func TestParse_blocklessMessageIsDropped(t *testing.T) {
	jsonl := []byte(
		`{"type":"user","uuid":"u1","timestamp":"2026-05-15T10:00:00Z","message":{"role":"user","content":[]}}` + "\n" +
			`{"type":"assistant","uuid":"a1","timestamp":"2026-05-15T10:00:01Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"","signature":""}]}}` + "\n" +
			`{"type":"user","uuid":"u2","timestamp":"2026-05-15T10:00:02Z","message":{"role":"user","content":[{"type":"text","text":"real prompt"}]}}` + "\n")

	c := parse(t, jsonl)

	if len(c.Messages) != 1 {
		t.Fatalf("messages = %d, want 1 (only the real user turn survives)", len(c.Messages))
	}
	if c.Messages[0].Role != contracts.RoleUser {
		t.Errorf("surviving message role = %q, want user", c.Messages[0].Role)
	}
	tb, ok := c.Messages[0].Blocks[0].(contracts.TextBlock)
	if !ok || tb.Text != "real prompt" {
		t.Errorf("surviving block = %+v, want the real prompt text", c.Messages[0].Blocks)
	}
}

// TestFlattenToolResultContent_shapes pins how a tool result's
// content field collapses to one string. Claude writes it as a bare
// string for simple tools and as an array of typed parts for others.
// Text parts contribute their text; image and tool-reference parts
// contribute a short bracketed marker so a result with no text part
// still shows what came back rather than flattening to nothing.
func TestFlattenToolResultContent_shapes(t *testing.T) {
	t.Run("bare string passes through", func(t *testing.T) {
		if got := flattenToolResultContent([]byte(`"plain output"`)); got != "plain output" {
			t.Errorf("got %q, want %q", got, "plain output")
		}
	})

	t.Run("text parts concatenate", func(t *testing.T) {
		got := flattenToolResultContent([]byte(`[{"type":"text","text":"line one\n"},{"type":"text","text":"line two"}]`))
		if got != "line one\nline two" {
			t.Errorf("got %q, want the two text parts concatenated", got)
		}
	})

	t.Run("an image-only result surfaces a reference, not an empty string", func(t *testing.T) {
		got := flattenToolResultContent([]byte(`[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAAA"}}]`))
		if got == "" {
			t.Fatal("an image result must not flatten to an empty string")
		}
		if !strings.Contains(got, "image/png") {
			t.Errorf("got %q, want a reference naming the image media type", got)
		}
	})

	t.Run("a tool-reference part surfaces the referenced tool name", func(t *testing.T) {
		got := flattenToolResultContent([]byte(`[{"type":"tool_reference","tool_name":"TaskCreate"}]`))
		if !strings.Contains(got, "TaskCreate") {
			t.Errorf("got %q, want the referenced tool name", got)
		}
	})

	t.Run("text and a non-text part stay on separate lines", func(t *testing.T) {
		got := flattenToolResultContent([]byte(`[{"type":"text","text":"output"},{"type":"image","source":{"media_type":"image/png","data":"AA"}}]`))
		if !strings.Contains(got, "output\n[image:") {
			t.Errorf("got %q, want the image marker on its own line after the text", got)
		}
	})
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

// blocksOfType collects every block of a given concrete type across
// a conversation, so the rescue tests below can assert on the
// content chronicle surfaced without caring which message it landed
// on.
func blocksOfType[T contracts.Block](c contracts.Conversation) []T {
	var out []T
	for _, m := range c.Messages {
		for _, b := range m.Blocks {
			if v, ok := b.(T); ok {
				out = append(out, v)
			}
		}
	}
	return out
}

// TestParse_queueOperationsAreDropped proves the queue-operation
// records (enqueue, dequeue, popAll, remove) are dropped. They carry
// no uuid and track the queue as the user edits it. The sent prompt
// reaches the transcript through its queued_command attachment
// instead, which the next test covers. The fixture has only queue
// bookkeeping and must produce no messages and no UnknownBlock.
func TestParse_queueOperationsAreDropped(t *testing.T) {
	jsonl := []byte(`{"type":"queue-operation","operation":"enqueue","content":"draft a prompt","sessionId":"s1","timestamp":"2026-05-15T10:00:00Z"}
{"type":"queue-operation","operation":"popAll","content":"draft a prompt","sessionId":"s1","timestamp":"2026-05-15T10:00:01Z"}
{"type":"queue-operation","operation":"remove","sessionId":"s1","timestamp":"2026-05-15T10:00:02Z"}
`)
	c := parse(t, jsonl)
	if len(c.Messages) != 0 {
		t.Fatalf("got %d messages, want 0 (queue bookkeeping is dropped)", len(c.Messages))
	}
}

// TestParse_sentQueuedPromptBecomesUserTurn proves the prompt the user
// sends from the queue reaches the transcript. Claude records a sent
// queued prompt as a queued_command attachment that carries a uuid and
// is replied to by the assistant, so it is a real user turn. The
// "task-notification" command mode is machine-generated markup, not a
// user prompt, so it stays dropped.
func TestParse_sentQueuedPromptBecomesUserTurn(t *testing.T) {
	jsonl := []byte(`{"type":"attachment","uuid":"a1","parentUuid":"p0","timestamp":"2026-05-15T10:00:00Z","attachment":{"type":"queued_command","commandMode":"prompt","prompt":"change the docs if they disagree"}}
{"type":"attachment","uuid":"a2","timestamp":"2026-05-15T10:00:01Z","attachment":{"type":"queued_command","commandMode":"task-notification","prompt":"<task-notification>done</task-notification>"}}
{"type":"assistant","uuid":"as1","parentUuid":"a1","timestamp":"2026-05-15T10:00:05Z","message":{"role":"assistant","model":"m","content":[{"type":"text","text":"on it"}]}}
`)
	c := parse(t, jsonl)
	if c.FirstUserPrompt() != "change the docs if they disagree" {
		t.Errorf("FirstUserPrompt = %q, want the sent queued prompt", c.FirstUserPrompt())
	}
	users := 0
	for _, m := range c.Messages {
		if m.Role == contracts.RoleUser {
			users++
		}
	}
	if users != 1 {
		t.Errorf("got %d user messages, want 1 (the prompt-mode command, not the task-notification)", users)
	}
}

// TestParse_awaySummaryBecomesBlock proves the away_summary system
// subtype is surfaced while ordinary system notes stay dropped. The
// turn_duration record in the fixture is the canonical bookkeeping
// case that must not leak into the transcript.
func TestParse_awaySummaryBecomesBlock(t *testing.T) {
	jsonl := []byte(`{"type":"system","subtype":"turn_duration","uuid":"s1","timestamp":"2026-05-15T10:00:00Z","durationMs":1234}
{"type":"system","subtype":"away_summary","uuid":"s2","timestamp":"2026-05-15T10:00:01Z","content":"Goal is the deploy. Tests pass. Next: run the steps.","isMeta":false}
`)
	c := parse(t, jsonl)
	got := blocksOfType[contracts.AwaySummaryBlock](c)
	if len(got) != 1 {
		t.Fatalf("got %d AwaySummaryBlock, want 1 (turn_duration must stay dropped)", len(got))
	}
	if !strings.Contains(got[0].Text, "Next: run the steps") {
		t.Errorf("AwaySummaryBlock.Text = %q, want the summary content", got[0].Text)
	}
}

// TestParse_fileContextBecomesBlocks proves chronicle folds all three
// file-context attachment shapes into FileContextBlock: a whole
// attached file (content under content.file.content), an edit
// snapshot (content under snippet), and an editor selection (content
// under content). Each stores its text in a different place, so the
// test pins that the parser reads the right field for each. The
// task_reminder record stands in for the bookkeeping inner types that
// must stay dropped rather than flood the transcript.
func TestParse_fileContextBecomesBlocks(t *testing.T) {
	jsonl := []byte(`{"type":"attachment","uuid":"a1","timestamp":"2026-05-15T10:00:00Z","attachment":{"type":"task_reminder","content":[]}}
{"type":"attachment","uuid":"a2","timestamp":"2026-05-15T10:00:01Z","attachment":{"type":"file","filename":"/proj/a.html","content":{"type":"text","file":{"filePath":"/proj/a.html","content":"<!doctype html>","numLines":1}}}}
{"type":"attachment","uuid":"a3","timestamp":"2026-05-15T10:00:02Z","attachment":{"type":"edited_text_file","filename":"/proj/main.go","snippet":"1\tpackage main"}}
{"type":"attachment","uuid":"a4","timestamp":"2026-05-15T10:00:03Z","attachment":{"type":"selected_lines_in_ide","filename":"/proj/x.py","content":"print(1)","lineStart":1,"lineEnd":1}}
`)
	c := parse(t, jsonl)
	got := blocksOfType[contracts.FileContextBlock](c)
	if len(got) != 3 {
		t.Fatalf("got %d FileContextBlock, want 3 (task_reminder must stay dropped)", len(got))
	}
	byPath := map[string]string{}
	for _, b := range got {
		byPath[b.Path] = b.Content
	}
	if !strings.Contains(byPath["/proj/a.html"], "<!doctype html>") {
		t.Errorf("attached file content = %q, want the file body", byPath["/proj/a.html"])
	}
	if !strings.Contains(byPath["/proj/main.go"], "package main") {
		t.Errorf("edited snapshot content = %q, want the snippet", byPath["/proj/main.go"])
	}
	if !strings.Contains(byPath["/proj/x.py"], "print(1)") {
		t.Errorf("selected lines content = %q, want the selection", byPath["/proj/x.py"])
	}
	// The editor attaches these, the user did not type them, so they
	// must not be rendered as user turns.
	for _, m := range c.Messages {
		for _, b := range m.Blocks {
			if _, ok := b.(contracts.FileContextBlock); ok && m.Role != contracts.RoleSystem {
				t.Errorf("file context has role %q, want system (not attributed to the user)", m.Role)
			}
		}
	}
}

// TestParse_knownBookkeepingNeverLeaksAsUnknown guards a real bug:
// the high-volume bookkeeping types ai-title and progress were
// missing from the drop list, so they fell through to the
// unknown-record branch and surfaced as hundreds of near-identical
// UnknownBlock entries per session. None of these are conversation
// content, so they must produce no blocks at all — while a genuinely
// unfamiliar type still has to surface (the resilience canary, tested
// separately in TestParse_syntheticFutureKeepsUnknowns).
func TestParse_knownBookkeepingNeverLeaksAsUnknown(t *testing.T) {
	jsonl := []byte(`{"type":"ai-title","aiTitle":"Some session title","sessionId":"s1","timestamp":"2026-05-15T10:00:00Z"}
{"type":"progress","uuid":"p1","timestamp":"2026-05-15T10:00:01Z","data":{"type":"hook_progress"}}
{"type":"agent-name","agentName":"some-subagent","sessionId":"s1"}
{"type":"permission-mode","permissionMode":"acceptEdits","sessionId":"s1"}
{"type":"user","uuid":"u1","timestamp":"2026-05-15T10:00:02Z","message":{"role":"user","content":"hi"}}
`)
	c := parse(t, jsonl)
	for _, b := range blocksOfType[contracts.UnknownBlock](c) {
		t.Errorf("record kind %q is bookkeeping and must be dropped, but it leaked through as an UnknownBlock", b.Kind)
	}
}

// TestParse_nonStringContentDoesNotDropRecord is the resilience
// guard for the content decode. A future Claude release that writes a
// non-string content on a record we read it from must not break the
// whole-record decode: the record's type, timestamp, and the records
// around it have to survive. We give an away_summary an array content
// and assert it produces no AwaySummaryBlock yet leaves the
// neighbouring real user message intact.
func TestParse_nonStringContentDoesNotDropRecord(t *testing.T) {
	jsonl := []byte(`{"type":"system","subtype":"away_summary","uuid":"s1","timestamp":"2026-05-15T10:00:00Z","content":["unexpected","shape"]}
{"type":"user","uuid":"u1","timestamp":"2026-05-15T10:00:01Z","message":{"role":"user","content":"a real prompt"}}
`)
	c := parse(t, jsonl)
	if got := blocksOfType[contracts.AwaySummaryBlock](c); len(got) != 0 {
		t.Errorf("got %d AwaySummaryBlock, want 0 for non-string content", len(got))
	}
	if c.FirstUserPrompt() != "a real prompt" {
		t.Errorf("FirstUserPrompt = %q, want the real prompt to survive the odd record", c.FirstUserPrompt())
	}
}

// TestParse_documentPartIsReferencedNotDumped proves a base64
// document content part becomes a DocumentBlock that records the MIME
// type and a byte-count reference, the same way images are handled,
// instead of falling through to UnknownBlock and dumping the whole
// base64 payload into the transcript.
func TestParse_documentPartIsReferencedNotDumped(t *testing.T) {
	data := strings.Repeat("A", 5000)
	jsonl := []byte(`{"type":"user","uuid":"u1","timestamp":"2026-05-15T10:00:00Z","message":{"role":"user","content":[{"type":"document","source":{"type":"base64","media_type":"application/pdf","data":"` + data + `"}}]}}` + "\n")
	c := parse(t, jsonl)
	docs := blocksOfType[contracts.DocumentBlock](c)
	if len(docs) != 1 {
		t.Fatalf("got %d DocumentBlock, want 1", len(docs))
	}
	if docs[0].MIME != "application/pdf" {
		t.Errorf("DocumentBlock.MIME = %q, want application/pdf", docs[0].MIME)
	}
	if !strings.HasPrefix(docs[0].PathOrInlineRef, "base64:") {
		t.Errorf("DocumentBlock.PathOrInlineRef = %q, want a base64 byte-count reference", docs[0].PathOrInlineRef)
	}
	if strings.Contains(docs[0].PathOrInlineRef, data) {
		t.Error("DocumentBlock must reference the payload by size, not carry the raw base64")
	}
}

// parse runs the JSONL through the production readSessionFile path
// using an in-memory filesystem, the same trick the readSession
// helper uses for the fixture-file tests.
func parse(t *testing.T, jsonl []byte) contracts.Conversation {
	t.Helper()
	fsys := fstest.MapFS{"projects/-p/s.jsonl": &fstest.MapFile{Data: jsonl}}
	c, err := readSessionFile(fsys, "projects/-p/s.jsonl", contracts.StorageVersion{Adapter: "claude"})
	if err != nil {
		t.Fatalf("readSessionFile: %v", err)
	}
	return c
}
