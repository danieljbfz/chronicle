package contracts

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

// IsAbandoned reports whether the session has zero non-meta user prompts.
// This is the criterion the cleanup feature uses in Plan C.
func (c Conversation) IsAbandoned() bool {
	return c.FirstUserPrompt() == ""
}
