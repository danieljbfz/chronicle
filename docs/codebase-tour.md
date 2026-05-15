# Codebase tour

A walkthrough of every package in chronicle, what each file does, and how they fit together. Read this once cover to cover and the rest of the source should make sense at a glance.

## The thirty-second summary

Chronicle is a Go program. You build it with `go build -o chronicle ./cmd/chronicle`. The binary reads the on-disk history of AI coding assistants (only Claude Code today, more coming) and lets you list sessions, export them as Markdown, copy them to the clipboard, and inspect what chronicle found with the `doctor` command.

The architecture follows the Hexagonal pattern, also called Ports and Adapters or Onion. The vocabulary maps cleanly to our packages.

| Concept | Where it lives |
|---|---|
| The port for read-only operations | `contracts/Provider` |
| The optional port for destructive operations | `contracts/Cleaner` |
| The adapters (one per upstream tool) | `adapters/claude/`, `adapters/copilotchat/`, `adapters/copilotagent/`, future `adapters/cursor/`, ... |
| The application core (orchestrates the adapters) | `composition/` |
| The pure transforms (filter, render, fingerprint) | `steps/` |
| The driving sides (the CLI today, TUI and web later) | `cmd/chronicle/` |

The `Provider` interface covers everything a read-only chronicle install needs. Optional capability interfaces (`Cleaner`, `MemoryStore`, `GlobalMemoryStore`, `GlobalConfig`, `Resumable`) layer destructive or tool-specific operations on top. Adapters implement whichever optional interfaces match their upstream tool. The Claude adapter implements all of them today. The Copilot Chat adapter implements `Cleaner`. The Copilot agent adapter is read-only for now. Composition uses a type assertion (`if c, ok := provider.(Cleaner); ok`) to find which adapters support each optional capability. This keeps the base contract small and makes every destructive surface visible in the type system: any code that touches a capability interface is doing something the read-only contract did not allow.

The strict rule is that imports only ever flow downhill. The contracts package is a leaf and depends on nothing inside chronicle. The adapters depend on contracts. The steps depend on contracts. Composition depends on adapters and steps. The CLI depends on composition. Nothing ever flows the other way.

## Repository layout

```
chronicle/
├── cmd/chronicle/        the binary's main package and CLI subcommands
├── adapters/             one folder per upstream tool, plus the registry
│   ├── all.go            the registry — one line per provider
│   ├── claude/           the Claude Code adapter
│   ├── copilotchat/      the VS Code Copilot Chat extension adapter
│   └── copilotagent/     the @github/copilot-sdk runtime adapter
├── composition/          the application core, the only layer that touches disk
├── contracts/            shared domain types and the Provider interface
├── internal/             helpers private to chronicle
│   ├── config/           TOML config loader
│   └── paths/            XDG path resolver
├── steps/                pure transforms over the contract types
├── docs/                 design spec, research, this tour, the Go primer
└── README.md             the user-facing readme
```

A few things worth noticing:

- The folders sit directly under the repo root. Idiomatic Go puts packages at the module root rather than under a `src/` directory. The standard library, kubernetes, prometheus, and most well-known Go projects do the same. The import path for a package is its folder path, so `src/contracts` would just become noise in every import line.
- The `internal/` folder is special. The Go compiler enforces that anything inside an `internal/` directory can only be imported by code in the same module. That makes `internal/paths` and `internal/config` private to chronicle by guarantee, not by convention.
- The `cmd/<binary-name>/` convention is the Go standard for "this is where the main package lives." If chronicle ever ships a second binary (say a server), it would land at `cmd/chronicle-server/`.

## File-by-file walkthrough

Each row below is one Go file with a one-line description of its job.

### `contracts/`

The leaf layer. Pure types, no I/O, no imports of any other chronicle package.

| File | Job |
|---|---|
| `ids.go` | The named identifier types (`ProjectID`, `SessionID`, `MessageID`) and the `Role` constants. |
| `block.go` | The `Block` interface and its concrete implementations (`TextBlock`, `ThinkingBlock`, `ToolUseBlock`, `ToolResultBlock`, `ImageBlock`, `UnknownBlock`). |
| `message.go` | The `Message` struct that represents one turn in a conversation. |
| `conversation.go` | The `Conversation` struct plus the `FirstUserPrompt` and `IsAbandoned` helpers. |
| `storage_version.go` | The `StorageVersion` and `Capabilities` structs that adapters use to advertise what they understand about the storage they detected. |
| `project.go` | The `Project` and `SessionSummary` structs used by listing pages. |
| `delete_plan.go` | The `DeletePlan` and `DeleteItem` structs the cleanup feature will use. |
| `provider.go` | The `Provider` interface every adapter has to satisfy plus the optional `Cleaner` interface. The two architectural seams. |
| `provider_test.go` | Compile-time checks proving a stub type satisfies `Provider` alone, and a separate stub satisfies both `Provider` and `Cleaner`. |
| `block_test.go` | A compile-time check that every block type satisfies the `Block` interface, plus pin-down tests for the `Role` constants. |
| `conversation_test.go` | Behaviour tests for `FirstUserPrompt` and `IsAbandoned`. |

### `adapters/`

One folder per upstream tool, plus a registry that ties them all together.

| File | Job |
|---|---|
| `all.go` | The provider registry. The `All()` function returns one `Factory` per provider. Adding a new tool to chronicle is a one-line edit here plus a new sibling folder. The Copilot factory returns one `Entry` per detected install (regular VS Code, VS Code Insiders), so `Factory` returns a slice instead of a single `Entry`. |

### `adapters/claude/`

The Claude Code adapter. Reads `~/.claude` and turns its JSONL session files into normalized `Conversation` values.

| File | Job |
|---|---|
| `doc.go` | The package-level documentation. Describes the directory layout under `~/.claude` and what the package implements today. |
| `detect.go` | Storage version detection. Walks the projects directory, reads up to two hundred records from the first session file, and computes a fingerprint. Maps known fingerprints to internal version names like `claude-1.0`. |
| `parse.go` | The JSONL parser. Reads one session file end to end and produces a `Conversation`. Handles every kind of record and every kind of content block. Preserves anything it does not recognize as an `UnknownBlock`. |
| `provider.go` | The `Provider` implementation. Implements `Name`, `Detect`, `ListProjects`, `ListSessions`, `ReadSession`. |
| `cleanup.go` | The `Cleaner` implementation. `PlanDelete` returns the cascade plan for one session (the `.jsonl` plus `file-history/`, `tasks/`, `session-env/`, and `sessions/` siblings). `PlanOrphanScan` finds sibling files whose owning session no longer exists. |
| `errors.go` | The typed `Error` value the package returns from every public function. Carries `Op`, `Path`, and `Err` fields, supports `errors.Is` and `errors.As`. |
| `detect_test.go` | Behaviour tests for the detection logic, against in-memory fixtures. |
| `parse_test.go` | Behaviour tests for the parser, including the resilience canary. |
| `provider_test.go` | Behaviour tests for the `Provider` implementation against a hand-built fake filesystem, plus the test that confirms `*Provider` satisfies `contracts.Cleaner`. |
| `cleanup_test.go` | Behaviour tests for the cascade map: every sibling cascades, missing siblings are dropped silently, and orphan-scanning produces the right set. |
| `errors_test.go` | Behaviour tests for the typed error: format, `Unwrap`, `errors.Is`, `errors.As`. |
| `testdata/v1_0/` | Real-shape session fixtures used by the parser and provider tests. |
| `testdata/synthetic_future.jsonl` | The canary fixture. Contains an unknown record type and an unknown content kind. The test that consumes it asserts both survive parsing as `UnknownBlock` values. |
| `testdata/README.txt` | Human-readable explanation of every fixture and what it tests. |

### `adapters/copilotchat/`

The VS Code Copilot Chat extension adapter. Reads `workspaceStorage/<hash>/chatSessions/` (per-workspace chats) and `globalStorage/emptyWindowChatSessions/` (chats from folder-less VS Code windows). One folder, very different storage shape from Claude.

The big difference: each Copilot Chat session file is an event log, not a stream of independent records. The first line is a full snapshot, and every line after that is a small patch that mutates the snapshot. Reading a session means replaying every line in order. The `eventlog.go` file does the replay.

| File | Job |
|---|---|
| `doc.go` | Package-level documentation. Describes the VS Code storage layout and the event-log replay model. |
| `workspace.go` | Workspace decoding. Reads `workspace.json` to map an opaque hash like `0769784b...` back to the friendly project name. Defines the synthetic "(no workspace)" project for empty-window chats. |
| `eventlog.go` | The event-log replayer. Reads kind-0 snapshots and applies kind-1 (set field) and kind-2 (append to array) patches in order. Returns the reconstructed snapshot plus a list of any unknown event kinds it saw. |
| `parse.go` | Turns the replayed snapshot into a `Conversation`. Each entry in the snapshot's `requests` array becomes one user message and one assistant message. Recognizes markdown, thinking blocks, tool invocations, and a handful of UI-only response parts that get dropped. Reads `inputState.selectedModel.identifier` onto `Conversation.Model` so the by-model summary works for this adapter. |
| `detect.go` | Storage version detection. Walks workspaces and falls back to empty-window chats. Same fingerprinting algorithm as Claude. |
| `provider.go` | The `Provider` implementation. Knows how to list projects (one per workspace plus the synthetic empty-window bucket), list sessions, and read one session by id across both workspace and empty-window storage. |
| `cleanup.go` | The `Cleaner` implementation. `PlanDelete` covers both kinds of session (workspace-bound and folder-less window). `PlanOrphanScan` finds `chatEditingSessions/` directories whose owning chat is gone. |
| `errors.go` | The typed `Error` value, mirroring the Claude adapter's shape so both packages return errors with the same structure. |
| `*_test.go` | Behaviour tests for each piece, with the resilience canary in `eventlog_test.go` and `parse_test.go`, and the cleanup tests in `cleanup_test.go`. |
| `testdata/v3/` | Real-shape session fixtures for VS Code Copilot Chat schema version 3. |
| `testdata/synthetic_future.jsonl` | The Copilot Chat canary. Contains an unknown event kind in the middle of the stream and an unknown response part inside a request. Both must survive parsing. |

### `adapters/copilotagent/`

The GitHub Copilot agent runtime adapter. Reads `~/.copilot/session-state/<sessionId>/events.jsonl`, which is what the `@github/copilot-sdk` `LocalSessionManager` writes when the agent runs from any frontend (standalone Copilot CLI, VS Code's agent mode, or any SDK consumer). The data has zero overlap with the chat extension's storage on disk, which is why this is a distinct adapter package rather than a version of `copilotchat`.

The storage shape is a typed event envelope per line. Every event has a `type` field, a `data` payload whose schema depends on the type, and envelope fields (`timestamp`, `id`, `parentId`). Known types include `session.start`, `user.message`, `assistant.message`, `tool.execution_start`, `tool.execution_complete`, and `session.shutdown`. Unknown types survive as `UnknownBlock` values inside a meta message, the same resilience rule every adapter follows.

| File | Job |
|---|---|
| `doc.go` | Package-level documentation. Lists the directory layout under one session folder, the known event types, and the relationship to the Copilot Chat adapter. |
| `detect.go` | Storage version detection. Returns the single known version when `session-state/` exists, and an unknown-version sentinel when the directory is missing. |
| `parse.go` | The event-stream parser. Reads `events.jsonl` line-by-line, folds the events into a `Conversation`, and reads `selectedModel` from the `session.start` event onto `Conversation.Model`. Joins `tool.execution_start` and `tool.execution_complete` events into matching `ToolUseBlock` and `ToolResultBlock` pairs. |
| `provider.go` | The `Provider` implementation. Surfaces every session under one synthetic `agent-sessions` project, because the runtime stores sessions in a flat list rather than grouped by working directory. |
| `errors.go` | The typed `Error` value, same shape as the other two adapters. |
| `provider_test.go` | Every behaviour test for the package: detection, listing, reading, the model wiring, and the unknown-event-type canary. |

### `steps/`

Pure transforms. No I/O, no environment, no time. The easiest layer to test.

| File | Job |
|---|---|
| `fingerprint.go` | The `Fingerprint` function that turns a list of record shapes into a short hex hash. The detection layer uses this to decide whether the storage matches a known version. |
| `filter.go` | The `Filter` function that strips tools, thinking, meta records, or sub-agent traffic from a `Conversation`, based on the flags the user set. |
| `export.go` | The `Markdown` function that renders a `Conversation` as a Markdown document. |
| `clipboard.go` | The OSC 52 escape-sequence helper that copies text to the system clipboard. Works over SSH because the escape bytes travel as part of the terminal stream. |
| `*_test.go` | Behaviour tests for each transform. |

### `composition/`

The application core. The only layer that talks to the real filesystem in production.

| File | Job |
|---|---|
| `browse.go` | The `App` type, its `New` constructor, and the read-only methods every entrypoint calls. `ListProjects`, `ListSessionsAll`, `ReadSession`. Plus `NewForTest` so test code can build an `App` from fake providers. `New` runs every provider's `Detect` in parallel through `errgroup`. |
| `doctor.go` | The `Doctor()` method returns one `ProviderHealth` per detected provider for the `chronicle doctor` command. The result splits errors and warnings into separate slices so the renderer and any consuming script can branch on severity without parsing message text. |
| `clean.go` | The cleanup orchestrator. `PlanCleanup` finds sessions matching a category like "abandoned" across every provider that supports cleanup, and `ExecuteCleanup` routes each plan through the trash subsystem. Today only the abandoned category is implemented. |
| `trash.go` | The trash subsystem. `Trash` moves the items in a deletion plan into a fresh trash entry with a manifest. `TrashList`, `TrashRestore`, and `TrashEmpty` cover the rest of the entry lifecycle. The `moveFileOrDir` helper handles cross-filesystem moves through a copy+remove fallback. |
| `browse_test.go` | Behaviour tests for the `App` methods, using a `fakeProvider` that satisfies the `Provider` interface without touching disk. |
| `trash_test.go` | Behaviour tests for every trash operation, including the round-trip through restore and the retention-window check on empty. |
| `integration_test.go` | End-to-end test that runs `composition.New()` against a real (temporary) home directory with a real fixture session inside. Exercises the full wiring path that the unit tests skip. |

### `internal/paths/`

Filesystem path resolution for chronicle's own config and data directories.

| File | Job |
|---|---|
| `paths.go` | The `Locations` struct and the `Resolve` function. Honours the `CHRONICLE_HOME` env override that the test suite uses. |
| `paths_test.go` | Behaviour tests for the resolver and the env override. |

### `internal/config/`

User configuration.

| File | Job |
|---|---|
| `config.go` | The `Config` struct (with all its nested subsections), the `Defaults` function, and the `Load` function. Reads `~/.config/chronicle/config.toml` and merges over the defaults. |
| `config_test.go` | Behaviour tests for the loader, including the missing-file and malformed-file cases. |

### `cmd/chronicle/`

The binary. Each subcommand lives in its own file so its flags and run function stay together.

| File | Job |
|---|---|
| `main.go` | The `main` function, the cobra root command, and the small helpers (`fail`, `fmtTime`) shared by the subcommands. |
| `list.go` | The `chronicle list` subcommand. Emits one JSON line per session for shell pipelines. |
| `export.go` | The `chronicle export <id>` subcommand. Reads a session, applies the user's filters, and writes Markdown to a file or stdout. |
| `copy.go` | The `chronicle copy <id>` subcommand. Same Markdown pipeline as `export`, but writes the OSC 52 escape sequence so the result lands in the system clipboard. |
| `doctor.go` | The `chronicle doctor` subcommand. Renders the result of `App.Doctor()` as text or JSON. |
| `clean.go` | The `chronicle clean` subcommand and its category subcommands (`abandoned`, `orphans`, `stale`, `dangling`). Defaults to dry-run. The `--apply` flag is the explicit opt-in to actually move files. |
| `trash.go` | The `chronicle trash` subcommand and its three children: `list`, `restore <id>`, and `empty [--force]`. The user reaches restorable entries through the IDs that `trash list` prints. |
| `*_test.go` | Behaviour tests for the subcommand wiring, including a fake provider for the export pipeline. |

## The dependency graph

The arrows point in the direction the import goes. Every arrow goes downhill.

```
                    cmd/chronicle  (the binary)
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

- **Adapters never import each other.** `adapters/claude` is sealed off from any future `adapters/copilot`. They both depend on `contracts`, and the registry in `adapters/all.go` is the only place that knows about both.
- **Adapters never import composition.** A leaking import from a low layer to a high layer would create a cycle and would make adapters depend on the application core, which defeats the whole point of the seam.
- **Steps depend only on contracts.** No I/O, no environment, no clock. That is what makes `steps` the easiest layer to test.
- **Composition is the only layer that opens files in production.** Adapters are handed an `fs.FS` value and never call `os.Open` themselves. That single discipline is what lets the test suite swap in an in-memory filesystem without monkeypatching anything.

## How a request flows through the system

Concrete example: the user runs `chronicle export <session-id> --no-tools`.

1. **Cobra parses the command line.** `cmd/chronicle/main.go` builds the root command, finds the `export` subcommand, parses the `--no-tools` flag, and calls the subcommand's `RunE` function.
2. **The subcommand builds the App.** `composition.New()` runs. It resolves the filesystem paths via `internal/paths`, loads the config via `internal/config`, walks `adapters.All()` to build one `Entry` per enabled provider, and runs `Detect` on each one.
3. **The subcommand asks the App for the session.** `app.ReadSession(id)` walks each registered provider in turn until one of them recognizes the identifier. The Claude provider's `ReadSession` calls `parse.go`'s `readSessionFile`, which opens the file and produces a `Conversation`.
4. **The subcommand applies the filters.** `steps.Filter` returns a copy of the conversation with the tool blocks dropped. The function is pure: the original conversation is untouched.
5. **The subcommand renders Markdown.** `steps.Markdown` walks the filtered conversation and produces a `string`.
6. **The subcommand writes the result.** Either to stdout or to the file the user named with `-o`.

Notice that nothing in the request path knows about JSONL, fingerprints, or `~/.claude`. The CLI deals in `Conversation` values. The Claude adapter is the only thing in the binary that knows about Claude's storage shape. Adding a Copilot adapter later means one new folder under `adapters/`, one new line in `adapters/all.go`, and zero changes anywhere else.

## How `fs.FS` makes the test suite cheap

`fs.FS` is the Go standard library's interface for a read-only filesystem. The interface is tiny: one method, `Open(name string) (fs.File, error)`.

In production, composition passes `os.DirFS("/home/user/.claude")` to the adapter. `os.DirFS` is the standard library's adapter that turns a real directory into an `fs.FS`.

In tests, the suite passes `fstest.MapFS{"projects/p/s.jsonl": &fstest.MapFile{Data: ...}}`. `fstest.MapFS` is the standard library's in-memory filesystem. It is a `map[string]*fstest.MapFile` that satisfies `fs.FS`.

The adapter cannot tell the difference between the two. Production code reads real files. Test code reads fixture content. Same code path, no mocking, no monkeypatching, no patching of imports.

This is the single most important pattern in chronicle and it explains why every test in the project runs in milliseconds.

## The resilience contract in one breath

Upstream tools change their on-disk formats. Chronicle has to keep working when they do, instead of crashing or losing data. The contract has four rules.

1. **Detect.** Every adapter computes a short fingerprint of the storage shape it sees. Known fingerprints map to internal version names. Unknown fingerprints set `Version = "unknown"` and the system stays in read-only mode.
2. **Parse tolerantly.** Record types and content kinds the adapter does not recognize become `UnknownBlock` values, never silent drops. The renderer surfaces them so the user sees what happened.
3. **Capability flags.** The user interface checks `Capabilities` flags to decide which features to show, never the version string. This way, adding a fingerprint to the lookup table does not require a new chronicle release for the UI to keep working.
4. **Warn.** When detection produces an unknown fingerprint, chronicle attaches a banner to the affected views and the destructive operations require an extra confirmation.

Each adapter ships with a synthetic-future fixture that contains a fabricated unknown record type. The test that consumes that fixture is the canary. If anyone ever changes the parser in a way that drops unknowns, the canary fails immediately.

## What's not built yet

Today, chronicle is the read-only Claude and Copilot tool. The forward-looking pieces are stubbed in the architecture but not yet implemented.

- **Cleanup and trash.** `PlanDelete` and `PlanOrphanScan` on both adapters return `ErrNotImplemented`. The cascade-delete map (which sibling folders follow a session into the trash) is documented in the research notes but not yet executed in code.
- **Cursor adapter.** Cursor is a VS Code fork with its own chat storage shape (mostly inside `state.vscdb`). It is not stubbed yet. Adding it would be a new sibling folder under `adapters/` plus one line in the registry.
- **Antigravity and other future tools.** Same shape as Cursor: new folder, one registry line.
- **Terminal UI and web frontend.** The composition layer already exposes everything they would need. The `internal/config` package has the `TUIConfig` and `WebConfig` blocks ready. The actual code is future work.

## How to add a new provider (the recipe)

If you wanted to add Cursor support tomorrow, here is the entire change list:

1. Create `adapters/cursor/`. Copy the file structure from `adapters/claude/` (`doc.go`, `detect.go`, `parse.go`, `provider.go`, `cleanup_stub.go`).
2. Implement the four methods that read from disk (`Detect`, `ListProjects`, `ListSessions`, `ReadSession`). Reuse `steps/fingerprint.go` for detection. Use `contracts.UnknownBlock` for any content shape you do not recognize.
3. Add at least one real-shape fixture to `adapters/cursor/testdata/` and one synthetic-future fixture to satisfy the resilience canary.
4. Add a `CursorConfig` struct to `internal/config/config.go` and a default value in `Defaults()`.
5. Add a `cursorFactory` function to `adapters/all.go` and one entry to the `All()` slice.

Five steps, all additive. Composition does not change. The CLI does not change. The contracts do not change. That is what the architecture is for.

## How to read this document later

The fastest way to refresh your memory is to skim the dependency graph, then re-read the file-by-file walkthrough. If you are about to make a change, read the layer that owns the change and the layer immediately above and below it. Reading the whole document is only worth doing once.
