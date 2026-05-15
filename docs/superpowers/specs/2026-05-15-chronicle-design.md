# `chronicle` — design spec

Status: draft, awaiting user review.
Author: drafted with the user, 2026-05-15.
Supersedes: nothing.
Successors: implementation plan under `docs/superpowers/plans/`.

This document is the contract for what `chronicle` is and how its parts fit together. Implementation details (which library function to call, how a specific function is shaped) belong in the implementation plan, not here. The engineering bar for both is `SKILL_PROMPT.md` at the repository root.

---

## 1. Purpose

A local, single-binary tool that browses, exports, and cleans the on-disk history that AI coding assistants leave behind. Day-one targets are Claude Code (`~/.claude/`) and GitHub Copilot Chat / Copilot CLI (`~/Library/Application Support/Code/User/...`). Cursor and JetBrains Copilot are deliberate stubs that mark the seam without shipping.

The user-visible value is fourfold.

- **Browse.** A terminal UI and a local web UI that list projects and their sessions with a readable preview pane, fuzzy search across everything, and one-key toggles to hide tool output and assistant thinking. The transcript reads like Markdown, not like raw JSON.
- **Export and share.** Copy or save the current session as clean Markdown, with filters applied, named after the first user prompt and the project. Clipboard handling works over SSH via OSC52.
- **Clean.** Detect abandoned sessions (zero real user prompts), find orphans across the sibling folders (`file-history/`, `tasks/`, `paste-cache/`, `chatEditingSessions/`, …), and remove them with a cascade-aware trash that is reversible until the trash is emptied.
- **Survive format churn.** Both upstream tools ship weekly and change their storage on a quarterly rhythm. The reader detects version, parses tolerantly, advertises capabilities, and warns on unknown shapes without crashing. See §6.

## 2. Non-goals

The following are explicitly out of scope for v1 and remain out of scope unless a future spec re-opens them.

- Editing or rewriting a session. The tool is a reader and a cleaner, never a mutator of upstream content.
- Replaying or re-running a prompt — Claude Code and Copilot own that.
- Cost analytics. `ccusage` covers this domain. We may link to it from the Doctor page, no more.
- Cloud sync, remote backup, or any network surface beyond the local web server.
- Editing a session while VS Code or Claude Code is actively writing it. We refuse destructive operations against live writers and say so plainly.
- A JS-framework web frontend. The web UI is server-rendered HTML with `htmx` for interactivity.

## 3. Architecture

### 3.1 Style

The architecture is **Hexagonal (Ports & Adapters)**, also known as the Onion or Clean style. The vocabulary maps cleanly to our packages.

- **The port** is `contracts.Provider`. It is the single interface every tool-specific reader must satisfy. The port is defined in pure domain terms — `Conversation`, `Message`, `Block` — and knows nothing about JSONL, SQLite, or the filesystem.
- **The adapters** are `adapters/claude`, `adapters/copilot`, future `adapters/cursor`, `adapters/antigravity`, and so on. Each one translates a specific tool's storage into the domain types behind the port. Adapters never import each other.
- **The application core** is `composition`. It depends only on the port. It does not know which adapters exist at compile time — it asks `adapters.All()` for the registered set.
- **The driving sides** are `cmd/chronicle` (CLI), `internal/ui/tui` (Bubble Tea), and `internal/ui/web` (templ + htmx). They depend on the application core, never on adapters directly.

The shape gives us three properties that matter for a tool that has to survive upstream churn:

- **New providers are additive.** Adding Cursor or Antigravity is one new folder under `adapters/`, one new line in `adapters/all.go`, and zero changes elsewhere. The application core and every entrypoint compile against the same `Provider` interface they always did.
- **Formats can change without breaking the UI.** When a tool changes its storage shape, only its adapter changes. Capabilities (§6) gate which UI features render — never the version string — so the UI sees a steady contract.
- **Tests are cheap.** The application core takes an `fs.FS`, not a path. Tests pass a `fstest.MapFS` filled with fixture content. The same code runs against the user's real `~/.claude` in production.

### 3.2 The layers

The code is laid out in five layers. Imports flow strictly downhill. The contract layer is a leaf — nothing inside it imports from elsewhere in the tree.

```
contracts/              ← domain types and the Provider interface. Pure.
adapters/
    claude/             ← Claude Code reader and cleanup mapper.
    copilot/            ← VS Code Copilot reader and cleanup mapper.
    cursor/             ← stub: package doc only, marks the seam.
    jetbrains/          ← stub: package doc only, marks the seam.
steps/                  ← pure transforms over the domain types.
    filter.go           ← hide tool output, thinking, meta, sidechain.
    export.go           ← Conversation → Markdown text.
    search.go           ← fuzzy match across sessions.
    fingerprint.go      ← schema fingerprinting for version detection.
    fmtreport.go        ← structured format-report builder.
composition/            ← the only layer that touches the filesystem.
    browse.go           ← list providers, projects, sessions, read a transcript.
    cleanup.go          ← plan, preview, execute a cascade delete.
    serve.go            ← start the local web server.
    trash.go            ← move-to-trash, list-trash, empty-trash.
internal/
    ui/tui/             ← Bubble Tea models per page.
    ui/web/             ← templ components and htmx handlers.
    paths/              ← XDG-correct config and trash locations.
cmd/chronicle/main.go   ← flag parsing and wiring.
```

Composition is the entrypoint for I/O. Adapters do not read the filesystem from the outside — composition hands each adapter a `fs.FS` (or an absolute root path) and asks for results. This makes adapters trivially testable against fixture trees.

`internal/ui/tui` and `internal/ui/web` are sibling consumers of composition. Neither is "primary." The TUI is the default user experience because most actions are easier to confirm in a terminal, but the web frontend exists in the same binary and reads the same data model.

Side effects live at the edges. The step layer is pure — no clock reads, no environment variables, no logging beyond returning typed errors. Composition injects clock and randomness when steps need them.

## 4. Domain contracts

The normalized types under `contracts/` are the entire vocabulary every higher layer uses. Adapters translate provider-specific shapes into these. Steps and UI know nothing about provider-specific shapes.

```go
type (
    ProjectID  string   // opaque-to-the-UI identifier, adapter-defined
    SessionID  string
    MessageID  string
    Role       string   // "user" | "assistant" | "system"
)

type Project struct {
    ID           ProjectID
    DisplayName  string    // decoded human-readable name (the project folder name)
    Path         string    // absolute path on disk, when known
    SessionCount int
    SizeBytes    int64
}

type SessionSummary struct {
    ID          SessionID
    Project     ProjectID
    StartedAt   time.Time
    LastActive  time.Time
    Title       string    // first user prompt truncated, or custom title if set
    TurnCount   int
    SizeBytes   int64     // including sibling artifacts
    Capabilities Capabilities
    Source      StorageVersion
}

type Provider interface {
    Name() string                                  // "claude", "copilot", ...
    Detect(root fs.FS) (StorageVersion, error)    // never returns nil + nil
    ListProjects(root fs.FS) ([]Project, error)
    ListSessions(root fs.FS, project ProjectID) ([]SessionSummary, error)
    ReadSession(root fs.FS, id SessionID) (Conversation, error)
    PlanDelete(root fs.FS, id SessionID) (DeletePlan, error)
    PlanOrphanScan(root fs.FS) (DeletePlan, error)
}

type StorageVersion struct {
    Adapter      string                 // matches Provider.Name()
    Version      string                 // "claude-1.0", "copilot-3", "unknown"
    Fingerprint  string                 // short hex hash, see steps/fingerprint.go
    Capabilities Capabilities
}

type Capabilities struct {
    ThreadTree         bool   // parentUuid graph (Claude) vs flat list (Copilot)
    EditingSessions    bool   // sibling working-set storage exists
    ToolInvocations    bool   // adapter recognizes tool calls in the model
    ModelMetadata      bool   // storage records which model was used per turn
    LiveWriterDetected bool   // an upstream process is actively writing here
}

type Conversation struct {
    SessionID    SessionID
    Project      ProjectID
    StartedAt    time.Time
    EndedAt      time.Time
    Title        string
    Messages     []Message
    Capabilities Capabilities
    Source       StorageVersion
}

type Message struct {
    ID         MessageID
    ParentID   MessageID           // empty for the root, supports the tree
    Role       Role                // user | assistant | system
    Timestamp  time.Time
    Blocks     []Block
    IsMeta     bool                // synthetic (slash-command echo, hook noise)
    IsSidechain bool               // sub-agent traffic, hidden by default
    Model      string              // empty when unknown
}

type Block interface{ blockMarker() }

type TextBlock     struct{ Text string }
type ThinkingBlock struct{ Text string }
type ToolUseBlock  struct{ Tool string ; Input json.RawMessage ; CallID string }
type ToolResultBlock struct{ CallID string ; Output string ; IsError bool }
type ImageBlock    struct{ MIME string ; PathOrInlineRef string }
type FileEditBlock struct{ Path string ; Before, After string ; Diff string }
type UnknownBlock  struct{ Kind string ; Raw json.RawMessage }

type DeletePlan struct {
    SessionID  SessionID            // empty for orphan-scan plans
    Items      []DeleteItem         // every path we will move to trash
    SizeBytes  int64
    Warnings   []string             // "VS Code is running" etc.
}

type DeleteItem struct {
    Path     string
    Reason   string                 // "session file", "edit history", "orphan paste"
    SizeBytes int64
}
```

`UnknownBlock` is load-bearing. The §6 resilience contract requires that the model preserve unknown content rather than discarding it.

`MessageID` and `ParentID` exist on every message even when an adapter's storage is flat (Copilot). Flat-storage adapters generate synthetic IDs by index. Capabilities tell the UI whether the tree view is meaningful.

## 5. Provider adapters

Each adapter is one folder with the same shape: `detect.go`, `parse.go`, `cleanup.go`, `doc.go`, plus a `testdata/` fixture tree per version. Adapters never call each other.

### 5.1 Claude

- Detection inspects `~/.claude/projects/<encoded-cwd>/<sessionId>.jsonl`. Fingerprint is the sorted set of record `type` values observed in the first 200 records plus a hash of the union of their key sets. Known fingerprints map to internal versions (`claude-1.0`, `claude-1.1`, …).
- Parsing reads the JSONL stream once, folds records into a tree by `parentUuid`, and produces the normalized `Conversation`. Meta records (`isMeta: true`) and sidechain traffic are tagged in `Message`, not dropped.
- Cleanup follows the cascade map documented in `docs/research/01-claude-code-storage.md` §"Cross-references between folders". Eight categories of sibling artifact follow the JSONL into the trash.

### 5.2 Copilot (VS Code)

- Detection inspects `~/Library/Application Support/Code/User/workspaceStorage/<hash>/chatSessions/<sessionId>.jsonl`. The snapshot record (`kind: 0`) carries an explicit `version` integer. We pin to that. Fingerprint augments the version with a hash of observed `response[].kind` values, so we can tell apart e.g. "v3 with `toolInvocation`" from "v3 with `toolInvocation` + new-kind-foo".
- Parsing replays the event log: apply `kind: 1` mutations and `kind: 2` appends over the `kind: 0` snapshot, then normalize `requests[]` into `[]Message`. Each `requests[i]` becomes a user-message-then-assistant-message pair sharing a synthetic `requestId`.
- Cleanup follows the cascade map in `docs/research/06-copilot-storage.md` §"Cross-references". When VS Code is detected running (lsof on the relevant `state.vscdb`, plus presence of `state.vscdb-wal` younger than a minute), destructive operations against that workspace refuse with a typed error. The user is told to quit VS Code and retry.

### 5.3 Cursor and JetBrains stubs

`adapters/cursor/doc.go` and `adapters/jetbrains/doc.go` contain a package-level doc comment that names the storage paths and the schema-detection strategy. The package compiles but exports no `Provider`. The implementation plan does not include shipping them. They exist so the next contributor who needs them adds a `parse.go` next to the doc, not a new folder.

## 6. Format resilience contract

This section is the contract enforced by the `steps/fingerprint.go` and `steps/fmtreport.go` modules, plus per-adapter `detect.go`. Full reasoning lives in `docs/research/07-schema-resilience.md`.

Every adapter guarantees four behaviors.

**Version detection.** `Detect()` always returns a non-nil `StorageVersion`. Unrecognized fingerprints set `Version = "unknown"` and `Capabilities.ThreadTree / EditingSessions / ToolInvocations / ModelMetadata` to false. The function never returns an error for an unknown shape — unknown is a normal state, not an exceptional one. Errors are reserved for two cases only: the path is unreachable (permissions, missing root, I/O failure), or no record in the file is parseable as JSON at all. A file with valid JSON whose schema we do not recognize is `Version = "unknown"`, not an error.

**Tolerant parsing.** Unknown record types in Claude or unknown response-part kinds in Copilot land in `UnknownBlock` with the raw JSON preserved. Missing optional fields default to zero values. No struct uses strict-extra-key deserialization.

**Capability flags.** The UI keys off `Capabilities`, never off `StorageVersion.Version`. Adding support for a new VS Code release that adds a kind to `response[]` is a change in `adapters/copilot/parse.go`. The UI is untouched.

**Structured warning.** When `Detect()` returns `Version = "unknown"`, the affected view renders a non-blocking banner: "This session was written by a newer version of {adapter}. Showing what we recognized." The Doctor page lists every unknown fingerprint observed in this run. A structured `format-report` is written to `~/.config/chronicle/format-reports/<date>-<adapter>-<fingerprint>.json` with the fingerprint, the file path, a small sample of unknown content, and the chronicle version. Destructive operations against a session with `Version = "unknown"` require an extra opt-in confirmation per session and are recorded in the trash entry.

## 7. The TUI

Bubble Tea / Lip Gloss / Bubbles / Glamour. Three pages reachable via top-bar tabs or `[1] [2] [3]`. Bottom bar always shows the current page's key hints in the lazygit pattern. A persistent header shows which providers were detected ("claude · copilot") and any active warnings.

### 7.1 Browse (page 1)

Three panes: projects left, sessions middle, preview right.

- Projects are decoded from the storage hash or path (`/Users/djbf/Desktop/work/foo` → `foo`) with a count badge.
- Sessions show the relative start time, the first user prompt truncated to one line, the turn count, and the on-disk size including sibling artifacts.
- The preview renders the session via Glamour with filters applied. Three filter toggles persist across runs:
  - `t` — hide tool blocks (`ToolUseBlock`, `ToolResultBlock`).
  - `k` — hide thinking blocks.
  - `m` — hide meta and sidechain.
- `/` opens fuzzy search across project name, first prompt, and full transcript text. Results highlight inline in the preview.
- `e` exports the visible (filtered) transcript to a file. `c` copies it to the clipboard via OSC52. `d` requests delete (which routes to the Cleanup page with the session pre-selected).
- `r` is the optional "resume" hotkey that runs `claude --resume <sessionId>` for Claude sessions. Disabled for Copilot, where there is no equivalent.

### 7.2 Cleanup (page 2)

A list of cleanup plans, ranked by recoverable bytes.

- "Abandoned Claude sessions (12 sessions, 220 KB)".
- "Orphaned Claude `file-history/` directories (67 MB)".
- "Orphaned `paste-cache/` entries (3 entries, 12 KB)".
- "Empty VS Code Copilot chats (1 session, 1.4 KB)".
- "Orphaned `chatEditingSessions/` (4 directories, …)".
- "`history.jsonl` entries older than 90 days (…)".

Selecting a plan opens a detail view: every path that would move to trash, its size, the reason. The bottom bar shows `space` to toggle a row, `a` to select all in this plan, `enter` to execute, `?` to read the full cascade-delete map for this category. Execution moves files to `~/.config/chronicle/trash/<timestamp>/`. The TUI then offers undo, which moves them back. `chronicle clean --empty-trash` is the manual finalize step. Default trash retention is 30 days.

### 7.3 Doctor (page 3)

Detected providers and their versions. Unknown fingerprints, with a one-key action to open the most recent format report in `$EDITOR`. Live-writer status ("VS Code is running, destructive operations refuse"). Disk usage breakdown by category, matching the live numbers in `docs/research/04-local-data-observations.md`.

## 8. The web frontend

`chronicle serve [--port 6789] [--no-open]`. Same three pages, server-rendered HTML via `templ`. Interactivity via `htmx` — no JS framework, no build step. The web frontend in v1 is **read-only**: browse, preview, search, export. Delete, clean, and trash-empty are TUI-only. The web pages render the `delete` button as a disabled tooltip explaining "available in `chronicle` TUI" — they do not lie about a feature.

Serving rules: bind to `127.0.0.1` only, never `0.0.0.0`. Random port between 6700 and 7000 if `--port` is not given. Reject any `Host:` header that is not `localhost` or `127.0.0.1` (defense against DNS-rebinding). No authentication — the binding is the boundary.

The web frontend exists primarily for the cases the TUI cannot serve well: copying out a long code block with the OS clipboard, sharing a screenshot of a clean rendered transcript, viewing on a second monitor while you work in another terminal.

## 9. The CLI surface

Beyond `tui` and `serve`, the binary exposes scriptable commands.

- `chronicle list [--provider claude|copilot] [--project PATTERN] [--since 7d]` — JSON lines of sessions, suitable for piping.
- `chronicle export <sessionId> [--no-tools] [--no-thinking] [--no-meta] [-o FILE]` — write the filtered transcript to FILE or stdout.
- `chronicle copy <sessionId> [--no-tools] [--no-thinking]` — same content to clipboard via OSC52.
- `chronicle clean --dry-run [--category abandoned|orphan-file-history|...]` — print the cascade plan as JSON. Without `--dry-run`, executes.
- `chronicle trash list|restore <id>|empty` — manage the trash directory.
- `chronicle doctor [--json]` — provider versions, unknown fingerprints, warnings.

Every command prints structured output to stdout suitable for piping. Status and progress lines go to stderr. The two streams never mix payload and status.

## 10. Cleanup model

Two rules govern every destructive operation.

**Trash first, empty later.** No file leaves the filesystem on a single keypress. The "delete" operations move to `~/.config/chronicle/trash/<timestamp>-<sessionId>/` with a `manifest.json` recording every original path, its size, and the cleanup category. `chronicle trash list` shows what is recoverable. `chronicle trash restore <id>` puts everything back. `chronicle clean --empty-trash` permanently removes entries older than the retention window (default 30 days) — this is the only command in the binary that performs an unrecoverable delete, and it always prints what it removed.

**Cascade by category, not by file.** Every cleanup plan is one of a small set of named categories. A category encodes the full cross-folder map for that kind of artifact. A user can not "delete this one JSONL but leave its `file-history/`" — that is exactly the bug we are fixing in the world. Every cleanup either deletes the full cascade or none of it.

## 11. Configuration

Single TOML file at `~/.config/chronicle/config.toml`, written on first run with sensible defaults. Every option has a flag that overrides it for the current invocation. Documented examples live in the README. The full schema lives next to its parser in `internal/config/`. Default values:

```toml
[paths]
trash_dir   = "~/.config/chronicle/trash"
reports_dir = "~/.config/chronicle/format-reports"

[trash]
retention_days = 30

[ui.tui]
theme           = "auto"     # auto | light | dark
filters_default = ["tools", "meta"]  # filter toggles on at startup
nerd_font       = "auto"     # auto-detect, fall back to ASCII

[ui.web]
host           = "127.0.0.1"
port           = 0           # 0 = pick random in 6700-7000
open_browser   = true

[providers.claude]
enabled = true
root    = "~/.claude"

[providers.copilot]
enabled = true
roots   = [
    "~/Library/Application Support/Code/User",
    "~/Library/Application Support/Code - Insiders/User",
]
refuse_when_vscode_running = true
```

XDG paths follow `os.UserConfigDir()` on each OS. The hard-coded macOS path in the default Copilot roots is replaced at runtime per platform. Linux uses `~/.config/Code/User`. Windows is out of scope for v1, but the path-detection code shape will not block it.

## 12. Distribution

Single static binary per platform: `darwin-arm64`, `darwin-amd64`, `linux-amd64`, `linux-arm64`. Built with `go build`. No `cgo`, so SQLite uses `modernc.org/sqlite` (pure Go) for reading `state.vscdb` read-only. Released via a Homebrew tap and a GitHub Releases tarball.

`brew install <tap>/chronicle` and `chronicle` is on `$PATH`. No post-install steps. The first run writes the default config and exits with a one-page summary of what was detected.

## 13. Test strategy

Tests mirror the source layout. `adapters/claude/parse_test.go` is the test for `adapters/claude/parse.go`. Test names describe behavior, never implementation.

- **Fixture corpus.** `adapters/claude/testdata/<version>/` and `adapters/copilot/testdata/<version>/` hold real-shape session files with secrets scrubbed. Each shipped version of each adapter has at least one fixture.
- **Synthetic-future fixtures.** One per adapter, fabricated to contain an unknown record type (Claude) or response kind (Copilot). The test asserts parsing succeeds, the unknown is preserved in the model, and the renderer produces an "unknown block · click to inspect" cell. **This test is the canary for the resilience contract.** Failing it without an explicit spec change means we broke a guarantee.
- **Composition.** Uses a fake `Provider` that returns fixture data. Tests assert cleanup plans, cascade categories, and the trash-then-empty flow.
- **Step purity.** Steps tests run without any I/O. A linter forbids `os.*` imports from `steps/`.
- **TUI.** `bubbletea/teatest` harness. We test that pressing keys produces the right view states, not pixel diffs.
- **Web.** `httptest` against the same composition layer. Snapshot tests of rendered HTML are allowed for the templ output.
- **No live-fs tests.** The test suite never touches the user's real `~/.claude` or VS Code storage. CI runs the same way.

## 14. Out of v1, deliberately

These are tagged `[NICE]` in `docs/research/05-feature-ideas.md` and explicitly deferred.

- ParentUuid tree visualization (flat list ships in v1).
- Per-tool filter granularity ("hide only Read/Edit, keep Bash").
- Diff view for Claude's `file-history/` snapshots.
- Stats page with time-of-day heatmap and top-tool counts.
- Editing the Markdown export inline before copy.
- Multi-session bulk export.
- Trimming `history.jsonl` and `backups/` by age (the plans are listed in Cleanup, but execution is deferred to v1.1).

## 15. Open items for the implementation plan

These are decisions the implementation plan owns, not this spec.

- Exact module split inside `cmd/chronicle/main.go` — flag library choice (likely `cobra` or `spf13/pflag`, decided in the plan).
- Concrete Bubble Tea component composition for each TUI page — paginator vs custom list, viewport sizing strategy.
- The templ component layout and CSS approach (likely PicoCSS or hand-rolled minimal CSS — deferred).
- Build, lint, and release tooling (`goreleaser`, `golangci-lint` config — deferred).
- The format-report JSON shape, finalized once we have a first synthetic-future case.
- The exact set of cleanup categories enumerated in code, finalized against the cascade-delete maps in research docs 01 and 06.

## 16. Extensibility for future providers

This section is the contract for how a future contributor (or future-you) adds a new tool to chronicle. The shape is intentionally boring — that is the point.

### 16.1 The recipe

To add a provider — Cursor, Antigravity, Codex CLI, Aider, Continue.dev, or any tool that persists conversations locally:

1. **Create the folder.** `adapters/<name>/` with five files: `doc.go` (package documentation that names the storage paths and the detection strategy), `detect.go`, `parse.go`, `cleanup.go`, and `testdata/`.
2. **Implement the `Provider` interface** (`Name`, `Detect`, `ListProjects`, `ListSessions`, `ReadSession`, `PlanDelete`, `PlanOrphanScan`). Reuse `steps/fingerprint.go` for detection and `contracts.UnknownBlock` for unrecognized shapes.
3. **Ship fixtures.** At least one real-shape session file per version, secrets scrubbed, plus one synthetic-future fixture that proves the parser tolerates an unknown record or content kind. The synthetic-future test is the canary that protects the §6 resilience contract.
4. **Register the adapter.** Add one line to `adapters/all.go` constructing the new `Provider`.
5. **Add config.** Extend `config.ProvidersConfig` with a typed subsection (`enabled`, `root` or `roots`, any provider-specific safety flags). Defaults live in `config.Defaults()`. Document the new keys in the README.

That is the entire delta. No change to the application core, the UI, the CLI subcommands, the trash model, the format-report shape, or the test scaffolding.

### 16.2 Known future targets

These are tools we know we want to support eventually. They are listed here as forward-looking notes, not as commitments — each gets its own plan when there is concrete demand.

| Tool | Storage shape | Likely complexity |
|---|---|---|
| **Cursor** | Diverged VS Code fork. Chat in `globalStorage/state.vscdb` under `workbench.panel.aichat.*` and `composer.*` keys, plus a `cursorDiskKV` table. | Moderate. SQLite primary, no JSONL. |
| **Antigravity** (Google) | Recent. Storage layout still stabilizing — needs a fresh research pass before implementation. | Unknown until researched. |
| **Codex CLI / Codex IDE plugin** (OpenAI) | Conversation logs in a local DB or JSON files; layout varies by client. | Moderate. |
| **Aider** | Per-session JSON logs under the working directory or a configured cache dir. | Low — single file format. |
| **Continue.dev** | VS Code/JetBrains extension with persistence in workspace storage. | Moderate, similar to Copilot. |
| **JetBrains Copilot** | Plugin storage under JetBrains caches; unverified format. | Moderate. |
| **Cline / Roo Cline** | VS Code extension chat history. | Moderate. |

### 16.3 What stays the same

When you add provider seven, none of these change.

- `contracts/*` — same domain types serve every provider. New tools fit `Message`, `Block`, `ToolUseBlock`, etc. without modification. If they genuinely cannot fit, the right answer is to extend the contract once for everyone — not to add per-provider escape hatches.
- `steps/*` — the filter, exporter, fingerprint, and clipboard helpers work on the normalized types and are provider-agnostic.
- `composition/*` — orchestrates whatever the registry hands it. The only place that knows the list of adapters is `adapters/all.go`.
- `cmd/chronicle/*`, `internal/ui/*` — entrypoints render `Conversation` and `SessionSummary`. They never branch on `provider.Name()` for behavior. They may show the provider name in the UI; they never key behavior off it.

### 16.4 What is allowed to change

When a new provider exposes something genuinely new — say, a provider that records per-turn cost data we cannot infer — the right move is to:

1. Add a new field to `contracts.Message` or a new `Block` type.
2. Update every existing adapter so the new field has a defined zero value or unsupported state.
3. Update the UI to render the new field, falling back gracefully when capabilities say it is unsupported.

This is the "extend the contract once for everyone" rule. Per-provider hacks are the failure mode that turns a clean abstraction into a leaky one.

## 17. Appendix: directory map at first commit

```
/
├── README.md
├── SKILL_PROMPT.md
├── LICENSE
├── go.mod
├── go.sum
├── cmd/
│   └── chronicle/
│       └── main.go
├── contracts/
│   ├── conversation.go
│   ├── message.go
│   ├── block.go
│   ├── provider.go
│   ├── storage_version.go
│   └── delete_plan.go
├── adapters/
│   ├── all.go             ← the registry: one line per provider, the extensibility seam
│   ├── claude/
│   │   ├── doc.go
│   │   ├── detect.go
│   │   ├── parse.go
│   │   ├── cleanup.go
│   │   └── testdata/
│   ├── copilot/
│   │   ├── doc.go
│   │   ├── detect.go
│   │   ├── parse.go
│   │   ├── cleanup.go
│   │   ├── eventlog.go
│   │   └── testdata/
│   ├── cursor/
│   │   └── doc.go
│   └── jetbrains/
│       └── doc.go
├── steps/
│   ├── filter.go
│   ├── export.go
│   ├── search.go
│   ├── fingerprint.go
│   └── fmtreport.go
├── composition/
│   ├── browse.go
│   ├── cleanup.go
│   ├── serve.go
│   └── trash.go
├── internal/
│   ├── ui/
│   │   ├── tui/
│   │   │   ├── app.go
│   │   │   ├── page_browse.go
│   │   │   ├── page_cleanup.go
│   │   │   └── page_doctor.go
│   │   └── web/
│   │       ├── server.go
│   │       ├── handlers.go
│   │       └── templates/
│   ├── paths/
│   │   └── paths.go
│   └── config/
│       └── config.go
├── tests/
│   └── fixtures/
│       ├── claude/
│       └── copilot/
└── docs/
    ├── README.md
    ├── research/
    │   ├── 01-claude-code-storage.md
    │   ├── 02-existing-tools-landscape.md
    │   ├── 03-tui-libraries.md
    │   ├── 04-local-data-observations.md
    │   ├── 05-feature-ideas.md
    │   ├── 06-copilot-storage.md
    │   └── 07-schema-resilience.md
    └── superpowers/
        ├── specs/
        │   └── 2026-05-15-chronicle-design.md
        └── plans/
            └── (next)
```
