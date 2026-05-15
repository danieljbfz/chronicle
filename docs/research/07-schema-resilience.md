# Schema resilience: surviving format churn

Claude Code and VS Code Copilot both ship weekly and both have changed their on-disk formats multiple times in the past two years. A tool that reads their data must assume the format will change underneath it. This document is the contract for how we handle that.

## The problem

Concrete examples from the last two years.

| Date | Tool | Change | Impact on a naive reader |
|---|---|---|---|
| Dec 2023 | VS Code | Renamed "interactive sessions" → "chat" in storage keys. | Old keys still aliased; new keys ignored. |
| Spring 2024 | VS Code | Moved persisted chats from the `interactive.sessions` row in `state.vscdb` to per-session files under `workspaceStorage/<hash>/chatSessions/`. | A reader looking only at the SQLite row sees zero history. |
| Oct 2024 | VS Code | Introduced `chatEditingSessions/<sessionId>/` for Copilot Edits. | A cleanup tool that deletes only the JSONL leaks the edit directory. |
| 2025 | VS Code | Added `toolInvocation`, `confirmation`, `undoStop` response kinds. | A renderer that switches on the response `kind` enum crashes on the new values. |
| 2026 | VS Code | Chat session files moved from JSON to JSONL event-log format with `kind: 0 / 1 / 2`. Top-level `version: 3`. | A reader expecting a single JSON object per file fails on every read. |
| Throughout | Claude Code | Added `thinking`, `tool_use`, `tool_result`, `sidechain`, `isMeta`, `queue-operation`, `permission-mode`, `file-history-snapshot`, `last-prompt` record types incrementally. | Strict parsers reject unknown types. |

The pattern is **append-mostly with occasional moves**. Format-version fields exist sometimes (Copilot's `version: 3`), but not always (Claude's records are typed without an envelope version).

## The contract

Every adapter we ship — Claude, Copilot, Cursor, future — implements four guarantees.

### 1. Version detection

Each adapter exposes a `detect(path) -> StorageVersion` function that runs before any read.

- Where a version field exists (Copilot's `version` in the snapshot record, our future Claude session-file hint), read it.
- Where no version exists (current Claude), compute a **schema fingerprint**: the sorted set of `type` values seen in the first N records, plus a hash of their key sets.
- Each adapter ships with a list of **known fingerprints** mapped to internal version codes (`claude-1.0`, `claude-1.1`, `copilot-3`, …). A fingerprint that does not match any known version triggers **degraded mode**.

### 2. Tolerant parsing

Parsers ignore unknown fields and unknown enum values rather than failing.

- Records with an unknown `type` are kept in the conversation model as `Unknown{rawType, json}` so the UI can still render them as "Unknown record · click to inspect raw JSON".
- New `kind` values inside Copilot's `response[]` follow the same rule — show as a generic block, never crash.
- All structs are parsed with permissive deserialization. We never use the equivalent of "strict" mode that errors on extra keys.

### 3. Capability flags, not version branching

Internally, every adapter advertises **capabilities** rather than versions:

- `supports.threadTree` — true for Claude (parentUuid graph), false for Copilot (flat list).
- `supports.editingSessions` — true for Copilot since Oct 2024, false otherwise.
- `supports.toolInvocations` — true for both, format differs.
- `supports.modelMetadata` — true when the storage records which model was used.

UI features key off capabilities, not version numbers. Adding a new VS Code release that introduces a new kind only requires updating that adapter's `parse_response_part` to recognize the new shape — the UI is untouched.

### 4. The warning surface

When `detect()` returns an unknown fingerprint, the app:

1. Renders a non-blocking banner in the affected view: "This session was written by a newer version of Copilot Chat (`fingerprint: a1b2c3`). Showing what we recognized. Some content may be missing."
2. Writes a structured report to `~/.config/<our-tool>/format-reports/<date>-<adapter>-<fingerprint>.json` containing the fingerprint, the file path (not its contents), the first occurrence timestamp, the count of unrecognized record/kind values, and our app version.
3. Offers a one-click "Open report" action that the user can attach to a GitHub issue.
4. **Refuses to perform destructive operations** (delete, cascade-clean) on sessions with unknown fingerprints until the user confirms an extra opt-in. Read-only operations (browse, export) remain available.

The banner copy is intentionally non-alarming. The format changing is the upstream tool's normal lifecycle, not a bug on the user's side.

## Where the seam lives

Per the engineering contract in `SKILL_PROMPT.md`, the abstraction is justified only because there are two concrete callers — Claude and Copilot — on day one. The layout follows the project's import-downhill rule.

```
contracts/
    Conversation, Message, ToolCall, FileEdit     ← normalized domain types
    StorageVersion, Capabilities                  ← version + capability shape
    Provider                                      ← interface; no I/O

adapters/
    claude/
        detect.go      ← fingerprint + version
        parse.go       ← JSONL → contracts/Conversation
        cleanup.go     ← cascade-delete map for ~/.claude
    copilot/
        detect.go
        parse.go       ← replays kind:0/1/2 events
        cleanup.go     ← cascade for workspaceStorage + state.vscdb + CLI
    cursor/            ← stub; ship when we have a second user
    jetbrains/         ← stub; ship when we have a second user

steps/                 ← pure transforms; no I/O
    filter.go          ← hide tool outputs, hide thinking, etc.
    export.go          ← Conversation → markdown
    diff.go            ← file-history diff renderer

composition/           ← the only place that orchestrates I/O
    browse.go
    cleanup.go

entrypoints/
    tui/               ← Bubble Tea / Lip Gloss UI
    web/               ← local HTTP server, htmx/templ frontend
    cli/               ← `--export`, `--clean --dry-run` etc.
```

Imports go downhill: contracts → adapters → steps → composition → entrypoints. No sideways imports between adapters. No I/O outside composition. The TUI and the Web frontend are sibling consumers of the same composition layer.

## What the adapter does **not** do

- It does not migrate the upstream tool's storage. We are a reader and a cleaner, never an editor of someone else's files.
- It does not guess at unknown content. An unknown `kind` is rendered as raw JSON in a fenced block, not paraphrased.
- It does not silently drop unrecognized records. They appear in the conversation model so the user can see them.

## Test strategy

Every adapter ships with a fixture corpus under `tests/fixtures/<adapter>/<version>/`:

- One real-shape file per known version, with secrets scrubbed.
- A "synthetic-future" file in each adapter that introduces a fabricated unknown record type or response kind. The test asserts that parsing succeeds, the unknown is preserved in the model, and the renderer produces the "unknown" block.
- A regression test for every reported breakage: when a user files a format report, the captured fingerprint becomes a new fixture.

The synthetic-future test is the canary. When it fails because we made the parser stricter, we know we broke the contract.

## When to bump our own version

Our app has its own `--version`. We bump it on every release of an adapter that changes the **normalized contract** (the domain types in `contracts/`), not on every release of an upstream tool. Adapter-internal recognition of a new VS Code build is a patch release. Adding a new field to `Conversation` is a minor release. Removing or renaming a field is a major release.
