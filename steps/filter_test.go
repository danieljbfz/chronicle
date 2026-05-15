package steps

import (
	"testing"

	"github.com/danieljbfz/chronicle/contracts"
)

func sampleConversation() contracts.Conversation {
	return contracts.Conversation{
		Messages: []contracts.Message{
			{
				Role: contracts.RoleUser,
				Blocks: []contracts.Block{
					contracts.TextBlock{Text: "first prompt"},
				},
			},
			{
				Role: contracts.RoleAssistant,
				Blocks: []contracts.Block{
					contracts.ThinkingBlock{Text: "let me think"},
					contracts.TextBlock{Text: "reply"},
					contracts.ToolUseBlock{Tool: "Read", CallID: "1"},
				},
			},
			{
				Role: contracts.RoleUser,
				Blocks: []contracts.Block{
					contracts.ToolResultBlock{CallID: "1", Output: "file body"},
				},
			},
			{
				Role:   contracts.RoleUser,
				IsMeta: true,
				Blocks: []contracts.Block{contracts.TextBlock{Text: "<command>/clear</command>"}},
			},
		},
	}
}

func TestFilter_hideToolsRemovesToolBlocksAndEmptyTurns(t *testing.T) {
	out := Filter(sampleConversation(), FilterOptions{HideTools: true})
	if len(out.Messages) != 3 {
		t.Fatalf("got %d messages, want 3", len(out.Messages))
	}
	for _, m := range out.Messages {
		for _, b := range m.Blocks {
			switch b.(type) {
			case contracts.ToolUseBlock, contracts.ToolResultBlock:
				t.Errorf("tool block survived filter: %T", b)
			}
		}
	}
}

func TestFilter_hideThinkingDropsOnlyThinking(t *testing.T) {
	out := Filter(sampleConversation(), FilterOptions{HideThinking: true})
	for _, m := range out.Messages {
		for _, b := range m.Blocks {
			if _, ok := b.(contracts.ThinkingBlock); ok {
				t.Error("ThinkingBlock survived")
			}
		}
	}
}

func TestFilter_hideMetaDropsMetaMessage(t *testing.T) {
	out := Filter(sampleConversation(), FilterOptions{HideMeta: true})
	for _, m := range out.Messages {
		if m.IsMeta {
			t.Error("meta message survived")
		}
	}
}

func TestFilter_isPure(t *testing.T) {
	in := sampleConversation()
	before := len(in.Messages)
	_ = Filter(in, FilterOptions{HideTools: true, HideMeta: true, HideThinking: true})
	if len(in.Messages) != before {
		t.Errorf("Filter mutated its input: had %d, now %d", before, len(in.Messages))
	}
}
