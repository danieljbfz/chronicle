package contracts

import (
	"strings"
	"time"
)

// ListingTitle returns the identity a listing surface uses for one
// session. The transcript header has FirstUserPrompt, which skips
// meta records to keep the rendered conversation clean. The
// listing has the opposite need: every row should carry a label a
// reader can recognise, even for sessions whose first turn was a
// slash command, a tool result, or an attached file with no
// accompanying text. ListingTitle is the function listings reach
// for, and it never returns the empty string.
//
// The cascade picks the first non-empty value among, in order:
//
//  1. The Title field, when the upstream tool recorded one.
//  2. The first real user prompt (the FirstUserPrompt value).
//  3. The first slash-command name, extracted from the
//     `<command-name>…</command-name>` wrapper Claude Code writes
//     when a session begins with a command like /clear or /compact.
//  4. The first non-empty assistant text, which often carries the
//     user's intent even when no real user text exists.
//  5. The first tool the assistant invoked, by tool name.
//  6. A synthetic identity built from StartedAt or SessionID, so
//     the result is never the empty string.
//
// The return value collapses internal whitespace to single spaces
// and clips long values to maxListingTitleRunes runes.
func (c Conversation) ListingTitle() string {
	// Step 1: trust the upstream-recorded title when one exists.
	if title := condenseWhitespace(c.Title); title != "" {
		return clipRunes(title, maxListingTitleRunes)
	}

	// Step 2: fall through to the first real user prompt.
	if prompt := condenseWhitespace(c.FirstUserPrompt()); prompt != "" {
		return clipRunes(prompt, maxListingTitleRunes)
	}

	// Step 3: when the only user content was a slash command,
	// extract the command name from the meta record's wrapper.
	if name := firstSlashCommandName(c.Messages); name != "" {
		return clipRunes(name, maxListingTitleRunes)
	}

	// Step 4: surface the assistant's first reply when the user
	// record is empty of real text.
	if reply := condenseWhitespace(firstAssistantText(c.Messages)); reply != "" {
		return clipRunes(reply, maxListingTitleRunes)
	}

	// Step 5: a tool invocation is the next-best identity.
	if tool := firstToolName(c.Messages); tool != "" {
		return tool
	}

	// Step 6: last resort. The listing always has identity.
	return syntheticListingTitle(c.StartedAt, c.SessionID)
}

// maxListingTitleRunes bounds the length of the title every
// listing surface (the CLI's `chronicle list`, the TUI's session
// browser, a future web frontend) receives from ListingTitle. The
// limit is a soft upper bound on the conversation-level identity
// the function returns. Each presentation layer is free to
// truncate further to fit the width it actually has.
const maxListingTitleRunes = 120

// condenseWhitespace collapses every newline, carriage return, and
// tab to a space, collapses runs of spaces to one, and trims the
// trailing space the collapse can leave behind. The function is
// the one place ListingTitle and its helpers reach for when they
// need a single-line version of a string that might carry embedded
// breaks.
func condenseWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := true
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' {
			r = ' '
		}
		if r == ' ' {
			if !prevSpace {
				b.WriteRune(' ')
				prevSpace = true
			}
			continue
		}
		b.WriteRune(r)
		prevSpace = false
	}
	return strings.TrimRight(b.String(), " ")
}

// clipRunes returns at most max runes of s, appending an ellipsis
// when the value was actually truncated. The rune count keeps
// multi-byte titles from over- or under-shooting the budget.
func clipRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}

// slashCommandOpenTag and slashCommandCloseTag bracket the command
// name inside the wrapper Claude Code writes when a session begins
// with a slash command. The shape is stable across the Claude Code
// releases chronicle has observed, and matching the tag rather
// than the inner content means new slash commands need no parser
// update.
const (
	slashCommandOpenTag  = "<command-name>"
	slashCommandCloseTag = "</command-name>"
)

// firstSlashCommandName walks the messages and returns the name
// inside the first `<command-name>…</command-name>` wrapper it
// finds in a meta user message. Recognising the wrapper here keeps
// every consumer (CLI listings, TUI rows, the future web frontend)
// from re-implementing the same parse against the same shape.
func firstSlashCommandName(messages []Message) string {
	for _, m := range messages {
		if m.Role != RoleUser || !m.IsMeta {
			continue
		}
		for _, b := range m.Blocks {
			t, ok := b.(TextBlock)
			if !ok {
				continue
			}
			text := strings.TrimSpace(t.Text)
			if !strings.HasPrefix(text, slashCommandOpenTag) {
				continue
			}
			inner := strings.TrimPrefix(text, slashCommandOpenTag)
			end := strings.Index(inner, slashCommandCloseTag)
			if end < 0 {
				continue
			}
			return condenseWhitespace(inner[:end])
		}
	}
	return ""
}

// firstAssistantText walks the messages and returns the first
// non-empty text the assistant produced. Assistant messages can
// interleave TextBlock, ThinkingBlock, and tool blocks. Only the
// TextBlock contents count as visible reply text.
func firstAssistantText(messages []Message) string {
	for _, m := range messages {
		if m.Role != RoleAssistant || m.IsMeta {
			continue
		}
		for _, b := range m.Blocks {
			if t, ok := b.(TextBlock); ok && t.Text != "" {
				return t.Text
			}
		}
	}
	return ""
}

// firstToolName walks every message and returns the name of the
// first ToolUseBlock the conversation contains. The function does
// not restrict the walk to assistant messages — a session whose
// first non-empty action lives on an unusual envelope still
// produces identity.
func firstToolName(messages []Message) string {
	for _, m := range messages {
		for _, b := range m.Blocks {
			if u, ok := b.(ToolUseBlock); ok && u.Tool != "" {
				return u.Tool
			}
		}
	}
	return ""
}

// shortSessionIDPrefix is the number of leading runes ListingTitle
// keeps from a SessionID when it falls back to the id-derived
// synthetic title. Eight characters is enough to disambiguate
// hundreds of sessions on the same day without overflowing a
// narrow listing column.
const shortSessionIDPrefix = 8

// syntheticListingTitle is the last-resort identity for a session
// that has no recognisable content. The date form keeps the row
// sortable in a reader's eye when the timestamp is known. The id
// form covers the rare case where even the timestamp is the zero
// value. The constant string at the end is the floor — a row that
// reaches it has neither id nor timestamp, and the synthetic name
// makes the absence visible.
func syntheticListingTitle(startedAt time.Time, id SessionID) string {
	if !startedAt.IsZero() {
		return "Session started " + startedAt.Format("2006-01-02")
	}
	if id != "" {
		short := string(id)
		if len(short) > shortSessionIDPrefix {
			short = short[:shortSessionIDPrefix]
		}
		return "Session " + short
	}
	return "Session (no metadata)"
}
