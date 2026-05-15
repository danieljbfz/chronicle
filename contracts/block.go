package contracts

import "encoding/json"

// Block is one piece of content inside a Message. A single message can
// hold any mix of these in any order, and the renderer walks the slice
// in order to produce the final output. When a piece of code needs to
// react differently to each kind of block, it uses a type switch — the
// Markdown renderer in the steps package is a good example.
//
// The interface is satisfied by an unexported method called blockMarker
// that each concrete block declares with an empty body. The marker is
// there so the compiler can prove that only the types listed in this
// file count as Blocks. We could have left the interface empty, but an
// empty interface in Go accepts literally anything, which would make
// the type useless as a filter on what counts as block content.
type Block interface {
	blockMarker()
}

// TextBlock holds plain prose written by the user or returned by the
// assistant. This is the kind of content the renderer treats as the
// natural body of the conversation, and it is the only kind the user
// always sees no matter which filter toggles are on.
type TextBlock struct {
	Text string
}

// ThinkingBlock holds the assistant's internal reasoning, which Claude
// emits as a separate content kind alongside its visible reply. The
// renderer hides thinking by default, and the user can opt to show it.
// We never drop a thinking block at parse time, even though we hide it
// by default, because the resilience contract asks us to keep every
// piece of content the upstream tool wrote.
type ThinkingBlock struct {
	Text string
}

// ToolUseBlock represents the assistant invoking a tool such as Bash or
// Read. The Input field is the raw JSON the upstream tool stored. We
// keep it as a json.RawMessage and not a typed struct, because every
// tool has its own argument shape. Trying to model all of them would
// be a project of its own, and the renderer is fine with showing the
// raw JSON inside a fenced block for inspection.
type ToolUseBlock struct {
	Tool   string
	Input  json.RawMessage
	CallID string
}

// ToolResultBlock represents the value that came back from a
// previous ToolUseBlock after the tool finished running. The
// CallID links the result back to the originating call, the
// IsError flag tells us whether the tool reported a failure,
// and the Output field carries the textual content. We
// flatten the output to a single string here, because the
// upstream tool can store it either as a bare string or as
// an array of typed parts, and the rest of chronicle does
// not need to care about the difference.
type ToolResultBlock struct {
	CallID  string
	Output  string
	IsError bool
}

// ImageBlock describes an image that was attached to a turn.
// Chronicle does not copy image bytes out of the provider's
// storage today. The block only records the MIME type and a
// reference (either a filesystem path or an inline
// identifier), which is enough information for a future
// version of the user interface to locate the image when it
// needs to render it.
type ImageBlock struct {
	MIME            string
	PathOrInlineRef string
}

// UnknownBlock is what we produce when the upstream tool emits a content
// kind that no version of chronicle knows how to interpret. The renderer
// surfaces these as "Unknown block" entries that the reader can inspect,
// with the raw JSON included verbatim. This is how the resilience
// contract gets honored at parse time: when Claude or Copilot ships a
// brand-new content kind, chronicle still loads the file, still shows
// the conversation, and still surfaces the unfamiliar piece to the user
// instead of dropping it on the floor.
type UnknownBlock struct {
	Kind string
	Raw  json.RawMessage
}

// Each concrete block type satisfies the Block interface by carrying an
// empty blockMarker method. The compiler checks the relationship every
// time we assign a value of one of these types into a Block variable,
// and the marker itself adds nothing to the runtime cost.
func (TextBlock) blockMarker()       {}
func (ThinkingBlock) blockMarker()   {}
func (ToolUseBlock) blockMarker()    {}
func (ToolResultBlock) blockMarker() {}
func (ImageBlock) blockMarker()      {}
func (UnknownBlock) blockMarker()    {}
