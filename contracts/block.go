package contracts

// -----------------------------------------------------------------------
// Go concepts introduced in this file
// -----------------------------------------------------------------------
//
// 1. `import` BLOCK. The grouped `import (...)` form is the Go style for
//    pulling in standard-library or third-party packages. `encoding/json`
//    is part of the standard library; chronicle has no third-party
//    dependencies in this file.
//
// 2. INTERFACES. The `type Block interface { ... }` declaration defines a
//    contract: "any type that has a `blockMarker()` method satisfies the
//    Block interface." Crucially, no type ever *declares* "I implement
//    Block" — Go discovers it. If `TextBlock` happens to have the right
//    methods, the compiler accepts a `TextBlock` value wherever a `Block`
//    is expected. This is called "structural" or "implicit" interface
//    satisfaction and is the single most important Go design idea.
//
// 3. MARKER METHODS. `blockMarker()` does nothing; it exists only so the
//    compiler can match the interface. We use this pattern to fake "sum
//    types" (a `Block` is one of: text, thinking, tool-use, …) in a
//    language that has no built-in sum/union construct. Other languages
//    might write `enum Block { Text(...), Thinking(...), ... }`; Go uses
//    a tag-free interface with concrete struct types under it.
//
// 4. STRUCTS. `type TextBlock struct { Text string }` declares a record
//    with one field. Field names start with a capital letter when they
//    should be visible to other packages (`Text` is exported). A
//    lowercase field would be package-private.
//
// 5. `json.RawMessage` is a `[]byte` the JSON decoder leaves alone. We
//    use it to keep the original bytes of an unknown content block, which
//    the §6 resilience contract requires — we render the unknown back to
//    the user instead of dropping it.
//
// 6. METHOD RECEIVERS. `func (TextBlock) blockMarker() {}` reads as: "the
//    type `TextBlock` has a method called `blockMarker` that takes no
//    arguments and returns nothing." The parentheses before the method
//    name are the *receiver* — what `this`/`self` would be in other
//    languages. We omit the variable name (just `(TextBlock)` instead of
//    `(b TextBlock)`) because the method body does not use it.

import "encoding/json"

// Block is one piece of a Message. The Message.Blocks slice is the
// normalized form of provider-specific content arrays. Use a type switch
// (see steps/export.go writeBlock for an example) to handle each concrete
// kind.
type Block interface {
	blockMarker()
}

// TextBlock is plain prose from user or assistant.
type TextBlock struct {
	Text string
}

// ThinkingBlock is the assistant's internal reasoning. Hidden by default
// in the UI; the user can opt to show it via the `--show-thinking` flag.
type ThinkingBlock struct {
	Text string
}

// ToolUseBlock is the assistant invoking a tool. Input is the raw JSON
// argument as the provider stored it — we do not parse it because tool
// argument schemas vary per tool and per provider.
type ToolUseBlock struct {
	Tool   string
	Input  json.RawMessage
	CallID string
}

// ToolResultBlock is the user-side return value for a previous
// ToolUseBlock, linked by CallID.
type ToolResultBlock struct {
	CallID  string
	Output  string
	IsError bool
}

// ImageBlock describes an image attached to a turn. We keep a reference
// rather than the bytes themselves — chronicle never copies image data
// out of the provider's storage in v1.
type ImageBlock struct {
	MIME            string
	PathOrInlineRef string
}

// UnknownBlock preserves provider content we do not recognize. The
// renderer shows it as "Unknown block · click to inspect", and the
// resilience contract requires we keep the raw JSON rather than dropping
// it — when Claude or Copilot ship a new block kind, we surface it
// instead of losing it.
type UnknownBlock struct {
	Kind string
	Raw  json.RawMessage
}

// Each concrete block type implements the Block interface by carrying an
// empty `blockMarker()` method. The compiler sees this and accepts our
// blocks anywhere a Block is expected.
func (TextBlock) blockMarker()       {}
func (ThinkingBlock) blockMarker()   {}
func (ToolUseBlock) blockMarker()    {}
func (ToolResultBlock) blockMarker() {}
func (ImageBlock) blockMarker()      {}
func (UnknownBlock) blockMarker()    {}
