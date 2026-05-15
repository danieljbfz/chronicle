package steps

import "github.com/danieljbfz/chronicle/contracts"

// FilterOptions tells Filter which blocks and which messages to
// drop. Every field starts as false, so passing an empty
// FilterOptions keeps everything.
//
// Callers turn on the flags they want. The export command turns on
// HideTools when the user passed --no-tools at the command line.
// The user interface turns on HideMeta by default, because the
// slash-command echoes that meta marks are noise in the reading
// view. The flags are independent, so callers can mix and match.
//
// Using a struct of flags instead of separate function arguments
// means there is no constructor to remember and no risk of
// forgetting an argument when a new flag is added later.
type FilterOptions struct {
	HideTools     bool
	HideThinking  bool
	HideMeta      bool
	HideSidechain bool
}

// Filter returns a copy of the conversation with the requested blocks
// and messages removed. The function is pure: it never mutates the
// input. Messages that become empty after block filtering are dropped
// entirely, so a turn that contained only a tool_use disappears when
// HideTools is on. That behaviour is deliberate, because a tool-call
// turn with no surviving text is just noise once tool output is
// hidden.
func Filter(c contracts.Conversation, opts FilterOptions) contracts.Conversation {
	// Step 1: copy the conversation by value, then clear the
	// Messages slice on the copy so we can rebuild it. Without this
	// step we would be mutating the caller's input, because Go slices
	// share their backing arrays after a value copy.
	out := c
	out.Messages = nil

	for _, m := range c.Messages {
		// Step 2: drop whole messages whose top-level flags match a
		// hide-flag the caller turned on. We do this before looking
		// at the blocks because there is no reason to inspect a
		// message we already know we will not keep.
		if opts.HideMeta && m.IsMeta {
			continue
		}
		if opts.HideSidechain && m.IsSidechain {
			continue
		}

		// Step 3: walk the blocks and keep only those the caller
		// allowed. The type assertion at each check is the
		// standard Go idiom for asking "is this Block actually
		// one of these concrete types?" The second return value
		// is true when it is and false when it is not.
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

		// Step 4: drop the entire message when no blocks survived
		// the filter. Otherwise, attach the surviving blocks and
		// keep the message.
		if len(blocks) == 0 {
			continue
		}
		m.Blocks = blocks
		out.Messages = append(out.Messages, m)
	}

	return out
}
