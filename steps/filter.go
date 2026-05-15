package steps

import "github.com/danieljbfz/chronicle/contracts"

// FilterOptions controls which blocks and messages survive a Filter pass.
// All fields default to false — zero value keeps everything.
type FilterOptions struct {
	HideTools     bool // drop ToolUseBlock and ToolResultBlock
	HideThinking  bool // drop ThinkingBlock
	HideMeta      bool // drop messages with IsMeta = true
	HideSidechain bool // drop messages with IsSidechain = true
}

// Filter returns a copy of the conversation with the requested blocks and
// messages removed. The function is pure: it never mutates the input.
//
// Messages that become empty after block filtering are dropped, so a turn
// that contained only a tool_use disappears entirely when HideTools is set.
func Filter(c contracts.Conversation, opts FilterOptions) contracts.Conversation {
	// Step 1: shallow copy the conversation; we will replace Messages.
	out := c
	out.Messages = nil

	for _, m := range c.Messages {
		// Step 2: skip whole messages when the opt-out matches.
		if opts.HideMeta && m.IsMeta {
			continue
		}
		if opts.HideSidechain && m.IsSidechain {
			continue
		}

		// Step 3: filter blocks within the message.
		blocks := make([]contracts.Block, 0, len(m.Blocks))
		for _, b := range m.Blocks {
			if opts.HideTools {
				if _, ok := b.(contracts.ToolUseBlock); ok {
					continue
				}
				if _, ok := b.(contracts.ToolResultBlock); ok {
					continue
				}
			}
			if opts.HideThinking {
				if _, ok := b.(contracts.ThinkingBlock); ok {
					continue
				}
			}
			blocks = append(blocks, b)
		}

		// Step 4: drop the message if no blocks remain.
		if len(blocks) == 0 {
			continue
		}
		m.Blocks = blocks
		out.Messages = append(out.Messages, m)
	}

	return out
}
