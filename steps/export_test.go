package steps

import (
	"strings"
	"testing"
	"time"

	"github.com/danieljbfz/chronicle/contracts"
)

// TestMarkdown_includesTitleAndMessages is the happy-path test. A
// conversation with a user message and an assistant message should
// produce a document that contains the title (taken from the first
// user prompt), both role headings, and the session identifier in
// the metadata blockquote.
func TestMarkdown_includesTitleAndMessages(t *testing.T) {
	c := contracts.Conversation{
		SessionID: "abc-123",
		Source:    contracts.StorageVersion{Adapter: "claude"},
		StartedAt: time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC),
		Messages: []contracts.Message{
			{Role: contracts.RoleUser, Blocks: []contracts.Block{contracts.TextBlock{Text: "Hello"}}},
			{Role: contracts.RoleAssistant, Blocks: []contracts.Block{contracts.TextBlock{Text: "Hi there"}}},
		},
	}
	out := Markdown(c)
	if !strings.Contains(out, "# Hello") {
		t.Error("output should include title from first prompt")
	}
	if !strings.Contains(out, "## User") || !strings.Contains(out, "## Assistant") {
		t.Error("output should label roles")
	}
	if !strings.Contains(out, "Session `abc-123`") {
		t.Error("output should include session id in metadata")
	}
}

// TestMarkdown_emptySessionRendersNonEmptyTitle proves the renderer
// copes with a session that has no real content. An abandoned
// session is still going to flow through here when the user runs
// an export against its identifier, and the document should not
// render an empty heading at the top. The exact synthetic title
// is owned by contracts.ListingTitle and pinned by that package's
// tests — this test only asserts the renderer surfaces it.
func TestMarkdown_emptySessionRendersNonEmptyTitle(t *testing.T) {
	out := Markdown(contracts.Conversation{})
	const headingPrefix = "# "
	start := strings.Index(out, headingPrefix)
	if start < 0 {
		t.Fatal("Markdown output should begin with a top-level heading")
	}
	rest := out[start+len(headingPrefix):]
	end := strings.Index(rest, "\n")
	if end < 0 {
		t.Fatal("Markdown heading should end with a newline")
	}
	title := strings.TrimSpace(rest[:end])
	if title == "" {
		t.Error("Markdown heading for an empty conversation should not be blank")
	}
}

// TestMarkdown_preservesUnknownBlock is the canary for the renderer
// side of the resilience contract: if the parser hands us an
// UnknownBlock, the renderer must surface both the kind label and
// the raw JSON. Dropping either one would silently lose information,
// which is exactly what the contract forbids.
func TestMarkdown_preservesUnknownBlock(t *testing.T) {
	c := contracts.Conversation{
		Messages: []contracts.Message{{
			Role: contracts.RoleAssistant,
			Blocks: []contracts.Block{
				contracts.UnknownBlock{Kind: "future_kind", Raw: []byte(`{"weird":true}`)},
			},
		}},
	}
	out := Markdown(c)
	if !strings.Contains(out, "future_kind") {
		t.Error("Markdown should mention the unknown kind")
	}
	if !strings.Contains(out, `"weird":true`) {
		t.Error("Markdown should preserve the raw JSON of an unknown block")
	}
}

// TestMarkdown_renderToolBlocks proves the tool-call and tool-result
// renderings end up as recognisable, navigable Markdown. The
// strings the test looks for are the same labels a human reader
// would skim for in the document.
func TestMarkdown_renderToolBlocks(t *testing.T) {
	c := contracts.Conversation{
		Messages: []contracts.Message{{
			Role: contracts.RoleAssistant,
			Blocks: []contracts.Block{
				contracts.ToolUseBlock{Tool: "Bash", CallID: "1", Input: []byte(`{"cmd":"ls"}`)},
			},
		}, {
			Role: contracts.RoleUser,
			Blocks: []contracts.Block{
				contracts.ToolResultBlock{CallID: "1", Output: "file.txt"},
			},
		}},
	}
	out := Markdown(c)
	if !strings.Contains(out, "Tool call") || !strings.Contains(out, "Bash") {
		t.Error("ToolUseBlock should render as a Tool call")
	}
	if !strings.Contains(out, "Tool result") || !strings.Contains(out, "file.txt") {
		t.Error("ToolResultBlock should render as a Tool result")
	}
}

// TestMarkdown_renderRescuedBlocks proves the rescued block kinds
// render with their own labels and carry their content through. The
// labels are what a reader skims for to tell an away summary or a
// file-context snapshot apart from a typed turn.
func TestMarkdown_renderRescuedBlocks(t *testing.T) {
	c := contracts.Conversation{
		Messages: []contracts.Message{
			{Role: contracts.RoleAssistant, Blocks: []contracts.Block{contracts.AwaySummaryBlock{Text: "goal and next steps"}}},
			{Role: contracts.RoleSystem, Blocks: []contracts.Block{contracts.FileContextBlock{Path: "/proj/main.go", Content: "1\tpackage main"}}},
		},
	}
	out := Markdown(c)
	for _, want := range []string{
		"Away summary", "goal and next steps",
		"File context", "/proj/main.go", "package main",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Markdown missing %q\n---\n%s", want, out)
		}
	}
}

// TestMarkdown_blockquoteDropsTrailingQuoteLine pins the blockquote
// renderer against the trailing-quote-line bug. Assistant thinking
// text routinely ends in a newline, and the renderer must not turn
// that trailing newline into a stray "> " line at the end of the
// quote. Thinking and away-summary blocks render through the same
// helper, so pinning thinking covers both.
func TestMarkdown_blockquoteDropsTrailingQuoteLine(t *testing.T) {
	c := contracts.Conversation{
		Messages: []contracts.Message{{
			Role:   contracts.RoleAssistant,
			Blocks: []contracts.Block{contracts.ThinkingBlock{Text: "I reasoned.\n"}},
		}},
	}
	out := Markdown(c)
	if !strings.Contains(out, "> _Thinking_\n>\n> I reasoned.\n\n") {
		t.Errorf("thinking blockquote should end cleanly after the content line\n---\n%s", out)
	}
	if strings.Contains(out, "> \n") {
		t.Errorf("blockquote should not contain a stray empty quote line\n---\n%s", out)
	}
}

// TestMarkdown_toolResultTurnIsLabeledTool pins the heading for a
// turn that carries only a tool result. The providers file these on
// different roles — Claude on a user-role record — so the renderer
// labels the turn "Tool" rather than echoing a role that would read
// as if the person had typed the tool's output.
func TestMarkdown_toolResultTurnIsLabeledTool(t *testing.T) {
	c := contracts.Conversation{
		Messages: []contracts.Message{{
			Role:   contracts.RoleUser,
			Blocks: []contracts.Block{contracts.ToolResultBlock{CallID: "1", Output: "ok"}},
		}},
	}
	out := Markdown(c)
	if !strings.Contains(out, "## Tool") {
		t.Errorf("a tool-result-only turn should be labeled Tool\n---\n%s", out)
	}
	if strings.Contains(out, "## User") {
		t.Errorf("a tool-result-only turn should not be labeled by its carrying role\n---\n%s", out)
	}
}

// TestMarkdown_assistantTurnWithToolsKeepsItsRole guards the other
// side: a turn that mixes prose with tool blocks is the assistant's
// own turn and keeps the Assistant heading.
func TestMarkdown_assistantTurnWithToolsKeepsItsRole(t *testing.T) {
	c := contracts.Conversation{
		Messages: []contracts.Message{{
			Role: contracts.RoleAssistant,
			Blocks: []contracts.Block{
				contracts.TextBlock{Text: "Running it now."},
				contracts.ToolResultBlock{CallID: "1", Output: "done"},
			},
		}},
	}
	out := Markdown(c)
	if !strings.Contains(out, "## Assistant") {
		t.Errorf("a mixed prose-and-tool turn should keep its role heading\n---\n%s", out)
	}
}

// TestMarkdown_fencedContentSurvivesAnInnerFence pins fence safety.
// Tool output, file context, and raw unknown JSON are dumped inside
// code fences, and that content often contains its own ``` fence (a
// tool that printed a Markdown file, for example). A bare three-tick
// fence would let the inner fence close the block early and spill the
// rest of the document out as broken Markdown, so the renderer opens
// a fence longer than the longest backtick run in the content.
func TestMarkdown_fencedContentSurvivesAnInnerFence(t *testing.T) {
	output := "before\n```\ncode inside\n```\nafter"
	c := contracts.Conversation{
		Messages: []contracts.Message{{
			Role:   contracts.RoleUser,
			Blocks: []contracts.Block{contracts.ToolResultBlock{CallID: "1", Output: output}},
		}},
	}
	out := Markdown(c)
	if !strings.Contains(out, output) {
		t.Errorf("the tool output should survive intact inside the fence\n---\n%s", out)
	}
	if !strings.Contains(out, "````\n"+output) {
		t.Errorf("content with an inner ``` should open a longer (four-tick) fence\n---\n%s", out)
	}
}

// TestMarkdown_encryptedThinkingRendersMarker pins the rendering of
// Claude's "omitted" thinking: a thinking block with no readable
// text but an encrypted signature. It must render an honest marker
// that the reasoning happened, not an empty thinking quote with no
// body.
func TestMarkdown_encryptedThinkingRendersMarker(t *testing.T) {
	c := contracts.Conversation{
		Messages: []contracts.Message{{
			Role:   contracts.RoleAssistant,
			Blocks: []contracts.Block{contracts.ThinkingBlock{Encrypted: true}},
		}},
	}
	out := Markdown(c)
	if !strings.Contains(out, "encrypted form") {
		t.Errorf("encrypted thinking should render an honest marker\n---\n%s", out)
	}
	if strings.Contains(out, "> _Thinking_\n>\n") {
		t.Errorf("encrypted thinking should not render an empty thinking quote\n---\n%s", out)
	}
}
