package contracts

import "time"

// Conversation is a normalized session, ready for any layer above
// contracts to use. Adapters produce these by folding the records they
// read from disk into ordered Message values, and they copy the
// Capabilities and Source fields across from the StorageVersion they
// produced during detection. Carrying the capabilities on the
// conversation itself is intentional: the renderer can decide whether
// to show, for example, a thread-tree view by looking at the
// conversation alone, without having to ask the adapter again.
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

// FirstUserPrompt returns the text of the first real user message
// in the conversation. It returns the empty string when there is
// no real user message at all.
//
// "Real" means two things. The message has to come from the user,
// not the assistant. And it has to be a normal message, not a meta
// record. Meta records are the synthetic ones Claude Code writes
// when the user runs commands like /clear inside a session. If we
// did not skip them, every Claude session would look like it began
// with the literal text "<command-name>/clear</command-name>",
// which is never what the reader wants to see at the top of a
// transcript.
func (c Conversation) FirstUserPrompt() string {
	for _, m := range c.Messages {
		if m.Role != RoleUser || m.IsMeta {
			continue
		}
		for _, b := range m.Blocks {
			if t, ok := b.(TextBlock); ok && t.Text != "" {
				return t.Text
			}
		}
	}
	return ""
}

// IsAbandoned reports whether the session has zero real user
// prompts. The cleanup feature uses this check to find the
// sessions the user opened by accident, ran a command or two in,
// and then never returned to.
//
// On the author's own machine, sessions like that account for
// nearly one in five of every session file on disk. Each one takes
// up around eighteen kilobytes of session-start hooks and zero
// actual conversation, so cleaning them up is the easiest disk win
// chronicle can offer.
func (c Conversation) IsAbandoned() bool {
	return c.FirstUserPrompt() == ""
}
