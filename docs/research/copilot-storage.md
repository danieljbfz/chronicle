# GitHub Copilot local storage

Where VS Code Copilot Chat, Copilot CLI, Cursor, and JetBrains Copilot persist their conversations on disk, the formats they use, and what we can do with them.

Sources: direct inspection of a running install on macOS 2026, plus VS Code release notes through v1.100 and the open-source `microsoft/vscode` chat module. JetBrains specifics are noted as unverified because the user does not have JetBrains installed and the docs were not reachable from this session.

## Contents

- [Top-level paths](#top-level-paths-macos-current-vs-code)
- [Linking a workspace hash to a folder](#linking-a-workspace-hash-to-a-folder)
- [Chat session JSONL format](#chat-session-jsonl-format-current-schema-version-3)
- [`chatEditingSessions/<sessionId>/`](#chateditingsessionssessionid)
- [`state.vscdb`](#statevscdb-sqlite-wal-mode)
- [`globalStorage/github.copilot-chat/`](#globalstoragegithubcopilot-chat)
- [Cross-references (cascade-delete map)](#cross-references-cascade-delete-map)
- [Cursor and VS Code Insiders](#cursor-and-vs-code-insiders)
- [JetBrains Copilot (unverified)](#jetbrains-copilot-unverified)
- [Local-data observations](#local-data-observations-this-machine-2026-05-15)
- [Why the format model differs from Claude's](#why-the-format-model-differs-from-claudes)

## Top-level paths (macOS, current VS Code)

| Path | What it is |
|---|---|
| `~/Library/Application Support/Code/User/workspaceStorage/<hash>/chatSessions/<sessionId>.jsonl` | **The conversations.** Per-workspace, one file per session, JSONL event log. |
| `~/Library/Application Support/Code/User/workspaceStorage/<hash>/chatEditingSessions/<sessionId>/` | Per-session file-edit snapshots (analogue of Claude's `file-history/`). |
| `~/Library/Application Support/Code/User/workspaceStorage/<hash>/state.vscdb` | SQLite, WAL mode. Holds Copilot view state and the session index. |
| `~/Library/Application Support/Code/User/workspaceStorage/<hash>/workspace.json` | Maps the workspace `<hash>` to its folder URI. Critical for the `hash → /Users/...` decoding. |
| `~/Library/Application Support/Code/User/globalStorage/github.copilot-chat/` | Global Copilot extension data — embeddings, agent definitions, CLI shim. |
| `~/Library/Application Support/Code/User/globalStorage/github.copilot-chat/copilotCli/` | Copilot CLI session metadata. |
| `~/Library/Application Support/Code/User/globalStorage/github.copilot-chat/copilot-cli-images/` | Image attachments from the Copilot CLI. |
| `~/Library/Application Support/Code/User/globalStorage/emptyWindowChatSessions/` | Chats from VS Code windows that were opened without a workspace folder. Same schema as `chatSessions/`. |

> Linux uses `~/.config/Code/User/...` and Windows uses `%APPDATA%\Code\User\...`. Same internal structure.

## Linking a workspace hash to a folder

The hash is opaque on the filesystem. The mapping lives in `workspace.json` inside each workspace-storage directory.

```json
{ "folder": "file:///Users/djbf/Desktop/work/claude-history" }
```

Parse that URI and we recover the human-readable project name. Without this step, the user sees `0769784b324abbbf18e1b1cea35bb367` instead of `claude-history`.

## Chat session JSONL format (current schema, `version: 3`)

Each `chatSessions/<sessionId>.jsonl` is an **event log**. Every line is a JSON object with a `kind` field:

| `kind` | Shape | Meaning |
|---|---|---|
| `0` | `{ "kind": 0, "v": { ...snapshot... } }` | Full session snapshot. Always the first line. Rewritten when the file is compacted. |
| `1` | `{ "kind": 1, "k": ["path", "to", "field"], "v": <new-value> }` | Mutation to a field at the given JSON path. |
| `2` | `{ "kind": 2, "k": ["path"], "v": <appended-value> }` | Append-to-array at the given path. |

To read the current state, replay all lines in order. To stream changes, watch for new lines and apply them.

The `v` of the initial snapshot has:

- `version` — the schema version, `3` in the builds observed here. **This is our format-stability anchor.**
- `sessionId` — UUID, matches the filename.
- `creationDate`, `lastMessageDate` — epoch ms.
- `responderUsername`, `responderId` — "GitHub Copilot" / Copilot extension id.
- `initialLocation` — `panel`, `editor`, `terminal`, `notebook`.
- `customTitle` — user-renamed title, when present.
- `inputState` — current input box state, including:
  - `mode` — `{ id: "agent" | "ask" | "edit", kind: ... }`. The user can switch between modes.
  - `selectedModel` — full model descriptor with `family`, `pricing`, `maxInputTokens`, etc. Currently observed values include `claude-sonnet-4.6` and `gpt-5`.
- `requests[]` — the conversation turns.

Each `requests[i]` has:

- `requestId`, `timestamp`.
- `message` — `{ parts: [...] }` with `parts[]` of `kind`: `text`, `agent` (`@workspace`, `@terminal`), `slash` (`/explain`, `/fix`), `var` (`#file`, `#selection`), `dynamicVariable`.
- `agent`, `slashCommand`, `variableData` — denormalized copies for replay.
- `response[]` — assistant response parts. Each part has a `kind`: `markdown`, `inlineReference`, `codeblockUri`, `commandButton`, `progressMessage`, `confirmation`, `toolInvocation`, `textEdit`, `notebookEdit`, `undoStop`. **New `kind` values appear roughly every few VS Code releases.** Parsers must pass unknown kinds through.
- `result` — `{ errorDetails?, metadata?, nextQuestion? }`.
- `usedContext`, `contentReferences` — files the model actually consumed. Powers the "References" panel.
- `codeCitations` — public-code matches with license info.
- `followups`, `voteDirection`, `editedFileEvents` — feedback and edit tracking.

### Filtering for a clean transcript

To produce a human-readable export (the analogue of our Claude "hide tool outputs" filter), keep:

- `message` `parts[]` where `kind in {text, agent, slash, var}` — render naturally.
- `response[]` where `kind == "markdown"` — the actual answer text.

Drop:

- `response[]` where `kind in {toolInvocation, progressMessage, confirmation, commandButton, textEdit, notebookEdit, undoStop}` — tool plumbing.
- `usedContext`, `contentReferences` — informational, noisy.
- All `kind: 1` / `kind: 2` mutation events that are pure UI state (cursor position, scroll, attachments preview).

## `chatEditingSessions/<sessionId>/`

Mirrors Claude's `file-history/`. Each session directory contains:

- `state.json` — `{ entries: [{ resource, state, telemetryInfo }, ...], ... }` describing the working set.
- `contents/` — per-stop snapshot blobs of the files Copilot edited.

This is the orphan candidate when a session is deleted but the working-set survives. We must cascade-delete it together with the JSONL.

## `state.vscdb` (SQLite, WAL mode)

Schema: `ItemTable(key TEXT PRIMARY KEY, value BLOB)` where `value` is a UTF-8 JSON string. Each workspace gets its own DB.

Copilot-related keys observed locally (workspace DB):

- `chat.ChatSessionStore.index` — the session index that powers the chat history list.
- `chat.customModes.local` — user-defined chat modes.
- `chat.untitledInputState` — pending input for windows without an active session.
- `GitHub.copilot-chat` — extension state blob.
- `memento/interactive-session-view-copilot` — view UI memento.
- `workbench.panel.chat`, `workbench.panel.chat.numberOfVisibleViews` — panel state.

**Reading safety while VS Code is running:** open with `file:state.vscdb?mode=ro` (URI form). Do **not** set `immutable=1` on a live database — SQLite docs are explicit that this corrupts results on changing files. If we need stable snapshots, use `sqlite3 source ".backup target"` to copy together with the `-wal` and `-shm` sidecars.

**Never write to a live VS Code DB.** All our delete operations either touch JSON/JSONL files (atomic with rename) or refuse to run while VS Code is open.

## `globalStorage/github.copilot-chat/`

Beyond the CLI subdirectories, this folder holds:

| Path | Purpose | Touchable? |
|---|---|---|
| `commandEmbeddings.json` (~13 MB) | Cached command embeddings. | Regenerable but slow to rebuild. Leave alone by default. |
| `settingEmbeddings.json` (~12 MB) | Cached settings embeddings. | Same. |
| `toolEmbeddingsCache.bin` | Tool embedding cache. | Same. |
| `ask-agent/`, `explore-agent/`, `plan-agent/` | Built-in agent definitions (`*.agent.md`). | Read-only definitions, leave alone. |
| `memory-tool/memories/` | Persistent memory the user has saved. | **User content — never auto-delete.** Expose to the browser, allow export and manual delete. |
| `debugCommand/` | The `copilot-debug` helper script. | Don't touch. |
| `copilotCli/copilotcli.session.metadata.json` | Maps Copilot CLI sessionId → workspace folder + custom title. | Read for indexing. Cascade-delete entries we delete. |
| `copilotCli/copilotCLIShim.js`, `copilotCLIShim.ps1`, `copilot` | CLI shim scripts. | Don't touch. |
| `copilot-cli-images/` | Image attachments from CLI sessions. | Cascade-delete with the owning CLI session. |

## Cross-references (cascade-delete map)

When we delete a Copilot Chat session, the full cleanup is:

1. `workspaceStorage/<hash>/chatSessions/<sessionId>.jsonl`
2. `workspaceStorage/<hash>/chatEditingSessions/<sessionId>/` (directory tree)
3. Index entry in `workspaceStorage/<hash>/state.vscdb` row `chat.ChatSessionStore.index` (only when VS Code is **closed**, otherwise refuse)
4. For Copilot CLI sessions: matching key in `globalStorage/.../copilotCli/copilotcli.session.metadata.json`
5. For Copilot CLI sessions: images under `globalStorage/.../copilot-cli-images/` keyed by that session

Today, removing the JSONL by hand leaves the editing-session directory, the state.vscdb index entry, and the CLI metadata orphaned.

## Cursor and VS Code Insiders

- **VS Code Insiders** — identical layout under `~/Library/Application Support/Code - Insiders/User/...`. The schema can be one to four weeks ahead of stable VS Code.
- **Cursor** — a fork that uses the same skeleton (`workspaceStorage/<hash>/state.vscdb`, `globalStorage/`) but **stores chat differently**: chat data lives primarily in `globalStorage/state.vscdb` under keys like `workbench.panel.aichat.view.aichat.chatdata`, `composer.composerData`, and `aiService.prompts`. Cursor introduces a `cursorDiskKV` table for larger payloads. **Cursor does not use `chatSessions/<id>.jsonl`.** Treat Cursor as a separate adapter.

## JetBrains Copilot (unverified)

Plugin storage is reported to live under `~/Library/Application Support/JetBrains/<Product><version>/options/github-copilot.xml` for settings and `~/Library/Caches/JetBrains/<Product><version>/copilot/` for chat caches. Format is JSON, not the same shape as VS Code's. **Treat this section as unverified.** Confirm against the `github/copilot-intellij` repo before shipping a JetBrains adapter.

## Local-data observations (this machine, 2026-05-15)

- **109 chat sessions across 12 workspaces.** `workspaceStorage/` totals **2.0 GB** — bigger than `~/.claude/` (376 MB).
- `globalStorage/github.copilot-chat/` totals 24 MB, dominated by the embeddings caches (25 MB combined).
- One abandoned `emptyWindowChatSessions/` session sits at 1.4 KB.
- Sessions are individually tiny (1.7–1.8 KB for empty ones), but `workspaceStorage/` accumulates other extensions' data and that is most of the 2 GB.
- Cleanup target for our tool: the empty/abandoned chat sessions, orphan `chatEditingSessions/` directories, and `emptyWindowChatSessions/` entries. We intentionally do not delete the embeddings caches — too costly to rebuild.

## Why the format model differs from Claude's

Claude Code writes **append-only typed records** with parent/child UUIDs forming a tree. VS Code Copilot writes a **snapshot + mutations event log**. Both end up as `.jsonl`, but the right reader for each is different:

- For Claude, fold records into a parent-pointer tree and render.
- For Copilot, replay `kind: 0` then apply `kind: 1` and `kind: 2` patches to get the current state, then render `requests[]`.

The `Provider` interface hides this difference behind a `ReadSession` method that returns a normalized `Conversation`, so every layer above the adapter is blind to the storage shape. The package layout is in [`../codebase-tour.md`](../codebase-tour.md).
