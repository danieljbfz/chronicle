# Go primer for chronicle

A complete, bottom-up guide to the Go a newcomer needs to read and change chronicle. It starts from the language itself, builds up to the abstractions chronicle leans on, then covers the project mechanics and the libraries. Read it once cover to cover and the source should make sense at a glance, or jump to a section when something in the code feels new.

This is not the full Go language spec. It is the slice of Go that chronicle actually uses, explained with examples taken from chronicle itself.

## Contents

**Reading any Go file**

- [1. The mental model](#1-the-mental-model)
- [2. The shape of a file](#2-the-shape-of-a-file)
- [3. Variables, constants, and the zero value](#3-variables-constants-and-the-zero-value)
- [4. Control flow: if, for, switch](#4-control-flow-if-for-switch)
- [5. Pointers](#5-pointers)
- [6. Structs and methods](#6-structs-and-methods)
- [7. Typed constants and enums](#7-typed-constants-and-enums)
- [8. Slices and maps](#8-slices-and-maps)
- [9. Functions, multiple returns, and closures](#9-functions-multiple-returns-and-closures)
- [10. Errors are values](#10-errors-are-values)
- [11. defer](#11-defer)
- [12. Formatting with fmt](#12-formatting-with-fmt)

**The abstractions chronicle is built on**

- [13. Interfaces](#13-interfaces)
- [14. Type assertions and type switches](#14-type-assertions-and-type-switches)
- [15. The small standard interfaces: io.Reader, io.Writer, fs.FS](#15-the-small-standard-interfaces-ioreader-iowriter-fsfs)
- [16. Working with JSON](#16-working-with-json)

**Project mechanics**

- [17. Packages, modules, and project structure](#17-packages-modules-and-project-structure)
- [18. Concurrency in chronicle](#18-concurrency-in-chronicle)
- [19. The init() function (and why this project avoids it)](#19-the-init-function-and-why-this-project-avoids-it)
- [20. Tests](#20-tests)
- [21. The blank identifier](#21-the-blank-identifier)
- [22. The toolchain and make](#22-the-toolchain-and-make)

**The libraries**

- [23. The libraries chronicle uses](#23-the-libraries-chronicle-uses)
- [Further reading](#further-reading)

---

## 1. The mental model

Go is a small language built for one goal: code that another person can read on the day it is written and the day it is debugged five years later. It has few constructs, and the community has settled on a small set of patterns. When a snippet of Go looks confusing, it is usually a naming choice, not a language feature.

A handful of one-liners explain most of Go's design:

- **Capitalization is access control.** A name that starts with a capital letter (`Conversation`) is exported and visible to other packages. A name that starts lowercase (`parseStream`) is private to its package. There are no `public` or `private` keywords.
- **One concept per file, and files stay short.** A 200-line file is normal. A 1000-line file is a smell.
- **Errors are values, not exceptions.** A function that can fail returns `(result, error)`, and the caller checks `if err != nil`. There is no `try`/`catch`.
- **Interfaces are satisfied implicitly.** A type does not declare "I implement `Provider`." If it happens to have the right methods, it satisfies the interface, and the compiler checks that where the value is used.
- **The zero value is always usable.** Every type has a well-defined default — `0`, `""`, `false`, `nil` — and good Go makes that default mean something sensible, so you rarely need a constructor.

## 2. The shape of a file

```go
// Package contracts defines the normalized domain types ...
package contracts

import (
    "encoding/json"
    "time"
)

type ProjectID string

func (c Conversation) FirstUserPrompt() string {
    // ...
}
```

Every file starts with a `package` declaration, and by convention the package name matches the folder name. Then come the imports, grouped and sorted by Go's formatter (`gofmt`): the standard library first, then third-party packages, then the project's own. After the imports, types and functions appear in any order — Go reads the whole package in one pass, so there is no header-then-body split like C has.

## 3. Variables, constants, and the zero value

```go
var sessionCount int           // declared, zero-valued (0)
var name string = "claude"     // declared with an explicit type and value
greeting := "hi"               // short declaration: the type is inferred (string)
const adapterName = "claude"   // a compile-time constant
```

The `:=` short form is the common one inside functions. The longer `var` form is used at package level and when you want the zero value without assigning anything. The **zero value** of every type is defined: `0` for numbers, `""` for strings, `false` for booleans, and `nil` for pointers, slices, maps, interfaces, channels, and function values. You never hit an "undefined variable" at runtime — only the compiler complains, and it complains about a variable you declared but never used, which it treats as an error.

## 4. Control flow: if, for, switch

Go has three control structures, and `for` is the only loop.

```go
// if — parentheses are not used; braces are always required.
if conv.IsAbandoned() {
    return
}

// if with an initializer: the variable is scoped to the if/else only.
if err := f(); err != nil {
    return err
}

// for as a counted loop.
for i := 0; i < n; i++ { ... }

// for as a while loop.
for scanner.Scan() { ... }

// for as an infinite loop.
for { ... }

// for ... range walks a slice (index, value) or a map (key, value).
for _, m := range conv.Messages { ... }
```

`range` is how you iterate. Over a slice it yields the index and a copy of each element. Over a map it yields the key and value in no guaranteed order. The blank identifier `_` (see [section 21](#21-the-blank-identifier)) drops the half you do not need.

`switch` in Go does not fall through from one case to the next, so you do not write `break` at the end of every case. A case can list several values, and a bare `switch` with no expression is a clean way to write an if/else ladder.

```go
switch m.Role {
case contracts.RoleUser:
    builder.WriteString("## User\n\n")
case contracts.RoleAssistant:
    builder.WriteString("## Assistant\n\n")
default:
    fmt.Fprintf(builder, "## %s\n\n", m.Role)
}
```

There is one special form, the **type switch**, which switches on the concrete type behind an interface. It is common enough in chronicle that it has its own section ([section 14](#14-type-assertions-and-type-switches)).

## 5. Pointers

A pointer is the address of a value rather than the value itself. Two operators are all you need:

- `&x` takes the address of `x` — it produces a pointer to `x`.
- `*p` follows the pointer — it reads or writes the value `p` points at.

The reason to reach for a pointer is one of two: you want a function to change the caller's value rather than a copy of it, or the value is large and you would rather not copy it on every call. Go has no pointer arithmetic, and the zero value of a pointer is `nil`.

chronicle's Markdown renderer is a clear example. Every helper takes a `*strings.Builder` — a pointer to one growing buffer — so they all write into the same buffer instead of each getting a copy:

```go
func writeHeader(builder *strings.Builder, c contracts.Conversation) {
    fmt.Fprintf(builder, "# %s\n\n", c.ListingTitle())
}

// the caller passes the address of its builder
var builder strings.Builder
writeHeader(&builder, c)
```

Pointers also decide how a method sees its receiver, which is the next section.

## 6. Structs and methods

A `struct` is a record with named fields. There is no inheritance. Methods attach to a type through a **receiver** written in parentheses before the method name.

```go
type Conversation struct {
    SessionID SessionID
    Messages  []Message
    StartedAt time.Time
}

func (c Conversation) FirstUserPrompt() string {  // value receiver
    for _, m := range c.Messages { ... }
    return ""
}

func (a *App) Detect() error {                    // pointer receiver
    a.detected = true
    return nil
}
```

- A **value receiver** `(c Conversation)` gives the method a copy. Use it when the method does not change the receiver and the struct is small.
- A **pointer receiver** `(a *App)` gives the method the original, so changes stick, and nothing large is copied.

Pick one style per type and stay with it. chronicle uses value receivers for plain data (`Conversation`, `StorageVersion`) and pointer receivers for the stateful coordinators (`App`, `*claude.Provider`).

## 7. Typed constants and enums

Go has no dedicated `enum` keyword. A closed set of values is a named type plus a block of constants. chronicle's roles are string-backed, which keeps them readable in JSON and in logs:

```go
type Role string

const (
    RoleUser      Role = "user"
    RoleAssistant Role = "assistant"
    RoleSystem    Role = "system"
)
```

When the values are just "one of a few states" and their numeric value does not matter, the `iota` keyword numbers them for you. `iota` is `0` in the first line of a `const` block and counts up by one per line:

```go
type status int

const (
    statusLoading status = iota // 0
    statusReady                 // 1
    statusError                 // 2
)
```

The TUI screens use this shape for their loading state. The point of giving these their own type (`status`, not bare `int`) is that the compiler then stops you passing a random integer where a `status` is expected.

## 8. Slices and maps

A **slice** is Go's growable list. A **map** is its key-value dictionary.

```go
var ids []SessionID                // a nil slice: len 0, but append still works
ids = append(ids, "abc")           // append returns a new slice — reassign it
names := []string{"alpha", "beta"} // a slice literal

counts := map[string]int{}         // an empty, ready-to-use map
counts["claude"]++
v, ok := counts["copilot"]         // ok is false when the key is absent; v is 0
```

Two things trip up newcomers:

- **`append` returns a new slice header.** If you forget to assign it back (`ids = append(ids, …)`), your append is lost. The reason is that a slice is a small header (pointer, length, capacity), and growing it can move the underlying array, which produces a new header.
- **Reading a missing map key returns the zero value, not an error.** Use the two-result form `v, ok := m[k]` when you need to tell "absent" apart from "present and zero."

A `nil` slice and an empty slice behave the same for `len` and `append`, so chronicle returns `nil` freely to mean "no items" without any special empty-slice ceremony.

## 9. Functions, multiple returns, and closures

Functions can return more than one value — the `(result, error)` pair is everywhere — and a function is itself a value you can store in a variable, pass as an argument, or return.

```go
func parseStream(r io.Reader, source StorageVersion) (Conversation, error) { ... }
```

A **closure** is a function value that captures variables from the scope around it. chronicle uses closures to hand a unit of work to something that will run it later. The parallel-detection code passes one closure per provider to an error group, and each closure captures the loop's data:

```go
group := new(errgroup.Group)
for i := range entries {
    group.Go(func() error {
        return detect(entries[i]) // the closure captures entries[i]
    })
}
```

The TUI uses the same idea: a Bubble Tea command is a closure of type `func() tea.Msg`, and the runtime runs it off the main loop (see [section 18](#18-concurrency-in-chronicle)).

## 10. Errors are values

An error is an ordinary value that satisfies the built-in `error` interface. The dominant pattern is the three-line check:

```go
data, err := os.ReadFile(path)
if err != nil {
    return Config{}, err
}
```

When you need to react to a *specific* error, compare with `errors.Is`, which sees through wrapping:

```go
if errors.Is(err, fs.ErrNotExist) {
    return Defaults(), nil // an absent config file is fine — use defaults
}
```

When you need to add context as an error travels up, wrap it with the `%w` verb in `fmt.Errorf`. Wrapping keeps the original error reachable by `errors.Is` and `errors.As` while adding a readable layer:

```go
return fmt.Errorf("read %s: %w", path, err)
```

There is no `try`/`catch`. The long runs of `if err != nil` are not noise — they are the explicit places where the next reader sees exactly what each step can fail at, and what happens when it does.

## 11. defer

`defer` schedules a call to run when the surrounding function returns, no matter which path it returns by. The classic use is "acquire a resource, defer its release, then stop thinking about it":

```go
f, err := root.Open(sessionFile)
if err != nil {
    return Conversation{}, err
}
defer f.Close() // runs on every return below

// ... read from f, return early on errors ...
```

It is the right tool for files, locks, and HTTP response bodies. Deferred calls run in last-in-first-out order if you register several.

## 12. Formatting with fmt

The `fmt` package builds strings from values using format verbs. The ones chronicle uses:

| Verb | Prints |
| --- | --- |
| `%s` | a string, or anything with a `String()` method |
| `%d` | an integer in base 10 |
| `%q` | a double-quoted string, useful in error messages |
| `%v` | any value in its default format |
| `%+v` | a struct with its field names |
| `%T` | the type of the value |
| `%w` | wraps an error (only in `fmt.Errorf`, see [section 10](#10-errors-are-values)) |

The family of functions differs only in where the result goes:

```go
s := fmt.Sprintf("processed %d sessions", n) // returns a string
fmt.Fprintf(builder, "# %s\n\n", title)      // writes to an io.Writer
err := fmt.Errorf("read %s: %w", path, err)  // builds a wrapped error
```

chronicle's Markdown renderer is mostly `fmt.Fprintf` into a `*strings.Builder`. A logging-style detail worth knowing: `%s` on a value that has a `String() string` method prints that method's output, which is how typed IDs and enums render as their readable form.

## 13. Interfaces

An interface is a set of method signatures. A type satisfies the interface simply by having those methods — there is no `implements` keyword, and the type never names the interface. This is called **structural** and **implicit** satisfaction.

```go
type Provider interface {
    Name() string
    Detect(root fs.FS) (StorageVersion, error)
    ReadSession(root fs.FS, id SessionID) (Conversation, error)
}

var p Provider = claude.New() // works because *claude.Provider has those methods
```

Two patterns recur in chronicle:

**The compile-time check.** At the bottom of an adapter file you will see:

```go
var _ contracts.Provider = (*Provider)(nil)
```

This declares a throwaway variable of the interface type and assigns a typed `nil` pointer to it. It produces no runtime cost — its only job is to make the compiler shout the moment `*Provider` stops satisfying `contracts.Provider`, so a missing method is caught at build time rather than in production.

**Fakes for tests.** Because satisfaction is implicit, a test can define a tiny `fakeProvider` with the same methods and the code under test never knows it is talking to a fake. This is why chronicle's tests need no mocking framework. See [section 20](#20-tests).

The base `Provider` interface is deliberately small. Everything destructive or tool-specific is a separate optional interface (`Cleaner`, `MemoryStore`, `Resumable`, and the rest), which the next section shows how the code discovers.

## 14. Type assertions and type switches

When you hold an interface value, sometimes you need to ask what concrete type is behind it. That is a **type assertion**, written `value.(Type)`, and its two-result form tells you whether the answer is yes:

```go
cleaner, ok := provider.(contracts.Cleaner)
if ok {
    // this provider supports cleanup — cleaner is now typed as contracts.Cleaner
}
```

This is exactly how chronicle discovers optional capabilities. Every adapter satisfies the base `Provider`. Composition asks each one "are you also a `Cleaner`? a `MemoryStore`? a `Resumable`?" with a type assertion, and only the adapters that answer yes take part in that feature. The capability lives in the type system, so any code touching a capability is visibly doing something the read-only contract does not allow.

When you want to handle *several* concrete types, a **type switch** does it in one block. Each case binds the value to its unwrapped type, so you read its fields directly. chronicle's Markdown renderer dispatches on the kind of content block this way:

```go
switch v := b.(type) {
case contracts.TextBlock:
    builder.WriteString(v.Text)
case contracts.ThinkingBlock:
    // v is a ThinkingBlock here
case contracts.ToolUseBlock:
    // v is a ToolUseBlock here
default:
    // some block kind we did not list
}
```

The single-value form `v := x.(Type)` (without the `ok`) panics if the type does not match, so reach for the two-result form unless you have already proven the type.

## 15. The small standard interfaces: io.Reader, io.Writer, fs.FS

The standard library is built on a few tiny interfaces, and using them is what makes chronicle's code easy to test.

`io.Reader` and `io.Writer` each have a single method — `Read([]byte) (int, error)` and `Write([]byte) (int, error)`. A function that takes an `io.Writer` can write to a file, a network socket, `os.Stdout`, or a `bytes.Buffer` in a test, without knowing the difference.

`fs.FS` is the same idea for a whole read-only filesystem, with one method: `Open(name string) (fs.File, error)`.

```go
func parseStream(r io.Reader, source StorageVersion) (Conversation, error) // reads anywhere
func readSessionFile(root fs.FS, name string, ...)                         // opens root/name
```

This is the single most important pattern for testable code in chronicle. The adapters never call `os.Open`. They call `root.Open(name)` on an `fs.FS` they were handed. In production, composition passes `os.DirFS("/home/user/.claude")`. In tests, the suite passes `fstest.MapFS{"projects/p/s.jsonl": &fstest.MapFile{Data: ...}}`, an in-memory filesystem from the standard library. The adapter cannot tell the two apart, so the same code path runs against real files and fixture content with no mocking. It is why the tests run in milliseconds.

## 16. Working with JSON

chronicle reads JSON that it does not fully control, so it decodes defensively. Three tools carry most of the work.

**Struct tags** map a Go field to a JSON key. The backtick string after a field is the tag:

```go
type rawRecord struct {
    Type    string          `json:"type"`
    Message json.RawMessage `json:"message"`
}
```

Without the tag, Go would look for a JSON key named `Type` instead of `type`. A JSON key with no matching field is ignored, and a field with no matching key keeps its zero value — both of which are what let chronicle survive a new field appearing upstream.

**`json.RawMessage`** is a `[]byte` the decoder leaves untouched. You use it when the shape of a value depends on a discriminator you have not read yet. chronicle leaves the message body raw until it has read the record's `type`.

**The two-step decode** is the standard Go technique for tagged-union JSON, since Go has no built-in sum type:

```go
// Step 1: decode the envelope to learn the type.
var rec rawRecord
json.Unmarshal(line, &rec)

// Step 2: decode the body into the right shape for that type.
switch rec.Type {
case "user":
    var body userBody
    json.Unmarshal(rec.Message, &body)
case "assistant":
    var body assistantBody
    json.Unmarshal(rec.Message, &body)
}
```

`json.Unmarshal(data, &target)` takes a pointer to the target so it can fill it in (see [section 5](#5-pointers)). chronicle wraps the tolerant case in a small `decodeOrZero` helper that ignores a decode error and leaves the target at its zero value, which is the resilient choice when a half-broken record should still yield the best block it can.

## 17. Packages, modules, and project structure

A Go project is a module. You create one with `go mod init <module-path>`, where the module path is the import prefix for every package in the project — chronicle's is `github.com/danieljbfz/chronicle`, recorded on the first line of `go.mod`. That one command writes `go.mod`, and from then on the directory is a module that `go build`, `go test`, and `go get` all understand without any further setup.

There is no `src/` directory and no separate build manifest. Packages sit directly under the module root, one directory per package, and the directory name is the package name. A binary's entry point lives under `cmd/<binary-name>/` in `package main`, and code that should be private to the module lives under `internal/` — the compiler refuses to let anything outside the module import an `internal/` package. The layout is the architecture: read the top-level directories and you know the shape of the system.

```
github.com/danieljbfz/chronicle/        <- module root, set by go.mod
├── go.mod                               <- module path + Go version + direct deps
├── go.sum                               <- checksums for every resolved dependency
├── cmd/chronicle/                       <- package main, the binary
├── contracts/                           <- package contracts
├── adapters/claude/                     <- package claude
└── internal/config/                     <- package config, module-private
```

The module path prefixes every import. From inside `composition/browse.go` you write:

```go
import (
    "github.com/danieljbfz/chronicle/contracts"
    "github.com/danieljbfz/chronicle/adapters/claude"
)
```

`go get <pkg>` adds a third-party dependency, recording the version in `go.mod` and its checksum in `go.sum`. Both files are committed, so every build resolves the same versions. `go mod tidy` drops dependencies the code no longer imports and adds any it does. There is no separate package manifest, no separate build configuration, and no external test runner — the standard tooling does it all.

How chronicle organizes its packages on top of these conventions — the hexagonal layers and the rule that imports only flow downhill — is the subject of [`codebase-tour.md`](codebase-tour.md).

## 18. Concurrency in chronicle

Concurrency in Go starts a goroutine with `go func()` and communicates over channels. chronicle uses concurrency in two places, and neither one writes raw goroutines or channels by hand — both reach for a higher-level helper instead.

The composition layer fans out detection. `composition/browse.go` runs every provider's `Detect` at once with an `errgroup.Group` — `group.Go(...)` per provider, then `group.Wait()` — so a slow provider does not serialize the others. This is the "fan out N tasks, wait for all, surface the first error" pattern, and the errgroup entry in [section 23](#23-the-libraries-chronicle-uses) covers it.

The TUI never starts a goroutine directly. Bubble Tea owns the concurrency through its command model: a `tea.Cmd` is a closure the runtime runs off the main loop, and whatever it returns comes back as a `tea.Msg` for `Update` to handle. chronicle loads a session this way — `ReadSession` runs inside a command, and the conversation arrives as a message rather than blocking the render loop. The bubbletea entry in [section 23](#23-the-libraries-chronicle-uses) explains the model.

## 19. The init() function (and why this project avoids it)

Go runs every `init()` function in every imported package before `main()` starts. It is tempting to use `init()` for "register myself" patterns. chronicle deliberately does not — see `adapters/all.go`. The reason is that an explicit list of providers in one file is easier to read and to grep than registrations scattered across packages that run at an order you have to reason about. The cost is one extra line per new provider, and the payoff is never having to debug `init()` ordering.

## 20. Tests

A test is any function in a `_test.go` file whose name starts with `Test` and takes a `*testing.T`. `go test ./...` finds and runs them.

```go
func TestFirstUserPrompt_skipsMetaAndAssistant(t *testing.T) {
    c := Conversation{ /* ... */ }
    got := c.FirstUserPrompt()
    if got != "read the docs" {
        t.Errorf("FirstUserPrompt() = %q, want %q", got, "read the docs")
    }
}
```

A few helpers you will meet: `t.Errorf` reports a failure and keeps going, `t.Fatalf` reports and stops the test, `t.Helper()` marks a helper so failure lines point at the caller, `t.TempDir()` makes an auto-cleaned temp directory, and `t.Setenv` sets an environment variable for one test. chronicle's tests lean on the standard-library fakes `fstest.MapFS` (an in-memory filesystem) and `httptest` rather than mocking frameworks, which is the payoff of the small-interface design in [section 15](#15-the-small-standard-interfaces-ioreader-iowriter-fsfs). Test names describe behavior, not implementation — `TestFirstUserPrompt_skipsMetaAndAssistant`, not `TestFirstUserPrompt_branchTwo` (see [`naming-conventions.md`](naming-conventions.md)).

## 21. The blank identifier

`_` is the "I am intentionally ignoring this" placeholder. It tells the reader and the compiler that an ignored value is deliberate, not forgotten.

```go
_, err := io.WriteString(w, text)            // we do not need the byte count
for _, m := range c.Messages { ... }         // we do not need the index
var _ contracts.Provider = (*Provider)(nil)  // the compile-time interface check
```

In production code, ignoring an *error* with `_` is a yellow flag — do it only when the operation is genuinely fire-and-forget, and prefer a named helper that documents the intent over a bare `_ = something()`.

## 22. The toolchain and make

The raw Go commands you reach for most:

| Command | What it does |
|---|---|
| `go build -o chronicle ./cmd/chronicle` | Compile the binary to the repo root. |
| `go test ./...` | Run every test in the module. |
| `go test -run TestName -v ./pkg` | Run one test verbosely. |
| `go vet ./...` | Static analysis the compiler does not do. |
| `gofmt -w .` | Reformat every file. (Your editor likely does this on save.) |
| `go doc <pkg>.<symbol>` | Print the doc comment for a symbol. |
| `go run ./cmd/chronicle list` | Compile and run in one step (no binary kept). |

The `Makefile` wraps these into the gates the project actually uses:

| Target | What it does |
|---|---|
| `make build` | Build the binary at the repo root. |
| `make test` | `go test -race ./...` — every test under the race detector. |
| `make fmt` | `gofmt -w .`. |
| `make vet` | `go vet ./...`. |
| `make lint` | `golangci-lint`, installed on demand if missing. |
| `make check` | `fmt` then `vet` then `lint` then `test` — the all-in-one gate. |
| `make cover` | Test with coverage, writing `coverage.html`. |

`make check` is the gate that has to pass before a change lands. If it fails locally, it fails in review.

## 23. The libraries chronicle uses

chronicle keeps its dependency surface small. Three direct dependencies do the heavy lifting — cobra for the CLI, the Charm stack for the TUI, and BurntSushi/toml for config — plus `golang.org/x/sync` for one piece of concurrency. Everything else in `go.mod` is pulled in by those four. This section is what each one is for and how chronicle uses it.

### cobra — the CLI framework

[`github.com/spf13/cobra`](https://pkg.go.dev/github.com/spf13/cobra) turns a tree of commands into a parser, the help text, and the dispatch. chronicle builds the tree in `cmd/chronicle/`: `main.go` has `newRootCmd()`, and every subcommand has its own `newXCmd()` constructor that returns a `*cobra.Command`. The root wires the children with `AddCommand`, and its own `RunE` launches the TUI when the user passes no subcommand.

```go
func newExportCmd() *cobra.Command {
    var noTools bool
    cmd := &cobra.Command{
        Use:   "export <sessionId>",
        Short: "Write a filtered Markdown transcript",
        RunE: func(cmd *cobra.Command, args []string) error {
            // do the work, return an error on failure
            return nil
        },
    }
    cmd.Flags().BoolVar(&noTools, "no-tools", false, "Drop tool blocks")
    return cmd
}
```

Two things worth knowing. Use `RunE` (returns an error) rather than `Run` — cobra prints the error and sets a non-zero exit code, so a subcommand never calls `os.Exit` itself. Flags bind to local variables through `Flags().BoolVar`, `StringVar`, and friends, declared next to the command they belong to.

### The Charm stack — the TUI

The terminal UI is built on Charm's v2 libraries. The four that matter:

**`bubbletea/v2` — the runtime, and the Elm architecture.** A screen is a value that implements three methods:

- `Init() tea.Cmd` — the work to kick off when the screen starts.
- `Update(tea.Msg) (tea.Model, tea.Cmd)` — react to one event, return the next state.
- `View() tea.View` — render the current state.

State lives in the model. Nothing mutates it from outside `Update`. Events arrive as **messages** (`tea.Msg`): a keypress is a `tea.KeyPressMsg`, a resize is a `tea.WindowSizeMsg`, and a custom message is whatever a command returns. **Commands** (`tea.Cmd`) are how a screen does work without blocking the render loop — a command is a closure the runtime runs off the main loop, and its return value comes back to `Update` as a message. chronicle reads a session inside a command and handles the result as a `loadedMsg`, which is why a slow disk never freezes the UI.

The v2 API has a few sharp edges, each easy to trip over once:

- `View()` returns a `tea.View`, not a `string`. Build one with `tea.NewView(content)`.
- There is no `tea.WithAltScreen()` option. Alt-screen mode is a per-frame field: `view.AltScreen = true`.
- Key messages are `tea.KeyPressMsg`, not `tea.KeyMsg`. `bubbles/v2/key.Matches` accepts any `fmt.Stringer`, which the key message satisfies.

**`bubbles/v2` — ready-made components.** `list` (the session list, with filtering), `viewport` (the scrollable transcript), `table` (the stats tables), `textinput`, `key` (declarative bindings with help text), `help` (the footer's style), and `spinner`. A component exposes its own message types, and a screen forwards messages into the component's `Update` so it stays live.

**`lipgloss/v2` — styling and layout.** Styles are values you build and compose, and the composition rules are not a CSS cascade — a child style does not inherit from a parent unless you copy it in. chronicle's styles and the canonical separators live in `cmd/chronicle/tui/theme/`.

**`glamour` — Markdown to ANSI.** Renders the transcript Markdown in the viewport, with syntax-highlighted code blocks. The style name comes from chronicle's config and defaults to `dark`. glamour v2 dropped the auto-style helper, so chronicle passes an explicit standard style rather than detecting the terminal background.

To test a screen without a real terminal, drive its `Update` and `View` directly in a unit test, or use `teatest` (`charmbracelet/x/exp/teatest/v2`), which runs a model in process and lets you assert on the rendered frames. Any claim about what a screen renders needs one of these, because the alt-screen output is not capturable from outside the terminal.

### BurntSushi/toml — config

[`github.com/BurntSushi/toml`](https://pkg.go.dev/github.com/BurntSushi/toml) decodes chronicle's own config. `internal/config/config.go` calls `toml.Decode` over a `Config` struct whose fields carry `toml:"..."` tags, after starting from the values `Defaults()` returns. A missing config file is not an error — the defaults stand, and anything the file does set merges on top.

### golang.org/x/sync/errgroup — parallel detection

[`errgroup`](https://pkg.go.dev/golang.org/x/sync/errgroup) is the standard "fan out, wait for all, return the first error" helper. `composition/browse.go` uses it in `New` to run every provider's `Detect` at once:

```go
group := new(errgroup.Group)
for i := range entries {
    group.Go(func() error {
        // detect one provider, record its result
        return nil
    })
}
err := group.Wait() // blocks until every goroutine returns
```

`group.Wait()` blocks until all the launched functions return, and hands back the first non-nil error any of them produced. It is the right tool when the tasks are independent and you want them to run together rather than one after another.

---

## Further reading

- The official tour: <https://go.dev/tour/> — does what it says.
- Effective Go: <https://go.dev/doc/effective_go> — short, opinionated, ages well.
- The standard library docs: <https://pkg.go.dev/std> — the authoritative reference, fast to search.
- The Bubble Tea tutorial and examples: <https://github.com/charmbracelet/bubbletea> — the Elm-architecture model with worked screens.
- The cobra user guide: <https://github.com/spf13/cobra/blob/main/site/content/user_guide.md> — commands, flags, and the `RunE` contract.

When in doubt, search `pkg.go.dev` for the symbol you are looking at. Go's documentation is on average the best of any major language ecosystem because the standard library's doc comments are a hard requirement.
