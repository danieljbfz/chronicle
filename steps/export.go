package steps

import (
	"fmt"
	"strings"
	"time"

	"github.com/danieljbfz/chronicle/contracts"
)

// Markdown renders a Conversation as a Markdown document a person
// can read. It does not filter anything by itself. Callers that
// want to drop tool calls or thinking blocks should run Filter
// first and then pass the result to Markdown.
//
// Keeping filtering and rendering as separate steps means each
// function has one job. The filter tests do not have to know
// anything about Markdown, and the Markdown tests do not have to
// know anything about filters.
func Markdown(c contracts.Conversation) string {
	var builder strings.Builder

	// Step 1: write the front matter with the title and a one-line
	// metadata blockquote.
	writeHeader(&builder, c)

	// Step 2: write each message as its own section, prefixed with
	// the role as a level-two heading.
	for _, m := range c.Messages {
		writeMessage(&builder, m)
	}

	return builder.String()
}

// writeHeader emits a top-level Markdown heading and a one-line
// metadata blockquote underneath. We accept a pointer to the
// builder so every helper in this file writes into the same
// growing buffer. Returning strings from each helper and gluing
// them together afterwards would do the same thing with more
// allocations and more code.
//
// The heading reads ListingTitle, which runs the same identity
// cascade the session-list surfaces rely on (Title, then first
// user prompt, then slash-command name, then first assistant
// reply, then first tool name, then a synthetic last-resort
// identity built from the timestamp or the session id). A
// session that began with a slash command or a tool result no
// longer renders as "# (empty session)" — the cascade picks up
// the user's actual first action and surfaces it as the
// document's title.
func writeHeader(builder *strings.Builder, c contracts.Conversation) {
	fmt.Fprintf(builder, "# %s\n\n", c.ListingTitle())
	fmt.Fprintf(builder, "> Session `%s`  ·  Provider `%s`  ·  Started %s\n\n",
		c.SessionID, c.Source.Adapter, formatTime(c.StartedAt))
	builder.WriteString("---\n\n")
}

// writeMessage emits one message as a Markdown section. The role
// becomes a level-two heading, and each block is written by
// writeBlock in turn. The trailing blank line gives every section a
// consistent visual gap from the next.
func writeMessage(builder *strings.Builder, m contracts.Message) {
	builder.WriteString(messageHeading(m))
	builder.WriteString("\n\n")
	for _, b := range m.Blocks {
		writeBlock(builder, b)
	}
	builder.WriteString("\n")
}

// messageHeading returns the level-two heading for a message. A turn
// whose content is entirely tool results is labeled "Tool" rather
// than by the role of the record that carried it. The providers
// disagree on that role — Claude files a tool result on a user-role
// record, the copilot agent on a system-role record — so keying the
// heading on the role would render the same thing as "User" in one
// export and "System" in another, and "User" in particular reads as
// if the person had typed the tool's output. A turn that mixes tool
// results with prose keeps its role heading, because the prose is
// the assistant's or the user's own.
func messageHeading(m contracts.Message) string {
	if isAllToolResults(m.Blocks) {
		return "## Tool"
	}
	switch m.Role {
	case contracts.RoleUser:
		return "## User"
	case contracts.RoleAssistant:
		return "## Assistant"
	case contracts.RoleSystem:
		return "## System"
	default:
		return "## " + string(m.Role)
	}
}

// isAllToolResults reports whether every block in a non-empty message
// is a tool result, the shape of a turn that carries only a tool's
// output and no prose of its own.
func isAllToolResults(blocks []contracts.Block) bool {
	if len(blocks) == 0 {
		return false
	}
	for _, b := range blocks {
		if _, ok := b.(contracts.ToolResultBlock); !ok {
			return false
		}
	}
	return true
}

// writeBlock dispatches on the concrete type of the Block and writes
// the right Markdown for each one. The switch is the standard Go way
// to handle this kind of "act differently for each concrete type
// behind an interface" situation: each case binds the variable v to
// the unwrapped value, so we can read its fields directly without a
// second assertion.
func writeBlock(builder *strings.Builder, b contracts.Block) {
	switch v := b.(type) {
	case contracts.TextBlock:
		builder.WriteString(v.Text)
		builder.WriteString("\n\n")
	case contracts.ThinkingBlock:
		// Readable reasoning renders as a labeled blockquote. The
		// encrypted "omitted" shape has no text to show, so it
		// renders as a one-line marker that records the reasoning
		// happened without pretending to quote it.
		if v.Text != "" {
			writeBlockquote(builder, "_Thinking_", v.Text)
		} else if v.Encrypted {
			builder.WriteString("> _Thinking — recorded in encrypted form, no readable text_\n\n")
		}
	case contracts.ToolUseBlock:
		fmt.Fprintf(builder, "**Tool call**: `%s` (id `%s`)\n\n", v.Tool, v.CallID)
		writeFenced(builder, "json", string(v.Input))
	case contracts.ToolResultBlock:
		marker := "Tool result"
		if v.IsError {
			marker = "Tool error"
		}
		fmt.Fprintf(builder, "**%s** (id `%s`)\n\n", marker, v.CallID)
		writeFenced(builder, "", v.Output)
	case contracts.ImageBlock:
		fmt.Fprintf(builder, "_[Image: %s · %s]_\n\n", v.MIME, v.PathOrInlineRef)
	case contracts.DocumentBlock:
		fmt.Fprintf(builder, "_[Document: %s · %s]_\n\n", v.MIME, v.PathOrInlineRef)
	case contracts.AwaySummaryBlock:
		writeBlockquote(builder, "_Away summary_", v.Text)
	case contracts.FileContextBlock:
		// Name the file, then dump its content in a plain fence. We do
		// not guess a language from the extension because the content
		// usually carries leading line numbers, which would make a
		// language-tagged fence render as broken syntax anyway.
		fmt.Fprintf(builder, "**File context**: `%s`\n\n", v.Path)
		if v.Content != "" {
			writeFenced(builder, "", v.Content)
		}
	case contracts.UnknownBlock:
		// The resilience contract requires us to keep unknown
		// content visible. We label it clearly and dump the raw JSON
		// inside a fenced block so the reader can still see what
		// the upstream tool wrote.
		fmt.Fprintf(builder, "_Unknown block kind `%s` (preserved as raw JSON below)_\n\n", v.Kind)
		writeFenced(builder, "json", string(v.Raw))
	}
}

// writeFenced writes content inside a fenced code block tagged with
// the given info string (for example "json", or "" for a plain
// fence). The fence is always one backtick longer than the longest
// backtick run in the content, so content that itself contains a
// ``` line cannot close the fence early and spill the rest of the
// document out as broken Markdown. The closing fence always lands on
// its own line, even when the content does not end in a newline.
func writeFenced(builder *strings.Builder, info, content string) {
	width := longestBacktickRun(content) + 1
	if width < 3 {
		width = 3
	}
	fence := strings.Repeat("`", width)

	builder.WriteString(fence)
	builder.WriteString(info)
	builder.WriteByte('\n')
	builder.WriteString(content)
	if !strings.HasSuffix(content, "\n") {
		builder.WriteByte('\n')
	}
	builder.WriteString(fence)
	builder.WriteString("\n\n")
}

// longestBacktickRun returns the length of the longest run of
// consecutive backticks in s, or zero when it has none.
func longestBacktickRun(s string) int {
	longest, run := 0, 0
	for _, r := range s {
		if r == '`' {
			run++
			if run > longest {
				longest = run
			}
			continue
		}
		run = 0
	}
	return longest
}

// writeBlockquote renders labeled text as a Markdown blockquote so
// it recedes from the main turn flow while staying legible. Thinking
// and away-summary content both use it: a reader can tell them apart
// by the label and skim past them without losing the answer.
//
// The trailing newline that assistant content routinely carries is
// trimmed before the split. Without the trim, the split yields an
// empty final element and the quote gains a stray "> " line at its
// end.
func writeBlockquote(builder *strings.Builder, label, text string) {
	builder.WriteString("> ")
	builder.WriteString(label)
	builder.WriteString("\n>\n")
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		builder.WriteString("> ")
		builder.WriteString(line)
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
}

// formatTime returns an RFC 3339 timestamp, or the literal string
// "(unknown)" for the zero value of time.Time. Using the zero value
// to mean "no time set" is a common Go convention because the
// language has no nullable types.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return "(unknown)"
	}
	return t.Format(time.RFC3339)
}
