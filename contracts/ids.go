// Package contracts defines the normalized domain types that every higher
// layer of chronicle speaks. Adapters translate provider-specific shapes
// into these. Steps, composition, and entrypoints know nothing about
// provider-specific schemas.
package contracts

// ProjectID identifies a project within a provider. It is opaque to the UI;
// adapters define their own format (Claude uses the encoded cwd, Copilot
// uses the workspace hash). The UI shows Project.DisplayName instead.
type ProjectID string

// SessionID identifies a single session within a project.
type SessionID string

// MessageID identifies a message within a session. For storage formats that
// do not assign IDs (Copilot's flat list), adapters synthesize stable IDs
// from the record index.
type MessageID string

// Role is the speaker of a Message.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
)
