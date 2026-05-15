package contracts

import "time"

// Message is one turn in a Conversation. The Blocks slice carries the
// content; the rest is metadata used for filtering and threading.
type Message struct {
	ID          MessageID
	ParentID    MessageID // empty for the root
	Role        Role
	Timestamp   time.Time
	Blocks      []Block
	IsMeta      bool   // synthetic record (slash-command echo, hook output)
	IsSidechain bool   // sub-agent traffic
	Model       string // empty when unknown
}
