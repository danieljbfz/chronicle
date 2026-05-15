package steps

import (
	"strings"
	"testing"
	"time"

	"github.com/danieljbfz/chronicle/contracts"
)

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

func TestMarkdown_emptySessionHasFallbackTitle(t *testing.T) {
	out := Markdown(contracts.Conversation{})
	if !strings.Contains(out, "(empty session)") {
		t.Error("empty conversation should render fallback title")
	}
}

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
