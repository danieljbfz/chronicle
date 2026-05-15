package contracts

import (
	"encoding/json"
	"testing"
)

// The package-level lines below are compile-time checks that every
// concrete block type satisfies the Block interface. Go evaluates
// `var _ Block = ...` at build time only, so these declarations
// have no runtime cost. If anyone removes the blockMarker method
// from one of these types, the build fails on the matching line
// here, with an error that names the type and the missing method.
//
// We use this pattern instead of an assignment loop inside a Test
// function because static analyzers correctly flag the assignment
// loop as ineffectual: the variable is never read. The package-
// level form makes the intent explicit and survives lint cleanly.
var (
	_ Block = TextBlock{}
	_ Block = ThinkingBlock{}
	_ Block = ToolUseBlock{}
	_ Block = ToolResultBlock{}
	_ Block = ImageBlock{}
	_ Block = UnknownBlock{}
)

// TestBlockFields confirms each concrete block type holds the
// fields its name promises. The test acts as a regression net so
// no future refactor accidentally renames a field on, say,
// ToolUseBlock without anyone noticing.
func TestBlockFields(t *testing.T) {
	if (TextBlock{Text: "hi"}).Text != "hi" {
		t.Error("TextBlock.Text round-trip failed")
	}
	if (ThinkingBlock{Text: "musing"}).Text != "musing" {
		t.Error("ThinkingBlock.Text round-trip failed")
	}
	tool := ToolUseBlock{Tool: "Bash", Input: json.RawMessage(`{}`), CallID: "1"}
	if tool.Tool != "Bash" || tool.CallID != "1" {
		t.Error("ToolUseBlock fields wrong")
	}
	result := ToolResultBlock{CallID: "1", Output: "ok", IsError: true}
	if result.Output != "ok" || !result.IsError {
		t.Error("ToolResultBlock fields wrong")
	}
	image := ImageBlock{MIME: "image/png", PathOrInlineRef: "abc"}
	if image.MIME != "image/png" {
		t.Error("ImageBlock fields wrong")
	}
	unknown := UnknownBlock{Kind: "weird", Raw: json.RawMessage(`null`)}
	if unknown.Kind != "weird" {
		t.Error("UnknownBlock fields wrong")
	}
}

// TestRoleConstants pins the string values of the Role constants.
// If anyone ever changes them, the tests catch the change
// immediately. The values are visible to humans (they appear in
// JSON exports and log lines), so changing them silently would be
// a small breaking change.
func TestRoleConstants(t *testing.T) {
	if RoleUser != "user" {
		t.Errorf("RoleUser = %q, want %q", RoleUser, "user")
	}
	if RoleAssistant != "assistant" {
		t.Errorf("RoleAssistant = %q, want %q", RoleAssistant, "assistant")
	}
	if RoleSystem != "system" {
		t.Errorf("RoleSystem = %q, want %q", RoleSystem, "system")
	}
}
