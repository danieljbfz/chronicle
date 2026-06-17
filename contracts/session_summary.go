package contracts

import "time"

// SessionSummary is the cheap-to-compute view of a session. The listing
// pages of the user interface use these. Only the preview pane and the
// export commands ever pay the cost of loading the full Conversation
// through Provider.ReadSession. SizeBytes is the on-disk size of the
// session's primary file — the JSONL transcript for Claude and Copilot
// Chat, the events log for the Copilot agent runtime — which the stats
// and listing surfaces sum into a disk-usage readout.
//
// The json tags exist because composition persists these summaries to its
// listing cache. They follow the project's snake_case convention so the
// cache file reads the same way as the trash manifest and the list
// command's output.
//
// Model is the model identifier each adapter pulls from its
// native session metadata. The exact shape varies per
// provider. Claude records per-message models, and the
// adapter reports the most-frequent value. The copilot-chat
// adapter records a per-session selection on inputState.
// The copilot-agent adapter records a single selectedModel
// in its session.start event. An adapter that cannot
// determine the model leaves the field empty, and the stats
// renderer groups empty values under "(unknown)".
type SessionSummary struct {
	ID           SessionID      `json:"id"`
	Project      ProjectID      `json:"project"`
	StartedAt    time.Time      `json:"started_at"`
	LastActive   time.Time      `json:"last_active"`
	Title        string         `json:"title"`
	TurnCount    int            `json:"turn_count"`
	SizeBytes    int64          `json:"size_bytes"`
	Model        string         `json:"model"`
	Capabilities Capabilities   `json:"capabilities"`
	Source       StorageVersion `json:"source"`
}

// NewSessionSummary builds the listing summary for one parsed session.
// The ref carries the on-disk size and the identifiers a summary needs
// without parsing. The conversation carries the timestamps, the title
// cascade, the turn count, and the model. The version carries the
// capabilities and storage source the adapter detected. Every adapter
// funnels through this one constructor, so the mapping from a parsed
// conversation to a listing summary lives in a single place rather than
// being repeated per adapter.
func NewSessionSummary(ref SessionRef, conv Conversation, version StorageVersion) SessionSummary {
	return SessionSummary{
		ID:           ref.ID,
		Project:      ref.Project,
		StartedAt:    conv.StartedAt,
		LastActive:   conv.EndedAt,
		Title:        conv.ListingTitle(),
		TurnCount:    len(conv.Messages),
		SizeBytes:    ref.SizeBytes,
		Model:        conv.Model,
		Capabilities: version.Capabilities,
		Source:       version,
	}
}
