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

// FirstUserPrompt returns the text of the first real user message in
// the conversation, or the empty string if no real user message exists.
// "Real" means two things: the message must come from the user, and it
// must not be a meta record. The meta filter is what skips the
// synthetic slash-command echoes that Claude Code writes whenever the
// user runs commands like /clear inside a session. Without that filter,
// every Claude session would look like it began with the literal text
// "<command-name>/clear</command-name>", which is not what any reader
// wants to see in a transcript.
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

// IsAbandoned reports whether the session contains zero real user
// prompts. The cleanup feature in a later plan uses this check to find
// the sessions that the user opened by accident, ran a command or two
// in, and then never returned to. On the contributor's own machine those
// sessions account for nearly one in five of every session file on
// disk, with each one taking up around eighteen kilobytes of
// session-start hooks and zero actual conversation.
func (c Conversation) IsAbandoned() bool {
	return c.FirstUserPrompt() == ""
}
