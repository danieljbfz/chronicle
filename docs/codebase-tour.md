# Codebase tour

A walkthrough of every package in chronicle, what each file does, and how they fit together. Read this once cover to cover and the rest of the source should make sense at a glance.

## Contents

- [The thirty-second summary](#the-thirty-second-summary)
- [Architecture](#architecture)
- [Repository layout](#repository-layout)
- [File-by-file walkthrough](#file-by-file-walkthrough)
  - [`contracts/`](#contracts)
  - [`adapters/`](#adapters)
  - [`steps/`](#steps)
  - [`composition/`](#composition)
  - [`internal/`](#internal)
  - [`cmd/chronicle/`](#cmdchronicle)
  - [`cmd/chronicle/tui/`](#cmdchronicletui)
- [The terminal UI](#the-terminal-ui)
- [The dependency graph](#the-dependency-graph)
- [How a request flows through the system](#how-a-request-flows-through-the-system)
- [How `fs.FS` makes the test suite cheap](#how-fsfs-makes-the-test-suite-cheap)
- [The resilience contract in one breath](#the-resilience-contract-in-one-breath)
- [How to add a new provider](#how-to-add-a-new-provider)

## The thirty-second summary

Chronicle is a Go program. You build it with `go build -o chronicle ./cmd/chronicle`. The binary reads the on-disk history that AI coding assistants leave behind — Claude Code and two GitHub Copilot products today — and lets you browse, search, export, and clean that history. Running `chronicle` with no arguments opens an interactive terminal UI. Every subcommand (`list`, `export`, `stats`, `doctor`, `clean`, and the rest) is the scripting surface.

The architecture follows the Hexagonal pattern, also called Ports and Adapters or Onion. The vocabulary maps cleanly to the packages.

| Concept | Where it lives |
|---|---|
| The port for read-only operations | `contracts.Provider` |
| The optional capability ports | `contracts.Cleaner`, `MemoryStore`, `GlobalMemoryStore`, `GlobalConfig`, `Resumable` |
| The adapters (one per upstream tool) | `adapters/claude/`, `adapters/copilotchat/`, `adapters/copilotagent/` |
| The pure transforms (filter, render, search, fingerprint) | `steps/` |
| The application core (orchestrates the adapters) | `composition/` |
| The driving sides (the CLI and the TUI) | `cmd/chronicle/` |

## Architecture

The `Provider` interface covers everything a read-only chronicle install needs — detect the storage, list projects, list sessions, read one session. Five optional capability interfaces layer the destructive and tool-specific operations on top: `Cleaner` (delete sessions, scan for orphans), `MemoryStore` and `GlobalMemoryStore` (per-project and user-global memory files), `GlobalConfig` (stale entries in a tool's global config file), and `Resumable` (re-open a session in its original tool).

Adapters implement whichever optional interfaces match their upstream tool. The Claude adapter implements all five. The Copilot Chat adapter implements `Cleaner`. The Copilot agent adapter is read-only. Composition discovers each capability with a type assertion (`if c, ok := provider.(contracts.Cleaner); ok`). This keeps the base contract small and makes every destructive surface visible in the type system: any code that touches a capability interface is doing something the read-only contract does not allow.

The strict rule is that imports only ever flow downhill. The contracts package is a leaf and depends on nothing inside chronicle. The adapters depend on contracts. The steps depend on contracts. Composition depends on adapters and steps. The binary depends on composition. Nothing ever flows the other way.

## Repository layout

```
chronicle/
├── cmd/chronicle/        the binary's main package, CLI subcommands, and the TUI
├── adapters/             one folder per upstream tool, plus the registry
│   ├── all.go            the registry — one entry per provider
│   ├── claude/           the Claude Code adapter
│   ├── copilotchat/      the VS Code Copilot Chat extension adapter
│   └── copilotagent/     the @github/copilot-sdk runtime adapter
├── composition/          the application core, the only layer that touches disk
├── contracts/            shared domain types and the Provider interface
├── internal/             helpers private to chronicle
│   ├── config/           TOML config loader
│   └── paths/            XDG path resolver
├── steps/                pure transforms over the contract types
├── docs/                 this tour, the Go primer, the roadmap, and research
└── README.md             the user-facing readme
```

A few things worth noticing:

- The folders sit directly under the repo root. Idiomatic Go puts packages at the module root rather than under a `src/` directory. The import path for a package is its folder path, so a `src/` prefix would be noise in every import line.
- The `internal/` folder is special. The Go compiler enforces that anything inside an `internal/` directory can only be imported by code in the same module, which makes `internal/paths` and `internal/config` private to chronicle by guarantee, not by convention.
- The `cmd/<binary-name>/` convention is the Go standard for where a main package lives. A second binary would land at its own `cmd/<name>/`.

## File-by-file walkthrough

Each row below is one Go file with a one-line description of its job. Test files (`*_test.go`) sit next to the file they cover and are omitted from the tables.

### `contracts/`

The leaf layer. Pure types, no I/O, no imports of any other chronicle package.

| File | Job |
|---|---|
| `ids.go` | The named identifier types (`ProjectID`, `SessionID`, `MessageID`) and the `Role` constants (`RoleUser`, `RoleAssistant`, `RoleSystem`). |
| `block.go` | The `Block` interface and its concrete kinds: `TextBlock`, `ThinkingBlock`, `ToolUseBlock`, `ToolResultBlock`, `ImageBlock`, `DocumentBlock`, `FileContextBlock`, `AwaySummaryBlock`, and `UnknownBlock`. |
| `message.go` | The `Message` struct: one turn, carrying its blocks plus the `IsMeta`, `IsSidechain`, and `Model` metadata the filter step reads. |
| `conversation.go` | The `Conversation` struct plus the `FirstUserPrompt` and `IsAbandoned` helpers. |
| `listing_title.go` | `Conversation.ListingTitle`, the cascade that always returns a recognizable label for a listing row even when a session opens with no real user text. |
| `project.go` | The `Project` struct used by the project-listing surfaces. |
| `session_ref.go` | The `SessionRef` parse-free handle a provider returns from `ListSessionRefs`, carrying a session's identifiers, on-disk size, and modification time without reading its content. |
| `session_summary.go` | The `SessionSummary` listing view and `NewSessionSummary`, the one place that maps a parsed `Conversation` plus its `SessionRef` into a summary. |
| `storage_version.go` | The `StorageVersion` and `Capabilities` structs adapters use to advertise what they understood about the storage they detected. |
| `delete_plan.go` | The `DeletePlan` and `DeleteItem` structs the cleanup feature produces. |
| `provider.go` | The required `Provider` interface plus the optional `Cleaner` interface. |
| `memory.go` | The optional `MemoryStore` and `GlobalMemoryStore` interfaces and the `MemoryFile` value they return. |
| `global_config.go` | The optional `GlobalConfig` interface for stale entries in a tool's global config file. |
| `resume.go` | The optional `Resumable` interface and the `ResumePlan` it returns. |

### `adapters/`

One folder per upstream tool, plus a registry that ties them together.

| File | Job |
|---|---|
| `all.go` | The provider registry. `All()` returns one `Factory` per provider, and adding a tool to chronicle is one entry here plus a sibling folder. A factory returns one `Entry` per detected install, so a provider that ships in several variants (regular VS Code, VS Code Insiders) registers each one. |

#### `adapters/claude/`

The Claude Code adapter. Reads `~/.claude` and turns its JSONL session files into normalized `Conversation` values. Implements every optional capability.

| File | Job |
|---|---|
| `doc.go` | The package documentation. Describes the directory layout under `~/.claude` and what the package implements. |
| `detect.go` | Storage-version detection. Walks the projects directory, reads a bounded prefix of the first session file, and computes a fingerprint. Maps known fingerprints to internal version names like `claude-1.0`. |
| `parse.go` | The JSONL parser. Reads one session file end to end and produces a `Conversation`. A queued user command surfaces as an ordinary user turn, editor-attached file content surfaces as a system-role `FileContextBlock`, step-away summaries and attached documents surface as their own blocks, pure bookkeeping records are dropped, and anything unrecognized is preserved as an `UnknownBlock`. |
| `provider.go` | The `Provider` implementation: `Name`, `Detect`, `ListProjects`, `ListSessionRefs`, `SummarizeSession`, `ReadSession`, plus the `decodeProjectPath` helper that turns an encoded-cwd folder name back into a readable path. |
| `cleanup.go` | The `Cleaner` implementation. `PlanDelete` returns the cascade plan for one session — the `.jsonl`, the `<sessionId>/` companion directory of subagents and tool results, and the `file-history/`, `tasks/`, and `session-env/` siblings. |
| `orphans.go` | `PlanOrphanScan`, which finds sibling files and companion directories whose owning session no longer exists. |
| `memory.go` | The `MemoryStore` and `GlobalMemoryStore` implementations for per-project `memory/` files and the user-global `~/.claude/CLAUDE.md`. |
| `global_config.go` | The `GlobalConfig` implementation for stale project entries in `~/.claude.json`. |
| `resume.go` | The `Resumable` implementation. Builds the argv and working directory that re-open a session with `claude --resume`. |
| `errors.go` | The typed `Error` value the package returns from every public function. Carries `Op`, `Path`, and `Err`, and supports `errors.Is` and `errors.As`. |
| `testdata/` | Real-shape session fixtures plus a synthetic-future fixture that contains an unknown record type and an unknown content kind. The test that consumes it is the resilience canary. |

#### `adapters/copilotchat/`

The VS Code Copilot Chat extension adapter. Reads `workspaceStorage/<hash>/chatSessions/` (per-workspace chats) and `globalStorage/.../emptyWindowChatSessions/` (chats from folder-less windows). Each session file is an event log, not a stream of independent records: the first line is a full snapshot and every later line is a patch that mutates it.

| File | Job |
|---|---|
| `doc.go` | The package documentation. Describes the VS Code storage layout and the event-log replay model. |
| `workspace.go` | Workspace decoding. Reads `workspace.json` to map an opaque hash back to the project folder, and defines the synthetic "(no workspace)" project for empty-window chats. |
| `eventlog.go` | The event-log replayer. Applies the snapshot and the field-set and array-append patches in order, and reports any unknown event kinds it saw. |
| `parse.go` | Turns the replayed snapshot into a `Conversation`. Each entry in the snapshot's `requests` array becomes a user message and an assistant message, and `inputState.selectedModel.identifier` becomes the model. |
| `detect.go` | Storage-version detection. Walks workspaces, falls back to empty-window chats, and fingerprints the same way Claude does. |
| `provider.go` | The `Provider` implementation across both workspace and empty-window storage. |
| `cleanup.go` | The `Cleaner` implementation. `PlanDelete` covers both kinds of session. |
| `orphans.go` | `PlanOrphanScan`, which finds `chatEditingSessions/` directories whose owning chat is gone. |
| `errors.go` | The typed `Error` value, mirroring the Claude adapter's shape. |
| `testdata/` | Real-shape fixtures for schema version 3 plus the resilience canary. |

#### `adapters/copilotagent/`

The GitHub Copilot agent runtime adapter. Reads `~/.copilot/session-state/<sessionId>/events.jsonl`, what the `@github/copilot-sdk` `LocalSessionManager` writes when the agent runs from any frontend. The data has zero overlap with the Copilot Chat extension, which is why it is a distinct adapter. The storage shape is one typed event envelope per line.

| File | Job |
|---|---|
| `doc.go` | The package documentation. Lists the directory layout, the known event types, and the relationship to the Copilot Chat adapter. |
| `detect.go` | Storage-version detection. Returns the known version when `session-state/` exists and an unknown sentinel when it does not. |
| `parse.go` | The event-stream parser. Folds the events into a `Conversation`, reads `selectedModel` from the `session.start` event, and joins tool-start and tool-complete events into matching `ToolUseBlock` and `ToolResultBlock` pairs. |
| `provider.go` | The `Provider` implementation. Surfaces every session under one synthetic `agent-sessions` project, because the runtime stores sessions in a flat list rather than grouped by working directory. |
| `errors.go` | The typed `Error` value, the same shape as the other two adapters. |

### `steps/`

Pure transforms. No I/O, no environment, no clock. The easiest layer to test.

| File | Job |
|---|---|
| `fingerprint.go` | `Fingerprint` turns a set of record shapes into a short hex hash. Detection uses it to decide whether the storage matches a known version. |
| `filter.go` | `Filter` returns a copy of a `Conversation` with tool, thinking, meta, sidechain, away-summary, or file-context content removed, based on the `FilterOptions` the caller set. |
| `export.go` | `Markdown` renders a `Conversation` as a Markdown document. |
| `search.go` | The substring matcher that finds query hits in a conversation and returns the snippets around them. |
| `clipboard.go` | The OSC 52 escape-sequence helper that copies text to the system clipboard, which works over SSH because the bytes travel in the terminal stream. |

### `composition/`

The application core. The only layer that talks to the real filesystem in production.

| File | Job |
|---|---|
| `browse.go` | The `App` type, its `New` constructor, and the read methods every entrypoint calls (`ListProjects`, `ListSessionsAll`, `ReadSession`), plus `NewForTest`. `New` runs every provider's `Detect` in parallel. |
| `session_list.go` | `summariesForProject`, the listing pipeline `ListSessionsAll`, `Stats`, and `clean stale` share: it enumerates a project's sessions, serves the summary cache's hits, and parses only the misses in parallel. |
| `summary_cache.go` | The persistent per-session summary cache. Keyed on each session's size, modification time, and storage fingerprint, it serves an unchanged session's summary without re-parsing, and rebuilds from scratch if its file is missing or corrupt. |
| `search.go` | `Search` and its `SearchOptions`. Walks the providers, runs `steps.Search` over each session, and returns the matches with their snippets. |
| `stats.go` | `Stats` and its `StatsOptions`. Builds the one-screen summary: totals, per-provider rows, top projects, and the by-model breakdown. |
| `doctor.go` | `Doctor` returns one `ProviderHealth` per detected provider, splitting errors and warnings into separate slices so a caller can branch on severity without parsing message text. |
| `clean.go` | The cleanup orchestrator. `PlanCleanup` finds sessions matching a category across every provider that supports cleanup, and `ExecuteCleanup` routes each plan through the trash. |
| `trash.go` | The trash subsystem. `Trash` moves a deletion plan into a fresh trash entry with a manifest, and `TrashList`, `TrashRestore`, and `TrashEmpty` cover the rest of the entry lifecycle. |
| `fsmove.go` | The cross-filesystem move helpers the trash relies on, falling back to copy-and-remove only on a cross-device rename error. |
| `memory.go` | `ListMemories`, `ShowMemory`, `EditMemoryPath`, `CleanProjectMemory`, and the three global-memory equivalents. |
| `global_config.go` | `ListConfigProjects` and `CleanConfigProjects` for stale project entries in a tool's global config file. |
| `resume.go` | `Resume`, which returns the argv and working directory that re-open a session in its original tool. |
| `bulk_export.go` | `BulkExport` and its `BulkExportOptions`. Writes one Markdown transcript per session in a project, streaming through the renderer rather than holding every session in memory. |
| `humanfmt.go` | The small human-readable formatters shared by the entrypoints, including `HumanAge`, which renders a relative time and returns "unknown" for a zero timestamp. |

### `internal/`

| File | Job |
|---|---|
| `paths/paths.go` | The `Locations` struct and the `Resolve` function. Honours the `CHRONICLE_HOME` override the test suite uses. |
| `config/config.go` | The `Config` struct with its nested subsections, the `Defaults` function, and the `Load` function that reads the TOML config and merges over the defaults. |

### `cmd/chronicle/`

The binary. Each subcommand lives in its own file so its flags and run function stay together.

| File | Job |
|---|---|
| `main.go` | The `main` function, the cobra root command (which launches the TUI when given no subcommand), the shared helpers, and the config-boundary validation for the theme and Markdown-style settings. |
| `list.go` | `chronicle list`. Emits one JSON line per session for shell pipelines. |
| `export.go` | `chronicle export <id>` and `chronicle export --bulk <project>`. Reads a session, applies the filter flags, and writes Markdown to a file, a directory, or stdout. |
| `copy.go` | `chronicle copy <id>`. The same Markdown pipeline as `export`, written to the clipboard via OSC 52. |
| `search.go` | `chronicle search <query>`. Substring search across every provider, with snippet results and a `--json` flag. |
| `stats.go` | `chronicle stats`. Renders the summary as text or JSON, with `--by-model` for the per-model breakdown. |
| `doctor.go` | `chronicle doctor`. Renders the per-provider health report as text or JSON. |
| `resume.go` | `chronicle resume <id>`. Re-opens a session in its original tool, or prints a clear message when the provider does not support resume. |
| `memory.go` | `chronicle memory list/show/edit/clean`, with `--global` to target the user-global memory file. |
| `config.go` | `chronicle config show/edit/path` for chronicle's own TOML configuration. |
| `clean.go` | `chronicle clean` and its `abandoned`, `orphans`, and `stale` categories. Defaults to dry-run, and `--apply` is the opt-in to move files. |
| `clean_dangling.go` | `chronicle clean dangling`, which removes config entries whose project directory is gone, editing the file byte-preservingly after a backup. |
| `trash.go` | `chronicle trash list/restore/empty`. |

### `cmd/chronicle/tui/`

The terminal UI. Its own package tree, described in [The terminal UI](#the-terminal-ui).

```
cmd/chronicle/tui/
├── tui.go              Run(app, opts) error — the entry point
├── app.go              the top-level model and the section router
├── screen.go           the Screen interface and the thin per-screen adapters
├── messages.go         the cross-screen tea.Msg types
├── keys/               the shared key bindings
├── theme/              the lipgloss styles and the canonical separators
├── ui/                 the shared render components (frame, spinner)
└── screens/
    ├── sessions/       the session list
    ├── transcript/     the transcript reader
    ├── stats/          the stats view
    └── doctor/         (next screen to build)
```

## The terminal UI

The TUI is a presentation layer over `composition.App`. Every screen reads from a composition method, and every action a screen offers calls a method that already exists. The TUI never imports an adapter and never invents a domain model.

The stack is Charm v2: `bubbletea/v2` for the runtime and message loop, `bubbles/v2` for the `list`, `viewport`, `table`, `help`, and `key` components, `lipgloss/v2` for layout and styling, and `glamour` for Markdown rendering inside the transcript viewport.

The design rules below are load-bearing. They exist so that the chrome stays identical across every screen and visual drift is structurally impossible.

- **One frame, every screen.** `ui/frame.go` holds the single `Frame` renderer that every top-level screen draws through. It takes the screen's dimensions, a state (`Loading`, `Empty`, `Error`, or `Ready`), an optional status row, and the screen-curated footer bindings, and returns the rendered string with the footer anchored to the bottom. Screens own content, not chrome — a screen that needs a piece of presentation the frame does not offer extends the frame so every screen benefits, rather than hand-rolling an inline equivalent.
- **One router.** `app.go` holds the screens in a section registry and routes the active one through the `Screen` interface (`Init`, `Update`, `View`) declared in `screen.go`. A horizontal tab strip grows automatically from the registry, so a new screen is one registry entry. Number keys jump to a section directly, and Tab and Shift-Tab cycle.
- **Global keys live in the app model.** Esc is the back-then-quit ladder: it closes an open overlay, then clears the session list's filter, then quits. `?` opens the full-help overlay and is modal while open. `r` refreshes the active screen. `q` and Ctrl-C quit, guarded so typing in the filter does not exit.
- **One footer rule.** The footer is a single line that never wraps and never truncates within its budget. A screen curates two or three bindings, and the frame appends `?` and `q`. Anything that does not fit lives in the `?` overlay.
- **Canonical separators.** `theme.Separator` (` • `) joins peer items on a line, and `theme.HierarchySeparator` (` › `) joins a parent to a child. Call sites use the named values, never literal characters.
- **Single-line list rows.** Every title and project path passes through `sanitizeSingleLine` before it reaches a list delegate, because the bubbles list trusts each row to paint exactly the height it was given.
- **Accessibility bar.** Every action has a key binding, the default bindings cover both Vim and arrow conventions, no action needs a multi-key chord, status and provider markers carry text as well as colour, and the loading, empty, and error states are full sentences that name the next step.

## The dependency graph

The arrows point in the direction the import goes. Every arrow goes downhill.

```
                    cmd/chronicle  (the binary and the TUI)
                          │
                          ▼
                   composition  (orchestrates everything)
                          │
              ┌───────────┼────────────┐
              ▼           ▼            ▼
          adapters     steps      internal/{paths, config}
              │           │
              └─────┬─────┘
                    ▼
                contracts  (pure types, leaf)
```

A few rules the graph enforces:

- **Adapters never import each other.** Each adapter depends on `contracts`, and the registry in `adapters/all.go` is the only place that knows about more than one.
- **Adapters never import composition.** A leaking import from a low layer to a high layer would create a cycle and make adapters depend on the application core, which defeats the seam.
- **Steps depend only on contracts.** No I/O, no environment, no clock. That is what makes `steps` the easiest layer to test.
- **Composition is the only layer that opens files in production.** Adapters are handed an `fs.FS` value and never call `os.Open` themselves. That single discipline is what lets the test suite swap in an in-memory filesystem without monkeypatching.

## How a request flows through the system

Concrete example: the user runs `chronicle export <session-id> --no-tools`.

1. **Cobra parses the command line.** `main.go` builds the root command, finds the `export` subcommand, parses `--no-tools`, and calls the subcommand's run function.
2. **The subcommand builds the App.** `composition.New()` resolves the filesystem paths via `internal/paths`, loads the config via `internal/config`, walks `adapters.All()` to build one `Entry` per enabled provider, and runs `Detect` on each one.
3. **The subcommand asks the App for the session.** `app.ReadSession(id)` walks each registered provider until one recognizes the identifier. The Claude provider opens the file and `parse.go` produces a `Conversation`.
4. **The subcommand applies the filters.** `steps.Filter` returns a copy of the conversation with the tool blocks dropped. The function is pure, so the original conversation is untouched.
5. **The subcommand renders Markdown.** `steps.Markdown` walks the filtered conversation and produces a string.
6. **The subcommand writes the result.** Either to stdout or to the file named with `-o`.

Nothing in the request path knows about JSONL, fingerprints, or `~/.claude`. The CLI deals in `Conversation` values, and the Claude adapter is the only thing in the binary that knows about Claude's storage shape.

## How `fs.FS` makes the test suite cheap

`fs.FS` is the Go standard library's interface for a read-only filesystem. The interface is tiny: one method, `Open(name string) (fs.File, error)`.

In production, composition passes `os.DirFS("/home/user/.claude")` to the adapter. In tests, the suite passes `fstest.MapFS{"projects/p/s.jsonl": &fstest.MapFile{Data: ...}}`, the standard library's in-memory filesystem. The adapter cannot tell the difference between the two. Production code reads real files, test code reads fixture content, and they run the same code path with no mocking and no monkeypatching.

This is the single most important pattern in chronicle, and it is why every test in the project runs in milliseconds.

## The resilience contract in one breath

Upstream tools change their on-disk formats. Chronicle has to keep working when they do, instead of crashing or losing data. The contract has four rules.

1. **Detect.** Every adapter computes a short fingerprint of the storage shape it sees. Known fingerprints map to internal version names. Unknown fingerprints set `Version = "unknown"` and the system stays in read-only mode.
2. **Parse tolerantly.** Record types and content kinds the adapter does not recognize become `UnknownBlock` values, never silent drops. The renderer surfaces them.
3. **Capability flags.** The user interface checks `Capabilities` flags to decide which features to show, never the version string. Adding a fingerprint to the lookup table does not require a code change in the UI.
4. **Warn.** When detection produces an unknown fingerprint, chronicle attaches a banner to the affected views, and destructive operations require an extra confirmation.

Each adapter ships with a synthetic-future fixture that contains a fabricated unknown record type. The test that consumes that fixture is the canary: if anyone changes the parser in a way that drops unknowns, the canary fails immediately. The full reasoning lives in [`research/schema-resilience.md`](research/schema-resilience.md).

## How to add a new provider

To add a tool — Cursor, Antigravity, or any other assistant that persists conversations locally — the change list is:

1. Create `adapters/<name>/`. Follow the file structure of `adapters/claude/`: `doc.go`, `detect.go`, `parse.go`, `provider.go`, `errors.go`, plus a `cleanup.go` and `orphans.go` if the tool supports cleanup.
2. Implement the `Provider` methods that read from disk (`Detect`, `ListProjects`, `ListSessionRefs`, `SummarizeSession`, `ReadSession`). Keep `ListSessionRefs` parse-free — stat each session, do not read it — so the composition cache can skip the parse for unchanged sessions; `SummarizeSession` does the parse on a miss and returns `contracts.NewSessionSummary`. Reuse `steps/fingerprint.go` for detection and `contracts.UnknownBlock` for any content shape you do not recognize.
3. Implement whichever optional capability interfaces the tool supports.
4. Add at least one real-shape fixture to `testdata/` and one synthetic-future fixture to satisfy the resilience canary.
5. Add a default for the provider in `internal/config/config.go`.
6. Add a factory to `adapters/all.go` and one entry to the `All()` slice.

The change is additive. Composition does not change, the CLI does not change, the contracts do not change. That is what the architecture is for.
