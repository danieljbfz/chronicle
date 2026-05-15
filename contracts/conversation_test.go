package contracts

import (
	"testing"
	"time"
)

func TestFirstUserPrompt_skipsMetaAndAssistant(t *testing.T) {
	c := Conversation{
		Messages: []Message{
			{Role: RoleAssistant, Blocks: []Block{TextBlock{Text: "hi"}}},
			{Role: RoleUser, IsMeta: true, Blocks: []Block{TextBlock{Text: "<command>/clear</command>"}}},
			{Role: RoleUser, Blocks: []Block{TextBlock{Text: "read the docs"}}},
			{Role: RoleUser, Blocks: []Block{TextBlock{Text: "second prompt"}}},
		},
	}
	got := c.FirstUserPrompt()
	if got != "read the docs" {
		t.Errorf("FirstUserPrompt() = %q, want %q", got, "read the docs")
	}
}

func TestIsAbandoned_emptySessionReturnsTrue(t *testing.T) {
	c := Conversation{
		Messages: []Message{
			{Role: RoleUser, IsMeta: true, Blocks: []Block{TextBlock{Text: "<command>/clear</command>"}}},
			{Role: RoleAssistant, Blocks: []Block{TextBlock{Text: "ok"}}},
		},
	}
	if !c.IsAbandoned() {
		t.Error("session with only meta + assistant should be abandoned")
	}
}

func TestIsAbandoned_realPromptReturnsFalse(t *testing.T) {
	c := Conversation{
		StartedAt: time.Now(),
		Messages: []Message{
			{Role: RoleUser, Blocks: []Block{TextBlock{Text: "hello"}}},
		},
	}
	if c.IsAbandoned() {
		t.Error("session with a real prompt should not be abandoned")
	}
}

func TestStorageVersion_IsKnown(t *testing.T) {
	cases := []struct {
		version string
		want    bool
	}{
		{"claude-1.0", true},
		{"copilot-3", true},
		{"unknown", false},
		{"", false},
	}
	for _, tc := range cases {
		got := StorageVersion{Version: tc.version}.IsKnown()
		if got != tc.want {
			t.Errorf("IsKnown(%q) = %v, want %v", tc.version, got, tc.want)
		}
	}
}
