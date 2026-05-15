package contracts

import (
	"testing"
	"time"
)

// TestFirstUserPrompt_skipsMetaAndAssistant proves the two filtering
// rules together: the function ignores assistant messages, and it
// ignores user messages flagged as meta. The third user message in
// the fixture is the first one that should match, so the test pins
// that expectation. If either rule ever drifts, the wrong text comes
// back and the test fails.
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

// TestIsAbandoned_emptySessionReturnsTrue confirms the cleanup
// criterion: a session with only meta records and assistant messages
// counts as abandoned, because there is no real human input to
// preserve. This is the shape of session that the cleanup feature
// will surface for one-key removal in a later plan.
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

// TestIsAbandoned_realPromptReturnsFalse is the negative case. A
// session that has even one real user prompt is not abandoned and
// must not show up in the cleanup view. The non-zero StartedAt is
// there to make the fixture look like a session that genuinely ran,
// even though the predicate does not actually look at the timestamp.
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

// TestStorageVersion_IsKnown pins the predicate's behaviour for the
// values that actually appear in practice. A known version like
// "claude-1.0" or "copilot-3" is known; the empty string and the
// literal "unknown" are not. Everything in chronicle that gates
// destructive operations on a known fingerprint depends on this
// function returning the right answer.
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
