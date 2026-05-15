package steps

import (
	"fmt"
	"strings"
	"time"

	"github.com/danieljbfz/chronicle/contracts"
)

// Markdown renders a Conversation as a human-readable Markdown document.
// Apply Filter first if you want to omit tools or thinking; Markdown does
// not filter, it only renders whatever it is given.
func Markdown(c contracts.Conversation) string {
	var builder strings.Builder

	// Step 1: front matter (title and metadata block).
	writeHeader(&builder, c)

	// Step 2: each message as a section, role-prefixed.
	for _, m := range c.Messages {
		writeMessage(&builder, m)
	}

	return builder.String()
}

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

func writeBlock(builder *strings.Builder, b contracts.Block) {
	switch v := b.(type) {
	case contracts.TextBlock:
		builder.WriteString(v.Text)
		builder.WriteString("\n\n")
	case contracts.ThinkingBlock:
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
		fmt.Fprintf(builder, "_Unknown block kind `%s` (preserved as raw JSON below)_\n\n```json\n%s\n```\n\n",
			v.Kind, string(v.Raw))
	}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "(unknown)"
	}
	return t.Format(time.RFC3339)
}
