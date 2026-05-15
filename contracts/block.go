package contracts

import "encoding/json"

// Block is one piece of a Message. The Message.Blocks slice is the normalized
// form of provider-specific content arrays. Use a type switch to handle each
// concrete kind.
type Block interface {
	blockMarker()
}

// TextBlock is plain prose from user or assistant.
type TextBlock struct {
	Text string
}

// ThinkingBlock is the assistant's internal reasoning. Hidden by default in
// the UI; the user can opt to show it.
type ThinkingBlock struct {
	Text string
}

// ToolUseBlock is the assistant invoking a tool. Input is the raw JSON
// argument as the provider stored it.
type ToolUseBlock struct {
	Tool   string
	Input  json.RawMessage
	CallID string
}

// ToolResultBlock is the user-side return value for a previous ToolUseBlock,
// linked by CallID.
type ToolResultBlock struct {
	CallID  string
	Output  string
	IsError bool
}

// ImageBlock describes an image attached to a turn.
type ImageBlock struct {
	MIME            string
	PathOrInlineRef string
}

// UnknownBlock preserves provider content we do not recognize. The renderer
// shows it as "Unknown block · click to inspect", and the resilience contract
// requires we keep the raw JSON rather than dropping it.
type UnknownBlock struct {
	Kind string
	Raw  json.RawMessage
}

func (TextBlock) blockMarker()       {}
func (ThinkingBlock) blockMarker()   {}
func (ToolUseBlock) blockMarker()    {}
func (ToolResultBlock) blockMarker() {}
func (ImageBlock) blockMarker()      {}
func (UnknownBlock) blockMarker()    {}
