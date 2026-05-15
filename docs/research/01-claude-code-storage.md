# Claude Code local storage

Everything Claude Code persists on disk, what it means, and what we can do with it.

Investigated on macOS, Claude Code version `2.1.133`. No official schema exists; this is reverse-engineered from a real `~/.claude/` directory.

## Top-level layout of `~/.claude/`

| Path | What it is | Touchable? |
|---|---|---|
| `projects/<encoded-cwd>/<sessionId>.jsonl` | **The conversations.** One JSONL per session, grouped by the encoded working directory. | Read, filter, export, delete. The primary data source. |
| `file-history/<sessionId>/<hash>@v<n>` | Versioned snapshots of files Claude edited. Referenced from the JSONL stream. | Safe to delete with the session. Reclaim disk. |
| `tasks/<sessionId>/` | Per-session task list data (the TaskCreate/TaskUpdate state). | Safe to delete with the session. |
| `shell-snapshots/snapshot-zsh-*.sh` | Captured shell state for resumed sessions. | Stale ones can be GC'd. |
| `paste-cache/<hash>.txt` | Pasted text/images, content-addressed by hash. | Orphans (not referenced by any live `history.jsonl` entry) can be cleaned. |
| `session-env/<sessionId>` | Captured environment variables per session. | Orphans can be cleaned. |
| `sessions/<sessionId>.json` | Small session metadata. | — |
| `plans/<name>.md` | Saved plans from plan mode. | User content — never auto-delete. |
| `backups/.claude.json.backup.<ts>` | Backups of the top-level `.claude.json` config. | Trim by age. |
| `history.jsonl` | Global prompt history (the up-arrow history), with `project`, `sessionId`, `timestamp`, and `pastedContents` refs. | Can be trimmed by age or project. |
| `settings.json` | User settings. | Don't touch. |
| `stats-cache.json` | Stats cache. | Disposable. |
| `cache/`, `downloads/` | Misc cache. | Disposable. |
| `mcp-needs-auth-cache.json` | MCP auth state. | Don't touch. |
| `security_warnings_state_<uuid>.json` | Per-session security-prompt state. | Orphans can be cleaned. |
| `plugins/` | Installed plugins. | Don't touch. |
| `skills/` | Skill files. | Don't touch. |
| `telemetry/` | (empty here) | — |
| `ide/` | IDE integration state. | Don't touch. |
| `.last-cleanup` | Marker for last cleanup run. | Update on our own cleanups. |

## `projects/` directory naming

Each subfolder is the absolute working directory with `/` replaced by `-`, e.g.
`/Users/djbf/Desktop/work/claude-history` → `-Users-djbf-Desktop-work-claude-history`.

That gives us a project ↔ folder mapping for free. The session JSONL itself carries `cwd` on every record, so we don't have to depend on the folder name.

## JSONL record types

A session file is a stream of newline-delimited JSON records. Each record has a `type`.

| `type` | Meaning |
|---|---|
| `user` | A user message. `message.role = "user"`, `message.content` is either a string or an array of typed parts (`text`, `tool_result`, `image`). `isMeta: true` for synthetic messages (e.g. local-command captures, slash-command echoes). |
| `assistant` | An assistant message. `message.content` is an array of parts: `text`, `thinking`, `tool_use`. |
| `system` | System notes (e.g. `subtype: "local_command"` with `<local-command-stdout>` content). |
| `attachment` | Files, hook outputs, screenshots, paste references attached to a turn. |
| `file-history-snapshot` | Inline reference to a versioned file backup at a given point in the conversation. |
| `last-prompt` | Bookmark recording the latest prompt and its `leafUuid` (head of the conversation tree). |
| `queue-operation` | Operations on the prompt queue (e.g. enqueued follow-up). |
| `permission-mode` | Records the permission mode at a given moment. |

## Conversation graph

Records carry both `uuid` and `parentUuid`. Resumed sessions branch off an earlier message — the structure is a **tree**, not a line. The `last-prompt` records flag the current leaf.

Most existing tools render the tree linearly and lose branches (see `02-existing-tools-landscape.md`).

## Useful per-record fields

- `sessionId`, `cwd`, `version`, `gitBranch` — same on every record in a session.
- `timestamp` — ISO 8601 string, monotonic enough to sort by.
- `userType: "external"` / `entrypoint: "cli"` — identifies real user-driven turns.
- `isMeta: true` — synthetic, should be hidden by default in any "clean transcript" view.
- `isSidechain: true` — sub-agent / sidechain conversation; useful to filter.
- `attachment.type` — `hook_success`, `image_paste`, `file_attached`, etc.

## What "tool output" looks like

A typical assistant turn that runs a tool produces two records:

1. `assistant` record with a `tool_use` content block (`name`, `input`, `id`).
2. `user` record with `role: "user"` and a `tool_result` content block (`tool_use_id`, `content`, `is_error`).

For the "export without tool outputs" filter, we drop:
- assistant `tool_use` blocks
- user `tool_result` blocks
- typically also assistant `thinking` blocks (configurable)

That leaves clean alternating `text`-only messages — the conversation as a human reads it.

## Cross-references between folders

When we delete a session, the **full** cleanup is:

1. `projects/<cwd>/<sessionId>.jsonl`
2. `file-history/<sessionId>/`
3. `tasks/<sessionId>/`
4. `session-env/<sessionId>`
5. `sessions/<sessionId>.json` (if present)
6. `security_warnings_state_<sessionId>.json` (if it matches)
7. Any `history.jsonl` entries with that `sessionId` (rewrite the file)
8. `paste-cache/<hash>.txt` referenced only by removed `history.jsonl` entries

Today, deleting a JSONL by hand leaves all of (2)–(8) as orphans. That's why `~/.claude` grows.
