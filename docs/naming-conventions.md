# Naming conventions

The single most important readability rule on this project is **one canonical name per concept across every layer**, which the engineering-and-writing-standards skill sets as a hard rule. The CLI flag, the module, the struct field, the test name, the doc heading — all the same word. This document is how that rule lands in a Go codebase, and where Go-idiomatic style qualifies it.

## Contents

- [Canonical names](#canonical-names)
- [Go-specific conventions](#go-specific-conventions)
  - [Identifier case](#identifier-case)
  - [Filenames](#filenames)
  - [Package names](#package-names)
  - [Receivers](#receivers)
  - [Local variable names](#local-variable-names)
  - [Function naming](#function-naming)
  - [Test naming](#test-naming)
- [Cross-layer vocabulary](#cross-layer-vocabulary)
- [When this document is wrong](#when-this-document-is-wrong)

## Canonical names

| Concept | Canonical name | Used where |
|---|---|---|
| The product | `chronicle` | binary, module path, README, docs, `--provider` values lowercased the same way |
| The reader interface (port) | `Provider` | `contracts.Provider`, `claude.Provider`, `adapters.All`, `--provider` flag |
| One conversation | `Conversation` | `contracts.Conversation`, `app.ReadSession` returns this, exported Markdown header |
| One turn | `Message` | `contracts.Message`, JSON output field `message` lowercased |
| One content piece inside a turn | `Block` | `contracts.Block`, `contracts.TextBlock`, `contracts.ToolUseBlock`, … |
| A session identifier | `SessionID` | type name, struct field, JSON `session_id`, CLI argument noun |
| A project identifier | `ProjectID` | type, field, JSON `project_id`, CLI flag `--project` |
| Schema discriminator | `StorageVersion` | type, JSON `version`, doctor output, format-report filename |
| The delete model | `DeletePlan` / `DeleteItem` | composition layer, the `chronicle clean` dry-run output |
| The XDG locations | `Locations` | `paths.Locations`, struct field `locations` |
| The user-loaded config | `Config` | `config.Config`, struct field `settings` (see below) |

If a future contributor writes `SessionId`, `session-id`, `sess_id`, or `sid`, they have violated this table. Reject the rename in code review.

## Go-specific conventions

Go has its own readability conventions that override or qualify some of the skill's rules. The resolutions below are intentional.

### Identifier case

- **Exported** (visible across packages): `PascalCase`. `Conversation`, `Provider`, `ListProjects`, `IsKnown`.
- **Unexported** (package-private): `mixedCaps`. `adapterName`, `parseStream`, `firstSessionFile`.
- **Constants follow the same exported/unexported rule, not `UPPER_SNAKE_CASE`.** The skill's default for constants is `UPPER_SNAKE_CASE`, but Go's standard library is uniformly `MixedCaps`: `time.RFC3339`, `os.O_RDONLY`, `tls.VersionTLS13`. Adopting `MAX_FINGERPRINT_RECORDS` in a Go codebase would read as foreign at every reader. **The Go convention wins here.** The skill explicitly invites this kind of override when the language convention is universal — when a rule would make the code worse for the case in front of you, say so and act on the better idea.

### Filenames

- Single-word, all lowercase, no underscores when possible: `parse.go`, `detect.go`, `cleanup.go`, `browse.go`.
- Test files match their target: `parse.go` → `parse_test.go`. The underscore here is required by the Go toolchain — it is the one place underscores in filenames are accepted.
- Multi-word filenames use the same case as Go identifiers when needed: `delete_plan.go` is acceptable when the concept itself contains two words. Pick one form per package and stay consistent.

### Package names

- Short, single word, all lowercase, no underscores. Match the folder name.
- The package name is the prefix every external caller types — `contracts.Conversation`, `steps.Markdown`. Pick names that read well in that compound form.
- We use `contracts` rather than `domain` or `model`, and `steps` rather than `transforms`, because those are the words the engineering-and-writing-standards skill uses for the same layers — vocabulary consistency with the engineering reference wins over generic alternatives.
- We use `composition` rather than `application` or `service` because hexagonal architecture literature calls it the application core, and `composition` reads more concretely: it composes adapters and steps.

### Receivers

- Single-letter receiver tied to the type: `c` for `Conversation`, `m` for `Message`, `p` for `Provider`, `a` for `App`.
- Pick one form per type — value (`c Conversation`) or pointer (`a *App`) — and stay with it. chronicle uses value receivers for plain data types (`Conversation`, `StorageVersion`) and pointer receivers for stateful coordinators (`App`, `*claude.Provider`).

### Local variable names

The skill forbids the cryptic-shorthand cluster `cfg`, `mgr`, `svc`, `bldr`. Idiomatic Go uses some of these everywhere. Where they collide:

| Idiomatic Go | What we use in chronicle | Why |
|---|---|---|
| `cfg` | `settings` (as a struct field) or `config` (as a local) | Full word. `cfg` is cryptic to a reader who has not memorized Go's shorthand. |
| `mgr`, `svc`, `bldr` shortened | full word every time | Same. |
| `sb` (`strings.Builder`) | `builder` | Same. |
| `rec` (a record from a JSON line) | `record` | Same. |
| `enc` (`json.NewEncoder`) | `encoder` | Same. |
| `fsys` (`fs.FS`) | **`fsys` is OK** | Not a true abbreviation — it is a workaround so the variable does not shadow the `fs` package name. |
| `id`, `ID` (an identifier) | **`id`, `ID` are OK** | Universally readable. Used by `os`, `net/http`, `database/sql`. |
| `err` for errors | **`err` is OK** | Universal in Go. Eliminating it would make every error check unreadable. |
| `t *testing.T` in tests | **`t` is OK** | Universal Go testing convention. |

The principle: **abbreviations that are universal to anyone reading Go code (`err`, `t`, `id`) stay**. Abbreviations that are merely common (`cfg`, `mgr`, `sb`) get the full word.

### Function naming

- Constructors are `New` or `NewSomething`. `claude.New()` returns a `*claude.Provider`. `config.Defaults()` returns the zero-config baseline. Multiple constructors use named factories: `composition.NewForTest`.
- Methods on a type read as verb-phrases: `provider.Detect(...)`, `app.ReadSession(...)`, `conversation.IsAbandoned()`.
- Pure transforms in `steps/` read as noun-or-verb: `steps.Markdown(c)` (the result is Markdown), `steps.Filter(c, opts)` (the verb is "filter"), `steps.Fingerprint(inputs)` (the result is a fingerprint). The shape matches the standard library — `fmt.Sprintf` produces a string, `strings.Replace` performs replacement.
- Predicates start with `Is` or `Has`: `Conversation.IsAbandoned()`, `StorageVersion.IsKnown()`.

### Test naming

The standard chronicle test name reads as a sentence:

```
TestFunctionUnderTest_describesTheBehavior
```

`TestFirstUserPrompt_skipsMetaAndAssistant`. `TestFilter_isPure`. `TestParse_syntheticFutureKeepsUnknowns`. The behavior describes what the test asserts, not which branch it covers. `TestParse_branchOne` does not read as a sentence and is wrong. This matches the skill's rule that test names describe behavior, not implementation.

## Cross-layer vocabulary

When a noun crosses layers, it stays the same word at every checkpoint.

- A user sees the JSON field `session_id` in `chronicle list` output.
- Internally that lives in `SessionSummary.ID`, typed as `contracts.SessionID`.
- It is keyed in the filesystem as the JSONL filename `<sessionId>.jsonl`.
- The doctor view labels it `Session ID`.
- The format-report JSON keys it as `session_id`.

Five places, one word: `session id` (lowercased, hyphenated, or snake-cased as the surface requires). `SessionId`, `sessId`, `session-uuid`, `sid` — none of these survive review.

## When this document is wrong

If you find an established Go community convention that this document gets wrong, propose the change here first. The cost of changing a name once everything compiles is one find-and-replace and a careful commit. The cost of two names for the same concept is months of small confusion.
