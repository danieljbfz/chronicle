package contracts

import "time"

// Project groups the sessions that belong to one working directory or
// workspace from the upstream tool's point of view. The DisplayName is
// the human-readable project name the user sees in the listing, and the
// Path is the absolute filesystem path the project was decoded from
// when that information is available. SessionCount and SizeBytes are
// computed at listing time so the user interface can show useful
// summary information without loading every session.
type Project struct {
	ID           ProjectID
	DisplayName  string
	Path         string
	SessionCount int
	SizeBytes    int64
}

// SessionSummary is the cheap-to-compute view of a session. The listing
// pages of the user interface use these. Only the preview pane and the
// export commands ever pay the cost of loading the full Conversation
// through Provider.ReadSession. The SizeBytes field includes the
// session's sibling artifacts on disk, like the file-history backups
// Claude Code writes alongside the JSONL, so the cleanup commands can
// show an accurate disk-reclaimable estimate without re-walking the
// tree at confirmation time.
//
// Model is the model identifier each adapter pulls from
// its native session metadata. The exact shape varies per
// provider: Claude records per-message models and the
// adapter reports the most-frequent value, copilot-chat
// records a per-session selection on inputState, and
// copilot-agent records a single selectedModel in its
// session.start event. An adapter that cannot determine
// the model leaves the field empty, and the stats
// renderer groups empty values under "(unknown)".
type SessionSummary struct {
	ID           SessionID
	Project      ProjectID
	StartedAt    time.Time
	LastActive   time.Time
	Title        string
	TurnCount    int
	SizeBytes    int64
	Model        string
	Capabilities Capabilities
	Source       StorageVersion
}
