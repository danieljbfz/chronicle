// Package contracts holds the shared types that the rest of chronicle
// uses to talk about conversations, sessions, and projects. Adapters
// translate the messy, provider-specific files on disk into these
// types, and everything above the adapter layer only ever works with
// what is defined here. That keeps the user interface, the export
// commands, and the cleanup logic free of any knowledge about Claude's
// JSONL files or Copilot's event log.
package contracts

// ProjectID is the internal identifier for a project. We do not show it
// to the user, because each adapter chooses its own format and the
// formats are not pretty. The Claude adapter uses the encoded current
// working directory, and the Copilot adapter uses the workspace hash
// that VS Code generates. The user always sees Project.DisplayName
// instead, which is the friendly name we decode from the identifier.
type ProjectID string

// SessionID is the identifier the upstream tool uses for one session.
// In both Claude and Copilot's case it happens to be a UUID, and it is
// also the name of the file the session lives in on disk.
type SessionID string

// MessageID is the identifier for one message inside a session. When
// the upstream storage already assigns identifiers, like Claude's uuid
// field, we pass them through. When it does not, like with Copilot's
// flat list of requests, the adapter makes them up from the position
// of the message in the file. Either way, every message in every
// Conversation has a unique identifier we can refer to.
type MessageID string

// Role names the speaker of a Message. Making it a named string type
// instead of a plain string means the compiler will catch a typo at a
// call site that tries to pass, say, "asistant" where a Role is
// expected. Without the named type, the typo would compile and only
// surface as a rendering bug when someone ran the code.
type Role string

// The three Role values chronicle understands. If a future version of
// any upstream tool starts emitting a fourth role, the adapter for that
// tool should map it to RoleSystem and keep the original text as an
// UnknownBlock so the reader still sees that something happened. That
// is the resilience contract talking, and it applies everywhere we
// might encounter unfamiliar data.
const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
)
