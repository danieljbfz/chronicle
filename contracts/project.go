package contracts

import "time"

// Project groups sessions belonging to one working directory or workspace.
type Project struct {
	ID           ProjectID
	DisplayName  string // human-readable, decoded from path or workspace.json
	Path         string // absolute filesystem path when known
	SessionCount int
	SizeBytes    int64
}

// SessionSummary is the cheap-to-compute view of a session. Listing pages
// use these; only the preview pane and export commands load the full
// Conversation via Provider.ReadSession.
type SessionSummary struct {
	ID           SessionID
	Project      ProjectID
	StartedAt    time.Time
	LastActive   time.Time
	Title        string // first user prompt or custom title
	TurnCount    int
	SizeBytes    int64 // includes sibling artifacts
	Capabilities Capabilities
	Source       StorageVersion
}
