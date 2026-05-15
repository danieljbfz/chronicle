package steps

import (
	"strings"
	"testing"

	"github.com/danieljbfz/chronicle/contracts"
)

// makeConv is a small builder for test conversations. The
// tests below build conversations from short text strings,
// and this helper keeps them readable by handling the
// boilerplate of wrapping each string in a TextBlock and a
// Message.
func makeConv(messages ...struct {
	role contracts.Role
	text string
}) contracts.Conversation {
	var msgs []contracts.Message
	for _, m := range messages {
		msgs = append(msgs, contracts.Message{
			Role:   m.role,
			Blocks: []contracts.Block{contracts.TextBlock{Text: m.text}},
		})
	}
	return contracts.Conversation{Messages: msgs}
}

// TestMatch_findsCaseInsensitiveSubstring is the happy path.
// A query "go" matches "Go" in the user message and "GoLang"
// in the assistant message, because the default is case
// folding. Two matches across two messages, both returned.
func TestMatch_findsCaseInsensitiveSubstring(t *testing.T) {
	conv := makeConv(
		struct {
			role contracts.Role
			text string
		}{contracts.RoleUser, "How do I read a file in Go?"},
		struct {
			role contracts.Role
			text string
		}{contracts.RoleAssistant, "GoLang has os.ReadFile."},
	)
	snippets := Match(conv, "go", SearchOptions{})
	if len(snippets) != 2 {
		t.Fatalf("snippets = %d, want 2", len(snippets))
	}
	if snippets[0].Role != contracts.RoleUser {
		t.Errorf("first snippet role = %q, want user", snippets[0].Role)
	}
	if snippets[1].Role != contracts.RoleAssistant {
		t.Errorf("second snippet role = %q, want assistant", snippets[1].Role)
	}
}

// TestMatch_emptyQueryReturnsNothing pins the empty-query
// contract. An empty query at the CLI would otherwise match
// every byte position, which would surface every session as
// a hit and overwhelm the output.
func TestMatch_emptyQueryReturnsNothing(t *testing.T) {
	conv := makeConv(struct {
		role contracts.Role
		text string
	}{contracts.RoleUser, "anything"})
	snippets := Match(conv, "", SearchOptions{})
	if len(snippets) != 0 {
		t.Errorf("empty query returned %d snippets, want 0", len(snippets))
	}
}

// TestMatch_caseSensitiveDoesNotFoldCase confirms the opt-in
// flag works. With CaseSensitive=true the same conversation
// returns one match (the exact "Go") instead of two.
func TestMatch_caseSensitiveDoesNotFoldCase(t *testing.T) {
	conv := makeConv(
		struct {
			role contracts.Role
			text string
		}{contracts.RoleUser, "How do I read a file in Go?"},
		struct {
			role contracts.Role
			text string
		}{contracts.RoleAssistant, "golang has os.ReadFile."},
	)
	snippets := Match(conv, "Go", SearchOptions{CaseSensitive: true})
	if len(snippets) != 1 {
		t.Errorf("case-sensitive snippets = %d, want 1", len(snippets))
	}
}

// TestMatch_skipsMetaAndSidechainMessages confirms the
// noise-filter. Slash-command echoes and sub-agent traffic
// are not what the user wants to search through, and the
// function drops them before the substring check runs.
func TestMatch_skipsMetaAndSidechainMessages(t *testing.T) {
	conv := contracts.Conversation{
		Messages: []contracts.Message{
			{Role: contracts.RoleUser, IsMeta: true, Blocks: []contracts.Block{contracts.TextBlock{Text: "find this if you can"}}},
			{Role: contracts.RoleUser, IsSidechain: true, Blocks: []contracts.Block{contracts.TextBlock{Text: "find this if you can"}}},
			{Role: contracts.RoleUser, Blocks: []contracts.Block{contracts.TextBlock{Text: "real prompt, find this if you can"}}},
		},
	}
	snippets := Match(conv, "find this", SearchOptions{})
	if len(snippets) != 1 {
		t.Errorf("snippets = %d, want 1 (only the real prompt)", len(snippets))
	}
}

// TestMatch_skipsToolAndThinkingBlocks confirms the function
// only walks TextBlock content. A user wants to find
// sessions by what they discussed, not by what tool calls or
// thinking blocks happened to include the query.
func TestMatch_skipsToolAndThinkingBlocks(t *testing.T) {
	conv := contracts.Conversation{
		Messages: []contracts.Message{
			{Role: contracts.RoleAssistant, Blocks: []contracts.Block{
				contracts.ThinkingBlock{Text: "I should grep for chronicle"},
				contracts.ToolUseBlock{Tool: "Grep", CallID: "1", Input: []byte(`{"q":"chronicle"}`)},
				contracts.TextBlock{Text: "no match in here"},
			}},
		},
	}
	snippets := Match(conv, "chronicle", SearchOptions{})
	if len(snippets) != 0 {
		t.Errorf("snippets = %d, want 0 (only text blocks count)", len(snippets))
	}
}

// TestMatch_capsSnippetsPerSession pins the limit option.
// When MaxSnippetsPerSession is set to 2 and the query
// matches three times in one conversation, only the first
// two snippets come back.
func TestMatch_capsSnippetsPerSession(t *testing.T) {
	conv := makeConv(struct {
		role contracts.Role
		text string
	}{contracts.RoleUser, "go go go go go"})
	snippets := Match(conv, "go", SearchOptions{MaxSnippetsPerSession: 2})
	if len(snippets) != 2 {
		t.Errorf("snippets = %d, want 2 (cap)", len(snippets))
	}
}

// TestMatch_snippetIncludesContextAroundMatch confirms the
// extraction window. We use a long enough input that the
// extracted snippet will need both leading and trailing
// ellipses, and we check that the match itself is still in
// the middle of the snippet.
func TestMatch_snippetIncludesContextAroundMatch(t *testing.T) {
	long := strings.Repeat("padding ", 30) + "needle " + strings.Repeat("padding ", 30)
	conv := makeConv(struct {
		role contracts.Role
		text string
	}{contracts.RoleUser, long})
	snippets := Match(conv, "needle", SearchOptions{})
	if len(snippets) != 1 {
		t.Fatalf("snippets = %d, want 1", len(snippets))
	}
	if !strings.Contains(snippets[0].Text, "needle") {
		t.Errorf("snippet should contain the match, got: %q", snippets[0].Text)
	}
	if !strings.HasPrefix(snippets[0].Text, "...") {
		t.Error("snippet should start with ellipsis for a deep match")
	}
	if !strings.HasSuffix(snippets[0].Text, "...") {
		t.Error("snippet should end with ellipsis for a deep match")
	}
}

// TestMatch_snippetHasNoLeadingEllipsisAtStart confirms the
// edge-of-text case. When the match begins at offset zero,
// there is no preceding context, so the snippet starts with
// the match itself instead of "...".
func TestMatch_snippetHasNoLeadingEllipsisAtStart(t *testing.T) {
	conv := makeConv(struct {
		role contracts.Role
		text string
	}{contracts.RoleUser, "needle and the rest of the message"})
	snippets := Match(conv, "needle", SearchOptions{})
	if len(snippets) != 1 {
		t.Fatalf("snippets = %d, want 1", len(snippets))
	}
	if strings.HasPrefix(snippets[0].Text, "...") {
		t.Errorf("snippet at start should not have leading ellipsis, got: %q", snippets[0].Text)
	}
}
