package contracts

import (
	"encoding/json"
	"testing"
)

// TestBlockMarker is really a compile-time check disguised as a
// runtime test. The body assigns each concrete block type into a
// variable of the Block interface type, which only succeeds when the
// concrete type satisfies the interface. If we ever forget to declare
// blockMarker on a new block type, this file stops compiling and the
// failure points at the exact line that broke.
func TestBlockMarker(t *testing.T) {
	var b Block
	b = TextBlock{Text: "hello"}
	b = ThinkingBlock{Text: "musing"}
	b = ToolUseBlock{Tool: "Bash", Input: json.RawMessage(`{}`), CallID: "1"}
	b = ToolResultBlock{CallID: "1", Output: "ok"}
	b = ImageBlock{MIME: "image/png"}
	b = UnknownBlock{Kind: "weird", Raw: json.RawMessage(`null`)}
	_ = b
}

// TestRoleConstants pins the string values of the Role constants. If
// anyone ever changes them, the tests catch the change immediately.
// The values are visible to humans (they appear in JSON exports and
// log lines), so changing them silently would be a small breaking
// change.
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
