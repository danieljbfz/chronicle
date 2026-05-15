// Package contracts defines the normalized domain types that every higher
// layer of chronicle speaks. Adapters translate provider-specific shapes
// (Claude JSONL, Copilot event-log) into these. Steps, composition, and
// entrypoints know nothing about provider-specific schemas — they only
// know the types in this package.
//
// In hexagonal architecture terms, this package is the "port" side: a
// pure description of the domain with no I/O and no dependency on any
// outside system.
//
// -----------------------------------------------------------------------
// Go concepts introduced in this file
// -----------------------------------------------------------------------
//
// 1. THE `package` DECLARATION. Every Go file starts with `package <name>`.
//    The package name must match the folder name. Other packages reach
//    into this one via `contracts.ProjectID`, etc.
//
// 2. NAMED TYPES (`type Foo Bar`). The line `type ProjectID string` does
//    not create an alias — it creates a brand-new type whose underlying
//    representation happens to be a string. The Go compiler will refuse to
//    pass a plain `string` where a `ProjectID` is expected without an
//    explicit conversion `ProjectID(s)`. That is the whole point: the type
//    system catches "I passed a session id where a project id was expected"
//    at compile time, before any test runs. This is a cheap way to add
//    safety with zero runtime cost.
//
// 3. CONSTANTS WITH `iota`-LIKE NAMING. Below we declare `Role` constants
//    using a block `const (...)`. Go's standard library uses `MixedCaps`
//    for constants (e.g. `time.RFC3339`), not `UPPER_SNAKE_CASE`. We
//    follow that. Python's `UPPER_SNAKE_CASE` convention for constants
//    has no equivalent in Go — everything follows the same exported /
//    unexported casing rule, including constants.
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

// Role is the speaker of a Message. It is a named string type — readers
// see "user" / "assistant" / "system" — but the type system prevents us
// from passing a random string anywhere a Role is expected.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
)
