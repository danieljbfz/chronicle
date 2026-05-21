package contracts

import (
	"strings"
	"testing"
	"time"
)

// TestListingTitle_prefersExplicitTitle pins the first rung of the
// cascade. When the upstream tool recorded a title on the
// conversation, ListingTitle returns it unchanged, regardless of
// what the messages contain.
func TestListingTitle_prefersExplicitTitle(t *testing.T) {
	c := Conversation{
		Title: "Refactor the user table",
		Messages: []Message{
			{Role: RoleUser, Blocks: []Block{TextBlock{Text: "ignored because Title wins"}}},
		},
	}
	got := c.ListingTitle()
	if got != "Refactor the user table" {
		t.Errorf("ListingTitle() = %q, want %q", got, "Refactor the user table")
	}
}

// TestListingTitle_fallsThroughToFirstUserPrompt covers the second
// rung. The Title field is empty but the conversation has a real
// user prompt, so the function returns the prompt text — exactly
// what FirstUserPrompt would.
func TestListingTitle_fallsThroughToFirstUserPrompt(t *testing.T) {
	c := Conversation{
		Messages: []Message{
			{Role: RoleUser, IsMeta: true, Blocks: []Block{TextBlock{Text: "<command-name>/clear</command-name>"}}},
			{Role: RoleUser, Blocks: []Block{TextBlock{Text: "read the docs and report back"}}},
		},
	}
	got := c.ListingTitle()
	if got != "read the docs and report back" {
		t.Errorf("ListingTitle() = %q, want the user prompt", got)
	}
}

// TestListingTitle_extractsSlashCommandName covers the third rung.
// When the only user content is a slash-command wrapper, the
// function returns the command name from inside the wrapper rather
// than the literal XML markup.
func TestListingTitle_extractsSlashCommandName(t *testing.T) {
	c := Conversation{
		Messages: []Message{
			{Role: RoleUser, IsMeta: true, Blocks: []Block{TextBlock{Text: "<command-name>/clear</command-name>"}}},
			{Role: RoleAssistant, Blocks: []Block{TextBlock{Text: "Cleared."}}},
		},
	}
	got := c.ListingTitle()
	if got != "/clear" {
		t.Errorf("ListingTitle() = %q, want /clear", got)
	}
}

// TestListingTitle_fallsThroughToFirstAssistantText covers the
// fourth rung. The conversation has no real user text and no
// recognisable slash command, but the assistant produced a reply.
// The reply becomes the listing's identity.
func TestListingTitle_fallsThroughToFirstAssistantText(t *testing.T) {
	c := Conversation{
		Messages: []Message{
			{Role: RoleUser, IsMeta: true, Blocks: []Block{TextBlock{Text: "tool-only first turn"}}},
			{Role: RoleAssistant, Blocks: []Block{TextBlock{Text: "I read the file and noticed three issues."}}},
		},
	}
	got := c.ListingTitle()
	if got != "I read the file and noticed three issues." {
		t.Errorf("ListingTitle() = %q, want the assistant text", got)
	}
}

// TestListingTitle_fallsThroughToFirstToolName covers the fifth
// rung. The session has no recognisable text from either side, but
// the assistant invoked a tool. The tool name is the next-best
// identity the cascade can produce.
func TestListingTitle_fallsThroughToFirstToolName(t *testing.T) {
	c := Conversation{
		Messages: []Message{
			{Role: RoleAssistant, Blocks: []Block{ToolUseBlock{Tool: "Bash"}}},
		},
	}
	got := c.ListingTitle()
	if got != "Bash" {
		t.Errorf("ListingTitle() = %q, want Bash", got)
	}
}

// TestListingTitle_syntheticFromStartedAt covers the sixth rung
// with a known StartedAt. The result names the session by its
// start date so the row carries identity a reader can sort.
func TestListingTitle_syntheticFromStartedAt(t *testing.T) {
	when := time.Date(2026, 5, 21, 9, 0, 0, 0, time.UTC)
	c := Conversation{StartedAt: when}
	got := c.ListingTitle()
	if got != "Session started 2026-05-21" {
		t.Errorf("ListingTitle() = %q, want %q", got, "Session started 2026-05-21")
	}
}

// TestListingTitle_syntheticFromSessionID covers the sixth rung
// with a zero StartedAt but a non-empty SessionID. The result
// names the session by its short id so the row still has identity.
func TestListingTitle_syntheticFromSessionID(t *testing.T) {
	c := Conversation{SessionID: SessionID("abcd1234-extra-suffix")}
	got := c.ListingTitle()
	if got != "Session abcd1234" {
		t.Errorf("ListingTitle() = %q, want %q", got, "Session abcd1234")
	}
}

// TestListingTitle_neverEmpty is the property test that guards the
// cascade's invariant: every reachable shape of Conversation
// returns a non-empty title. The cases below cover every place
// the cascade could in principle fall off.
func TestListingTitle_neverEmpty(t *testing.T) {
	cases := []struct {
		name string
		conv Conversation
	}{
		{"completely empty", Conversation{}},
		{"only meta user", Conversation{Messages: []Message{{Role: RoleUser, IsMeta: true, Blocks: []Block{TextBlock{Text: "noise"}}}}}},
		{"only assistant thinking", Conversation{Messages: []Message{{Role: RoleAssistant, Blocks: []Block{ThinkingBlock{Text: "hidden"}}}}}},
		{"only image", Conversation{Messages: []Message{{Role: RoleUser, Blocks: []Block{ImageBlock{MIME: "image/png"}}}}}},
		{"only tool result", Conversation{Messages: []Message{{Role: RoleUser, Blocks: []Block{ToolResultBlock{CallID: "x", Output: "y"}}}}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.conv.ListingTitle()
			if got == "" {
				t.Error("ListingTitle should never return the empty string")
			}
		})
	}
}

// TestListingTitle_clipsLongTitles confirms the function honours
// the rune budget. A very long first user prompt is clipped to
// maxListingTitleRunes runes with an ellipsis at the end so the
// listing surface receives a value the reader can scan.
func TestListingTitle_clipsLongTitles(t *testing.T) {
	long := strings.Repeat("a", maxListingTitleRunes+50)
	c := Conversation{
		Messages: []Message{{Role: RoleUser, Blocks: []Block{TextBlock{Text: long}}}},
	}
	got := c.ListingTitle()
	runes := []rune(got)
	if len(runes) != maxListingTitleRunes {
		t.Errorf("ListingTitle returned %d runes, want %d", len(runes), maxListingTitleRunes)
	}
	if runes[len(runes)-1] != '…' {
		t.Errorf("a clipped title should end with an ellipsis, got %q", string(runes[len(runes)-1]))
	}
}

// TestListingTitle_collapsesEmbeddedNewlines confirms a title that
// arrives with embedded line breaks comes out as a single line.
// The TUI's row delegate trusts the listing title to be a
// single-line value, so this invariant matters.
func TestListingTitle_collapsesEmbeddedNewlines(t *testing.T) {
	c := Conversation{
		Messages: []Message{{Role: RoleUser, Blocks: []Block{TextBlock{Text: "first line\n\nsecond\tline"}}}},
	}
	got := c.ListingTitle()
	if got != "first line second line" {
		t.Errorf("ListingTitle() = %q, want collapsed whitespace", got)
	}
	if strings.ContainsAny(got, "\n\r\t") {
		t.Errorf("ListingTitle output still carries a line-breaking character: %q", got)
	}
}
