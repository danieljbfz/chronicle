# Go primer for chronicle

A focused reference for the Go idioms you will meet while implementing this project. The order roughly follows the order tasks introduce each concept, so you can read it once cover-to-cover or skim back to a section when something in a task feels new.

This is not a complete Go tutorial. It explains the ten or twelve patterns the plan actually uses, with examples taken from chronicle itself.

---

## 1. Mental model

Go is a small language designed for one job: write code that other people can read on the day it is written and the day it is debugged five years later. The language has only a handful of constructs, and the community has converged on a small number of patterns. If a snippet of Go looks confusing, it is usually because of a *naming* choice, not a *language* feature.

A few one-liners that explain a lot of design choices:

- **Capitalization is access control.** `Foo` is exported (visible to other packages). `foo` is package-private. There are no `public` / `private` keywords.
- **One concept per file, files are short.** A 200-line file is normal. A 1000-line file is a code smell.
- **Errors are values, not exceptions.** A function that can fail returns `(result, error)`. The caller checks `err != nil`. There is no `try/catch`.
- **Interfaces are satisfied implicitly.** A type does not declare "I implement Provider". If it happens to have the right methods, it satisfies the interface. The compiler enforces this at the call site.

---

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

Every file starts with a `package` declaration. The folder name and the package name match by convention. `import` groups can hold standard-library packages first, then third-party packages, then your own — Go's formatter (`gofmt`) enforces the grouping with blank lines.

The file is then types, then functions, in any order — Go does not require a header / declaration split like C does. The compiler reads the whole package in one pass.

---

## 3. Variables and constants

```go
var sessionCount int          // declared, zero-valued (= 0)
var name string = "claude"    // declared with explicit type and value
greeting := "hi"              // short declaration: type inferred (string)
const adapterName = "claude"  // compile-time constant
```

The `:=` shortcut is the most common. Use it inside functions; use `var` at package level. The zero value of every type is well-defined (`0` for numbers, `""` for strings, `nil` for pointers, slices, maps, interfaces, channels, and function values). You will never see "undefined variable" runtime errors — only the compiler complains.

---

## 4. Structs and methods

```go
type Conversation struct {
    SessionID    SessionID
    Project      ProjectID
    Messages     []Message
    StartedAt    time.Time
}

func (c Conversation) FirstUserPrompt() string {
    for _, m := range c.Messages {
        // ...
    }
    return ""
}

func (a *App) Detect() error {
    a.detected = true
    return nil
}
```

A `struct` is a record with named fields. There is no inheritance. Methods attach via the **receiver** in parentheses before the method name.

- **Value receiver** `(c Conversation)` — the method sees a copy of the struct. Use this when the method does not mutate the receiver, the struct is small, and you want the caller to be allowed to call the method on either a value or a pointer.
- **Pointer receiver** `(a *App)` — the method sees the original. Use this when the method mutates state, or when the struct is large and copying it would waste work.

Pick one style per type and stick with it. Mixing value and pointer receivers on the same type is allowed but confuses readers.

---

## 5. Interfaces

```go
type Provider interface {
    Name() string
    Detect(root fs.FS) (StorageVersion, error)
    ReadSession(root fs.FS, id SessionID) (Conversation, error)
}

// Anywhere that needs a Provider can take this type. The Claude adapter
// happens to have a method set that satisfies the interface. The compiler
// proves the relationship at the moment the value is assigned.

var p Provider = claude.New()  // works because *claude.Provider has all three methods
```

Interfaces in Go are **structural** and **implicit**. The `claude.Provider` type does not say "I implement contracts.Provider" anywhere — it just has the right methods.

Two patterns you will see in chronicle:

**Compile-time interface check.** At the bottom of an adapter file:
```go
var _ contracts.Provider = (*Provider)(nil)
```
This says: "the compiler must verify that `*Provider` satisfies `contracts.Provider`". `(*Provider)(nil)` is a typed nil pointer; `_` is the blank identifier ("I do not need this value"). The line costs nothing at runtime — its only job is to make the compiler shout if a method goes missing.

**Mocking via interface.** Composition tests use a `fakeProvider` struct that implements the same `Provider` interface. The application code never knows it is talking to a fake.

---

## 6. Slices, maps, and `make`

```go
var ids []SessionID                // a nil slice; len == 0, append works fine
ids = append(ids, "abc")           // returns a new slice; reassign or you lose the result
names := []string{"alpha", "beta"} // composite literal

m := map[string]int{}              // empty map, ready to use
m["foo"] = 1
v, ok := m["bar"]                  // ok == false; v == 0 (zero value)

buf := make([]byte, 0, 1024)       // explicit capacity for performance
```

Two things that catch newcomers:

- **`append` returns a new slice header.** If you forget to assign it back, your append did nothing. Always write `xs = append(xs, …)`.
- **Reading a missing map key returns the zero value, not an error.** Use the two-result form `v, ok := m[k]` when you need to distinguish "not present" from "present and zero".

---

## 7. Errors

```go
func Load(path string) (Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        if errors.Is(err, fs.ErrNotExist) {
            return Defaults(), nil
        }
        return Config{}, err
    }
    // ...
}
```

Errors are ordinary values that implement the `error` interface. The dominant pattern is the three-line check:
```go
result, err := SomeCall()
if err != nil {
    return Zero, err
}
```

When you need to test for a *specific* error, use `errors.Is`:
```go
if errors.Is(err, fs.ErrNotExist) { /* handle absent file */ }
```

When you need to add context, wrap with `%w`:
```go
return fmt.Errorf("read %s: %w", path, err)
```

There is no `try/catch`. Long chains of `if err != nil` are not noise — they are the explicit place where the next reader sees what each step can fail at.

---

## 8. `defer`

```go
func parseStream(r io.Reader) (Conversation, error) {
    f, err := os.Open(path)
    if err != nil {
        return Conversation{}, err
    }
    defer f.Close()  // runs when the surrounding function returns, no matter how

    // ... read from f, possibly return early ...
}
```

`defer` registers a call that runs when the surrounding function exits. The classic use is "open this resource, defer close, then forget about it". It is the right tool for files, locks, and HTTP response bodies.

---

## 9. `io.Reader`, `io.Writer`, `fs.FS`

```go
func Markdown(c Conversation) string { /* ... */ }                          // returns a string
func CopyOSC52(w io.Writer, text string) error { /* writes to w */ }        // takes anywhere-writable
func parseStream(r io.Reader, ...) (Conversation, error) { /* reads r */ }  // takes anywhere-readable
func readSessionFile(root fs.FS, name string, ...) { /* opens root/name */ }
```

`io.Reader` and `io.Writer` are tiny interfaces (`Read([]byte) (int, error)` and `Write([]byte) (int, error)`) that the entire standard library uses. A function that takes `io.Writer` can be tested by passing a `bytes.Buffer`. It can write to a file, a network socket, or `os.Stdout` in production without knowing the difference.

`fs.FS` is the same idea for whole filesystems. Production code passes `os.DirFS("/home/u/.claude")`; tests pass `fstest.MapFS{"projects/p/s.jsonl": &fstest.MapFile{Data: ...}}`. The adapter is none the wiser.

This is the single most important Go pattern for testable code. chronicle's adapters never call `os.Open` directly. They call `root.Open(file)` where `root` is an `fs.FS`. Tests pass a fake FS, production passes the real one.

---

## 10. JSON: `json.RawMessage` and the two-step decode

```go
type rawRecord struct {
    Type    string          `json:"type"`
    Message json.RawMessage `json:"message"`  // leave undecoded for now
}

// Step 1: decode the envelope.
var rec rawRecord
json.Unmarshal(line, &rec)

// Step 2: decode the body based on Type.
switch rec.Type {
case "user":
    var body userBody
    json.Unmarshal(rec.Message, &body)
    // ...
case "assistant":
    var body assistantBody
    // ...
}
```

The backtick `\`json:"type"\`` is a **struct tag** — a string the standard library reads via reflection. Here it says "when decoding JSON, this field comes from a key called `type`".

`json.RawMessage` is a `[]byte` that the decoder leaves alone. We use it when the body's shape depends on a discriminator field. chronicle uses this for both Claude (where the `type` field decides whether the embedded message is a user or assistant body) and for the content array (where each part has its own `type`).

This two-step decode is the standard Go technique for tagged-union JSON. There is no built-in sum type — you fake it with `interface{}`, a discriminator, and `RawMessage`.

---

## 11. Packages, imports, `go mod`

```
github.com/djbf/chronicle/        <- module root, set by go.mod
├── go.mod
├── contracts/
│   └── conversation.go           package contracts
├── adapters/
│   └── claude/
│       └── parse.go              package claude
```

The module path in `go.mod` (e.g. `module github.com/djbf/chronicle`) prefixes every import. From inside `composition/browse.go` you write:
```go
import (
    "github.com/djbf/chronicle/contracts"
    "github.com/djbf/chronicle/adapters/claude"
)
```

`go get` adds third-party dependencies to `go.mod` and `go.sum`. `go build` compiles. `go test ./...` runs every test in the module. `go vet ./...` runs the static checks Go ships with.

There is no separate package manifest, no separate build configuration, no test runner. The standard tooling does it all.

---

## 12. Tests

```go
package contracts

import "testing"

func TestFirstUserPrompt_skipsMetaAndAssistant(t *testing.T) {
    c := Conversation{ /* ... */ }
    got := c.FirstUserPrompt()
    if got != "read the docs" {
        t.Errorf("FirstUserPrompt() = %q, want %q", got, "read the docs")
    }
}
```

A test is any function in a `_test.go` file whose name starts with `Test` and takes one argument of type `*testing.T`. `go test ./...` finds them.

- `t.Errorf` reports a failure and keeps going.
- `t.Fatalf` reports a failure and stops *this* test.
- `t.Helper()` marks a helper function so failure lines point to the caller, not the helper.
- `t.TempDir()` creates a temp directory that is auto-cleaned at test end.
- `t.Setenv("KEY", "value")` sets an env var for this test only.

Convention: test names describe behavior, not implementation. `TestFirstUserPrompt_skipsMetaAndAssistant` is good. `TestFirstUserPrompt_branchTwo` is not.

`fstest.MapFS` and `httptest.NewServer` are the standard-library fakes for filesystem and HTTP — chronicle uses both.

---

## 13. The blank identifier `_`

```go
_, err := io.WriteString(w, text)               // we don't care how many bytes were written
data, _ := os.ReadFile(path)                    // ignore the error (only ever in tests / throwaway)
var _ contracts.Provider = (*Provider)(nil)     // compile-time interface check
for _, m := range c.Messages { ... }            // ignore the index
```

`_` is the "I am intentionally not using this value" placeholder. It tells the reader (and the compiler) that an ignored return or parameter is deliberate, not forgotten.

In production code, *ignoring an error is a yellow flag*. Use it sparingly and only when the error is genuinely unrecoverable or the operation is fire-and-forget. The compiler does not enforce this — the style guide does.

---

## 14. Goroutines and channels (you will not use these in Plan A)

Mentioned only to flag what you will *not* see in Plan A. Concurrency in Go uses `go func()` to start a goroutine and channels for communication. The TUI plan will use them lightly. The read-only CLI does not need them — everything is synchronous and that is correct.

---

## 15. The `init()` function (and why this project avoids it)

Go runs every `init() {}` function in every imported package before `main()`. It is tempting to use this for "plugin registration" patterns. chronicle deliberately does not — see `adapters/all.go`. The reason: an explicit list of providers in one file is easier to read and grep than scattered `init()` calls that run at unpredictable points. The cost of one extra line of code per new provider is worth never having to debug `init()` ordering.

---

## 16. The Go toolchain commands you will use

| Command | What it does |
|---|---|
| `go mod init <path>` | Create a new module. |
| `go get <pkg>` | Add a dependency to `go.mod`. |
| `go build ./cmd/chronicle` | Compile the binary. |
| `go test ./...` | Run every test in the module. |
| `go test -run TestName -v ./pkg` | Run one test verbosely. |
| `go vet ./...` | Static analysis the compiler does not do. |
| `gofmt -w .` | Reformat every file. (Your editor likely does this on save.) |
| `go doc <pkg>.<symbol>` | Print the doc comment for a symbol. |
| `go run ./cmd/chronicle list` | Compile and run in one step (no binary kept). |

---

## 17. Five things that *look* weird but make sense once you know them

1. **`if err := f(); err != nil { ... }`** — the `if` statement can declare a variable in its initializer. The variable is in scope only for the `if`/`else` blocks. Idiomatic for one-shot error checks.
2. **Empty interface `interface{}` (or the alias `any`)** — a type that says "I accept anything". You will see it almost nowhere in chronicle. Type-safe Go avoids it.
3. **A `nil` slice and an empty slice behave the same.** `var xs []string` and `xs := []string{}` both let you `len(xs)` and `append`. There is no `null pointer exception` waiting.
4. **Methods can be added to your own types only.** You cannot add a method to `int` or to `time.Time`. You can define your own type around them: `type SessionID string` is a new type whose underlying type is string, and you can attach methods to `SessionID`.
5. **No constructors.** The convention is a function named `New<Type>` (or `New` if there is one type in the package) that returns a ready-to-use value. Zero values are valid wherever possible, which is why so few Go types need a constructor.

---

## Further reading

- The official tour: <https://go.dev/tour/> — does what it says.
- Effective Go: <https://go.dev/doc/effective_go> — short, opinionated, ages well.
- The standard library docs: <https://pkg.go.dev/std> — the authoritative reference, fast to search.

When in doubt, search `pkg.go.dev` for the symbol you are looking at. Go's documentation is on average the best of any major language ecosystem because the standard library's doc comments are a hard requirement.
