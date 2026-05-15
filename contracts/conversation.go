package contracts

// -----------------------------------------------------------------------
// Go concepts introduced in this file
// -----------------------------------------------------------------------
//
// 1. METHOD RECEIVERS ON STRUCTS. `func (c Conversation) FirstUserPrompt()`
//    reads as: "this is a method on `Conversation`; inside the body, `c`
//    refers to the conversation the caller is operating on." Pick a
//    single-letter receiver tied to the type (`c` for Conversation, `m`
//    for Message) and stay consistent. See docs/naming-conventions.md.
//
// 2. VALUE vs POINTER RECEIVERS. `(c Conversation)` is a *value* receiver:
//    the method sees a copy of the struct and cannot change the original.
//    A *pointer* receiver `(c *Conversation)` would see the original.
//    Use value receivers when the method reads but does not mutate; use
//    pointer receivers for stateful types like `*App` (in composition).
//
// 3. THE `for _, x := range slice` LOOP. The standard way to iterate. The
//    blank identifier `_` says "I do not care about the index" — using
//    just `_, m := range c.Messages` is idiomatic when only the value
//    matters. See docs/go-primer.md §13.
//
// 4. TYPE ASSERTIONS. The expression `b.(TextBlock)` reads as: "I expect
//    the interface value `b` to actually hold a `TextBlock` at runtime;
//    extract it." The two-result form `t, ok := b.(TextBlock)` returns
//    `ok = true` when the assertion succeeds, `ok = false` (no panic)
//    when `b` holds some other concrete type. This is the standard way
//    to discriminate between the concrete types behind a Block interface.

import "time"

// Conversation is a normalized session. Adapters produce these by folding
// their provider-specific records into Message values. Capabilities are
// copied from the StorageVersion that produced this conversation so the
// UI does not need to re-query the adapter.
type Conversation struct {
	SessionID    SessionID
	Project      ProjectID
	StartedAt    time.Time
	EndedAt      time.Time
	Title        string
	Messages     []Message
	Capabilities Capabilities
	Source       StorageVersion
}

// FirstUserPrompt returns the text of the first non-meta user message, or
// the empty string if no such message exists (an abandoned session).
//
// The two filtering rules — "must be from the user" and "must not be a
// meta record" — together skip the slash-command echoes Claude Code emits
// when the user runs `/clear` and similar. Without the meta filter, every
// session would appear to "start" with `<command>/clear</command>`.
func (c Conversation) FirstUserPrompt() string {
	for _, m := range c.Messages {
		if m.Role != RoleUser || m.IsMeta {
			continue
		}
		// `b.(TextBlock)` is a type assertion (concept 4 above). It pulls
		// the concrete value out of the Block interface only if it really
		// is a TextBlock; otherwise ok is false and we move on.
		for _, b := range m.Blocks {
			if t, ok := b.(TextBlock); ok && t.Text != "" {
				return t.Text
			}
		}
	}
	return ""
}

// IsAbandoned reports whether the session has zero non-meta user prompts.
// This is the criterion the cleanup feature uses in Plan C — an abandoned
// session typically holds 18 KB of session-start hooks and zero actual
// conversation, and is safe to delete.
func (c Conversation) IsAbandoned() bool {
	return c.FirstUserPrompt() == ""
}
