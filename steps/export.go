package steps

// -----------------------------------------------------------------------
// Go concepts introduced in this file
// -----------------------------------------------------------------------
//
// 1. `strings.Builder`. The recommended way to build up a string from
//    many small pieces. Concatenating with `+` allocates a new string
//    each time — fine for two or three pieces, ruinous for a thousand.
//    `strings.Builder` accumulates bytes in a growable buffer and
//    returns the final string with `.String()`. Reset between uses or
//    just declare a new one — they are cheap.
//
// 2. `fmt.Fprintf(w, format, args...)`. The "f" stands for *file* — it
//    writes formatted text to anything that satisfies `io.Writer`. A
//    `*strings.Builder` is an io.Writer, an `*os.File` is, a
//    `bytes.Buffer` is, an HTTP response writer is. This is the same
//    polymorphism we saw with `fs.FS`: code targets the interface, not
//    the concrete type.
//
//    Note the trio you will see across the standard library:
//        fmt.Printf(...)     -> writes to os.Stdout
//        fmt.Fprintf(w, ...) -> writes to the writer you pass
//        fmt.Sprintf(...)    -> returns a string
//
// 3. TYPE SWITCH WITH ASSIGNMENT. Same pattern as in filter.go. The
//    `switch v := b.(type)` form binds `v` to the concrete value
//    inside each case, so we can read its fields directly.
//
// 4. `for _, line := range strings.Split(s, "\n")` — splitting a string
//    on newlines and iterating each line. `strings.Split` returns a
//    `[]string`. The range loop is the standard iteration shape.

import (
	"fmt"
	"strings"
	"time"

	"github.com/danieljbfz/chronicle/contracts"
)

// Markdown renders a Conversation as a human-readable Markdown document.
// Apply Filter first if you want to omit tools or thinking; Markdown
// does not filter, it only renders whatever it is given.
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

// writeHeader emits a top-level Markdown heading plus a metadata
// blockquote. The functions below take `*strings.Builder` (a pointer)
// because they append to it — see the value-vs-pointer-receiver note in
// contracts/conversation.go.
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

// writeMessage emits a `## Role` heading and each block in order. The
// `switch m.Role { ... }` here is an ordinary (non-type) switch — it
// matches on the string value, not on a concrete type.
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

// writeBlock is a type switch (see concept #3 above). Each case receives
// the unwrapped concrete block value as `v` and can read its fields
// directly.
func writeBlock(builder *strings.Builder, b contracts.Block) {
	switch v := b.(type) {
	case contracts.TextBlock:
		builder.WriteString(v.Text)
		builder.WriteString("\n\n")
	case contracts.ThinkingBlock:
		// Render thinking as a Markdown blockquote so it visually
		// recedes from the actual answer.
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
		// The §6 resilience contract requires we keep unknown content
		// visible to the user. We label it and dump the raw JSON.
		fmt.Fprintf(builder, "_Unknown block kind `%s` (preserved as raw JSON below)_\n\n```json\n%s\n```\n\n",
			v.Kind, string(v.Raw))
	}
}

// formatTime returns an RFC 3339 timestamp, or "(unknown)" for the zero
// value. `time.Time{}.IsZero()` is the standard way to detect "no time
// set" — the zero value of time.Time is the year-1 epoch.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return "(unknown)"
	}
	return t.Format(time.RFC3339)
}
