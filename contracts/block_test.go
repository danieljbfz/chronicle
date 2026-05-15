package contracts

import (
	"encoding/json"
	"testing"
)

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
