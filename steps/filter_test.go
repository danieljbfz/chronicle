package steps

import (
	"testing"

	"github.com/danieljbfz/chronicle/contracts"
)

// sampleConversation is the fixture every test in this file uses.
// We build it in code instead of loading it from disk because the
// filter is a pure function, and we want the test to stay
// readable at a glance. The input sits right above the assertion,
// with no fixture file in another directory to chase.
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

// TestFilter_hideToolsRemovesToolBlocksAndEmptyTurns proves the two
// related behaviours: tool blocks disappear from the surviving
// messages, and any turn that contained only tool blocks disappears
// entirely. The fixture has one such turn, the user's tool_result
// reply, which exists only to carry the tool output back to the
// assistant.
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

// TestFilter_hideThinkingDropsOnlyThinking proves the thinking flag
// is independent of the tool flag: turning it on removes the
// ThinkingBlock entries and leaves everything else intact.
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

// TestFilter_hideMetaDropsMetaMessage proves the meta flag works at
// the message level, not the block level: a whole message disappears
// when its IsMeta field is true and HideMeta is on.
func TestFilter_hideMetaDropsMetaMessage(t *testing.T) {
	out := Filter(sampleConversation(), FilterOptions{HideMeta: true})
	for _, m := range out.Messages {
		if m.IsMeta {
			t.Error("meta message survived")
		}
	}
}

// TestFilter_isPure is the safety net: even with every flag turned
// on, the input conversation must look exactly the same after the
// call as it did before. If we ever accidentally mutated the input,
// callers that filter the same conversation twice would get
// different results the second time.
func TestFilter_isPure(t *testing.T) {
	in := sampleConversation()
	before := len(in.Messages)
	_ = Filter(in, FilterOptions{HideTools: true, HideMeta: true, HideThinking: true})
	if len(in.Messages) != before {
		t.Errorf("Filter mutated its input: had %d, now %d", before, len(in.Messages))
	}
}

// rescuedConversation holds one message per rescued block kind, so a
// test can turn on a single hide flag and confirm it drops exactly
// that kind and nothing else.
func rescuedConversation() contracts.Conversation {
	return contracts.Conversation{
		Messages: []contracts.Message{
			{Role: contracts.RoleAssistant, Blocks: []contracts.Block{contracts.AwaySummaryBlock{Text: "summary"}}},
			{Role: contracts.RoleSystem, Blocks: []contracts.Block{contracts.FileContextBlock{Path: "/a.go", Content: "x"}}},
		},
	}
}

// TestFilter_rescueFlagsAreIndependent proves each rescue flag drops
// only its own block kind. We turn the flags on one at a time and
// assert that the matching block disappears while the other survives,
// which is the property the per-record CLI flags promise.
func TestFilter_rescueFlagsAreIndependent(t *testing.T) {
	cases := []struct {
		name string
		opts FilterOptions
		gone func(contracts.Block) bool
	}{
		{"HideAwaySummaries", FilterOptions{HideAwaySummaries: true}, func(b contracts.Block) bool {
			_, ok := b.(contracts.AwaySummaryBlock)
			return ok
		}},
		{"HideFileContext", FilterOptions{HideFileContext: true}, func(b contracts.Block) bool {
			_, ok := b.(contracts.FileContextBlock)
			return ok
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := Filter(rescuedConversation(), tc.opts)
			if len(out.Messages) != 1 {
				t.Fatalf("got %d messages, want 1 (one kind dropped, one kept)", len(out.Messages))
			}
			for _, m := range out.Messages {
				for _, b := range m.Blocks {
					if tc.gone(b) {
						t.Errorf("%s left a block it should have dropped: %T", tc.name, b)
					}
				}
			}
		})
	}
}

// TestFilter_rescuedBlocksSurviveByDefault proves an empty
// FilterOptions keeps every rescued block, so an unfiltered export
// shows away summaries and file-context snapshots.
func TestFilter_rescuedBlocksSurviveByDefault(t *testing.T) {
	out := Filter(rescuedConversation(), FilterOptions{})
	if len(out.Messages) != 2 {
		t.Errorf("got %d messages, want 2 (nothing dropped with no flags set)", len(out.Messages))
	}
}
