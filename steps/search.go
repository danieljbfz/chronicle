package steps

import (
	"strings"

	"github.com/danieljbfz/chronicle/contracts"
)

// SearchSnippet is one match inside a conversation. It holds
// the surrounding text the user will read to decide whether
// the session is the one they were looking for. The window is
// short enough to fit on one terminal line but wide enough to
// give a sense of context.
//
// Role tells the user whether the match was in something they
// said or in something the assistant replied. The CLI uses
// this for the inline label like "you: ..." or "assistant:
// ...". The Position field is a byte offset into the original
// joined text, only used by tests to confirm matches landed
// where expected.
type SearchSnippet struct {
	Role     contracts.Role
	Text     string
	Position int
}

// SearchOptions controls how Match looks at a conversation.
// Every field has a sensible default at the zero value, so a
// bare SearchOptions{} works for most callers. The caller
// only sets a field when they want non-default behaviour.
type SearchOptions struct {
	// CaseSensitive controls whether the query matches the
	// text exactly or with case folded. The default (false)
	// folds case, because the user's typical mental model is
	// "I remember discussing X" and they do not usually
	// remember the exact capitalisation.
	CaseSensitive bool

	// MaxSnippetsPerSession caps how many matches Match
	// returns for one conversation. The default of zero
	// means "no cap", which is fine for callers that want
	// every match. The CLI passes a small cap (typically
	// three) so the listing output stays readable.
	MaxSnippetsPerSession int

	// SnippetContext is how many characters of surrounding
	// text Match includes on each side of the match. The
	// default of zero falls back to the constant defined in
	// this file, which is tuned to fit comfortably on one
	// terminal line.
	SnippetContext int
}

// defaultSnippetContext is the character window on each side
// of a match. Sixty on each side plus the match itself gives
// roughly 140 to 160 characters total, which fits in one
// terminal line with the role prefix.
const defaultSnippetContext = 60

// Match returns every place inside the conversation where
// the query string appears in a user or assistant text
// block. We deliberately ignore thinking blocks, tool calls,
// tool results, and meta messages: the user wants to find
// sessions by what they discussed, not by what the assistant
// thought to itself or what shell command it ran.
//
// An empty query returns no snippets and no error. The
// caller filters those out at the CLI level so a typo does
// not surface every session as a match.
//
// The function is pure. It reads only what it is given and
// does not consult the filesystem, the clock, or any global
// state.
func Match(conv contracts.Conversation, query string, opts SearchOptions) []SearchSnippet {
	if query == "" {
		return nil
	}
	context := opts.SnippetContext
	if context == 0 {
		context = defaultSnippetContext
	}

	needle := query
	if !opts.CaseSensitive {
		needle = strings.ToLower(query)
	}

	var snippets []SearchSnippet
	for _, message := range conv.Messages {
		if message.IsMeta || message.IsSidechain {
			continue
		}
		if message.Role != contracts.RoleUser && message.Role != contracts.RoleAssistant {
			continue
		}
		text := joinTextBlocks(message.Blocks)
		if text == "" {
			continue
		}
		haystack := text
		if !opts.CaseSensitive {
			haystack = strings.ToLower(text)
		}
		for _, pos := range findAll(haystack, needle) {
			snippets = append(snippets, SearchSnippet{
				Role:     message.Role,
				Text:     extractWindow(text, pos, len(query), context),
				Position: pos,
			})
			if opts.MaxSnippetsPerSession > 0 && len(snippets) >= opts.MaxSnippetsPerSession {
				return snippets
			}
		}
	}
	return snippets
}

// joinTextBlocks pulls every TextBlock out of a message and
// joins them with single newlines. We only walk TextBlock
// content because the other block kinds (thinking, tool use,
// tool result, image, unknown) are not what the user wants
// to search through. The user's mental model is conversation
// text, and that is exactly what TextBlock holds.
func joinTextBlocks(blocks []contracts.Block) string {
	var parts []string
	for _, block := range blocks {
		if text, ok := block.(contracts.TextBlock); ok && text.Text != "" {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// findAll returns the byte offsets of every non-overlapping
// occurrence of needle inside haystack. We use it instead of
// strings.Index in a loop because we want every match, not
// just the first, and we want to walk past each match by
// len(needle) so overlapping needles do not count twice.
func findAll(haystack, needle string) []int {
	if needle == "" {
		return nil
	}
	var positions []int
	offset := 0
	for {
		idx := strings.Index(haystack[offset:], needle)
		if idx < 0 {
			return positions
		}
		positions = append(positions, offset+idx)
		offset += idx + len(needle)
	}
}

// extractWindow pulls the snippet around one match. The
// window is the match itself plus context characters on
// each side, clipped to the bounds of the source text. An
// ellipsis prefix or suffix marks the cases where we trimmed
// content the snippet did not cover.
//
// The function counts in bytes, not runes. For ASCII text
// the two are identical; for text with multi-byte runes the
// snippet may start or end inside a rune, which produces
// the Unicode replacement character on display. We accept
// that trade-off because the snippet is short and the
// alternative (rune-aware windowing) is meaningfully more
// code for a small visual win.
func extractWindow(text string, matchStart, matchLen, context int) string {
	start := matchStart - context
	if start < 0 {
		start = 0
	}
	end := matchStart + matchLen + context
	if end > len(text) {
		end = len(text)
	}
	snippet := text[start:end]
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(text) {
		snippet = snippet + "..."
	}
	snippet = strings.ReplaceAll(snippet, "\n", " ")
	return snippet
}
