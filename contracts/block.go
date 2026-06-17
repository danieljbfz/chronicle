package contracts

import "encoding/json"

// Block is one piece of content inside a Message. A single
// message can hold any mix of these in any order, and the
// renderer walks the slice to produce the final output. When
// a piece of code needs to react differently to each kind of
// block, it uses a type switch. The Markdown renderer in the
// steps package is a good example of that pattern in use.
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
//
// Claude stores reasoning two ways. Most blocks carry the reasoning as
// readable text in Text. Some carry an empty Text and only an encrypted
// signature, the shape Claude writes when extended thinking runs with
// its display set to "omitted": the reasoning did happen, but the
// readable text is not on disk — the full reasoning is encrypted in the
// signature and only Anthropic's API can read it. Encrypted marks that
// second shape so the renderer can show an honest marker rather than an
// empty quote. A block with neither readable text nor a signature
// represents no reasoning at all and is dropped at parse time.
type ThinkingBlock struct {
	Text string

	// Encrypted is true when the readable reasoning text was omitted
	// on disk and only the encrypted signature remains. Text is empty
	// in that case.
	Encrypted bool
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

// ToolResultBlock carries what the tool produced when it ran
// in response to a previous ToolUseBlock. The CallID links
// the result back to that originating call, the IsError flag
// tells us whether the tool reported a failure, and the
// Output field carries the textual content. We flatten the
// output to a single string here, because the upstream tool
// can store it either as a bare string or as an array of
// typed parts, and the rest of chronicle does not need to
// care about the difference.
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

// DocumentBlock describes a document, such as a PDF, that was
// attached to a turn. It is the document counterpart of ImageBlock
// and behaves the same way. The block records only the MIME type and
// a reference (a byte count for inline base64, otherwise the source
// kind), never the document bytes, so a base64 payload never lands in
// the rendered transcript. That reference is enough information for a
// future version of the user interface to locate the document when it
// needs to render it.
type DocumentBlock struct {
	MIME            string
	PathOrInlineRef string
}

// FileContextBlock holds a file's content that the editor attached to
// a turn as context for the assistant, rather than content the user
// typed or the assistant read through a tool. The parser gives these
// the system role for that reason — the user did not write them.
// Claude records this three ways, and we fold all of them into this
// one block because they are the same thing to a reader: a file path
// and the content the assistant saw. The three sources are a whole
// file attached to the turn (the "file" attachment), a snapshot of a
// file open in the editor (the "edited_text_file" attachment), and a
// range highlighted in the editor (the "selected_lines_in_ide"
// attachment). Content is the text as Claude stored it, often with
// leading line numbers. This is the file state the assistant saw, and
// it usually appears nowhere else in the transcript, so dropping it
// would leave a reply with no visible sign of what it was reacting
// to. The HideFileContext filter flag drops these.
type FileContextBlock struct {
	Path    string
	Content string
}

// AwaySummaryBlock holds a session summary Claude writes when the
// user steps away mid-session, capturing the goal, current status,
// and next steps. Claude stores these as system records with
// subtype away_summary and, notably, marks them isMeta:false — it
// does not consider them bookkeeping. They are some of the most
// information-dense prose in a session, so we surface them. The
// HideAwaySummaries filter flag drops them.
type AwaySummaryBlock struct {
	Text string
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
func (TextBlock) blockMarker()        {}
func (ThinkingBlock) blockMarker()    {}
func (ToolUseBlock) blockMarker()     {}
func (ToolResultBlock) blockMarker()  {}
func (ImageBlock) blockMarker()       {}
func (DocumentBlock) blockMarker()    {}
func (FileContextBlock) blockMarker() {}
func (AwaySummaryBlock) blockMarker() {}
func (UnknownBlock) blockMarker()     {}
