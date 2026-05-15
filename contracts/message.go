package contracts

import "time"

// Message is one turn in a Conversation. The Blocks slice holds the
// content the renderer will display, and the rest of the fields are
// metadata the filter step uses to decide which messages survive a
// pass. The IsMeta flag marks synthetic records the upstream tool
// inserts for its own bookkeeping, like slash-command echoes and hook
// outputs. The IsSidechain flag marks traffic from sub-agents, which
// the user almost always wants hidden by default. The Model field
// records which underlying model produced an assistant turn when the
// upstream storage made that information available, and it stays empty
// otherwise.
type Message struct {
	ID          MessageID
	ParentID    MessageID
	Role        Role
	Timestamp   time.Time
	Blocks      []Block
	IsMeta      bool
	IsSidechain bool
	Model       string
}
