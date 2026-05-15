package steps

// -----------------------------------------------------------------------
// Go concepts introduced in this file
// -----------------------------------------------------------------------
//
// 1. TYPE SWITCH WITH ASSIGNMENT. The construct
//        switch v := b.(type) {
//        case TextBlock: ...
//        case ToolUseBlock: ...
//        }
//    reads as: "look at the concrete type behind the interface value
//    `b`; in each case the variable `v` is bound to the unwrapped
//    typed value." This is how we discriminate between the block kinds
//    that all satisfy the Block interface. Type switches are the
//    primary idiom for working with interface types in Go.
//
// 2. ZERO-VALUE STRUCTS AS DEFAULT OPTIONS. The `FilterOptions` struct
//    below has every field default to false. Callers can write
//        Filter(c, FilterOptions{})         // keeps everything
//        Filter(c, FilterOptions{HideTools: true})
//    No constructor needed; the zero value is meaningful. This is a
//    common Go pattern that often replaces builder objects in other
//    languages.
//
// 3. SHALLOW COPY OF STRUCTS. `out := c` copies the Conversation by
//    value: all the scalar fields are independent in `out`, but the
//    `Messages` slice header still points at the same backing array
//    as `c.Messages`. That is why we set `out.Messages = nil` and
//    append fresh messages — without that, mutating `out.Messages`
//    would also change `c.Messages`. The function is "pure" with
//    respect to its caller's view: we never mutate the input slice
//    elements either.

import "github.com/danieljbfz/chronicle/contracts"

// FilterOptions controls which blocks and messages survive a Filter
// pass. All fields default to false — zero value keeps everything.
type FilterOptions struct {
	HideTools     bool // drop ToolUseBlock and ToolResultBlock
	HideThinking  bool // drop ThinkingBlock
	HideMeta      bool // drop messages with IsMeta = true
	HideSidechain bool // drop messages with IsSidechain = true
}

// Filter returns a copy of the conversation with the requested blocks
// and messages removed. The function is pure: it never mutates the input.
//
// Messages that become empty after block filtering are dropped, so a
// turn that contained only a tool_use disappears entirely when HideTools
// is set. That's intentional: a tool-call turn with no text is just noise
// when tool output is hidden.
func Filter(c contracts.Conversation, opts FilterOptions) contracts.Conversation {
	// Step 1: shallow copy the conversation; we will replace Messages.
	// See concept #3 above for why we cannot just mutate `c`.
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
		// `b.(contracts.ToolUseBlock)` is a type assertion — see
		// contracts/conversation.go for the explanation.
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
