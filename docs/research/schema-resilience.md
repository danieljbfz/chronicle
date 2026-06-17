# Schema resilience: surviving format churn

Claude Code and VS Code Copilot both ship weekly and both have changed their on-disk formats multiple times in the past two years. A tool that reads their data must assume the format will change underneath it. This document is the contract for how chronicle handles that.

## Contents

- [The problem](#the-problem)
- [The contract](#the-contract)
- [Where the seam lives](#where-the-seam-lives)
- [What the adapter does not do](#what-the-adapter-does-not-do)
- [Test strategy](#test-strategy)
- [When to bump chronicle's own version](#when-to-bump-chronicles-own-version)

## The problem

Concrete examples from the last two years.

| Date | Tool | Change | Impact on a naive reader |
|---|---|---|---|
| Dec 2023 | VS Code | Renamed "interactive sessions" → "chat" in storage keys. | VS Code kept the old keys as aliases, so a reader that knew only them quietly missed the new chat keys. |
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

The resilience contract lives at the boundary between the adapters and the rest of the system. Detection and tolerant parsing are an adapter's job: `detect.go` computes the fingerprint and maps it to a version, and `parse.go` folds the storage into the normalized `contracts.Conversation`, preserving anything it does not recognize as a `contracts.UnknownBlock`. The capability flags travel up to the UI on the `StorageVersion` value, so a layer above the adapter decides what to render without knowing the storage format.

This works because imports flow strictly downhill — contracts is a leaf, adapters and steps depend on it, and composition depends on them. No adapter imports another, and no I/O happens outside composition, so a format change touches exactly one adapter and nothing above it. The full package layout is in [`../codebase-tour.md`](../codebase-tour.md).

## What the adapter does **not** do

- It does not migrate the upstream tool's storage. We are a reader and a cleaner, never an editor of someone else's files.
- It does not guess at unknown content. An unknown `kind` is rendered as raw JSON in a fenced block, not paraphrased.
- It does not silently drop unrecognized records. They appear in the conversation model so the user can see them.

## Test strategy

Every adapter ships with a fixture corpus under its own `testdata/` directory:

- One real-shape file per known version, with secrets scrubbed.
- A "synthetic-future" file in each adapter that introduces a fabricated unknown record type or response kind. The test asserts that parsing succeeds, the unknown is preserved in the model, and the renderer produces the "unknown" block.
- A regression test for every reported breakage: when a user files a format report, the captured fingerprint becomes a new fixture.

The synthetic-future test is the canary. When it fails because we made the parser stricter, we know we broke the contract.

## When to bump chronicle's own version

Chronicle's version moves on every change to the **normalized contract** (the domain types in `contracts/`), not on every release of an upstream tool. Adapter-internal recognition of a new VS Code build is a patch release. Adding a new field to `Conversation` is a minor release. Removing or renaming a field is a major release.
