package steps

import (
	"fmt"
	"strings"
	"time"

	"github.com/danieljbfz/chronicle/contracts"
)

// Markdown renders a Conversation as a human-readable Markdown
// document. The function does not filter anything: callers that want
// to omit tools or thinking should call Filter first and then pass
// the filtered conversation to Markdown. Keeping the two concerns
// separate means each step has one job and tests for one can stay
// independent of the other.
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
// metadata blockquote underneath. We accept a pointer to the builder
// because we want every helper in this file to write into the same
// growing buffer rather than producing strings to glue together
// afterwards.
func writeHeader(builder *strings.Builder, c contracts.Conversation) {
	title := c.Title
	if title == "" {
		title = c.FirstUserPrompt()
	}
	if title == "" {
		title = "(empty session)"
	}
	fmt.Fprintf(builder, "# %s\n\n", title)
	fmt.Fprintf(builder, "> Session `%s`  ·  Provider `%s`  ·  Started %s\n\n",
		c.SessionID, c.Source.Adapter, formatTime(c.StartedAt))
	builder.WriteString("---\n\n")
}

// writeMessage emits one message as a Markdown section. The role
// becomes a level-two heading, and each block is written by
// writeBlock in turn. The trailing blank line gives every section a
// consistent visual gap from the next.
func writeMessage(builder *strings.Builder, m contracts.Message) {
	switch m.Role {
	case contracts.RoleUser:
		builder.WriteString("## User\n\n")
	case contracts.RoleAssistant:
		builder.WriteString("## Assistant\n\n")
	case contracts.RoleSystem:
		builder.WriteString("## System\n\n")
	default:
		fmt.Fprintf(builder, "## %s\n\n", m.Role)
	}
	for _, b := range m.Blocks {
		writeBlock(builder, b)
	}
	builder.WriteString("\n")
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
		// Render thinking as a Markdown blockquote so it visually
		// recedes from the actual answer. The user can still read
		// it, but the formatting makes clear that it is a separate
		// kind of content.
		builder.WriteString("> _Thinking_\n>\n")
		for _, line := range strings.Split(v.Text, "\n") {
			builder.WriteString("> ")
			builder.WriteString(line)
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	case contracts.ToolUseBlock:
		fmt.Fprintf(builder, "**Tool call**: `%s` (id `%s`)\n\n```json\n%s\n```\n\n",
			v.Tool, v.CallID, string(v.Input))
	case contracts.ToolResultBlock:
		marker := "Tool result"
		if v.IsError {
			marker = "Tool error"
		}
		fmt.Fprintf(builder, "**%s** (id `%s`)\n\n```\n%s\n```\n\n", marker, v.CallID, v.Output)
	case contracts.ImageBlock:
		fmt.Fprintf(builder, "_[Image: %s · %s]_\n\n", v.MIME, v.PathOrInlineRef)
	case contracts.UnknownBlock:
		// The resilience contract requires us to keep unknown
		// content visible. We label it clearly and dump the raw JSON
		// inside a fenced block so the reader can still see what
		// the upstream tool wrote.
		fmt.Fprintf(builder, "_Unknown block kind `%s` (preserved as raw JSON below)_\n\n```json\n%s\n```\n\n",
			v.Kind, string(v.Raw))
	}
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
