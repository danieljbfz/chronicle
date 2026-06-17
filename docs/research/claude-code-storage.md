# Claude Code local storage

This document is the authoritative reference for Claude Code's on-disk layout. Every cleanup heuristic in `adapters/claude/` traces back to a row in here. The findings are a 2026-05-15 snapshot of the official docs, the upstream issue tracker, and a real Claude Code 2.1.x install.

## Contents

- [The top-level layout](#the-top-level-layout)
- [The encoded-cwd encoding](#the-encoded-cwd-encoding)
- [Files in detail](#files-in-detail)
- [Claude's built-in auto-cleaner: `cleanupPeriodDays`](#claudes-built-in-auto-cleaner-cleanupperioddays)
- [Cross-reference cascade-delete map](#cross-reference-cascade-delete-map)
- [Existing third-party cleaners](#existing-third-party-cleaners)
- [Sources](#sources)

## The top-level layout

```
~/.claude/
├── projects/                                ← session content (the gold)
│   └── <encoded-cwd>/
│       ├── <sessionId>.jsonl                ← one session, append-only
│       ├── <sessionId>/                     ← per-session companion dir (lazy)
│       │   ├── subagents/                   ← subagent transcripts
│       │   └── tool-results/                ← spilled large tool outputs
│       └── memory/                          ← per-project auto-memory
│           ├── MEMORY.md                    ← index, loaded at session start
│           └── <topic>.md                   ← loaded on demand
├── file-history/<sessionId>/                ← versioned file backups
├── tasks/<sessionId>/                       ← per-session TaskCreate state
├── session-env/<sessionId>                  ← captured env per session
├── sessions/<pid>.json                      ← live process tracking (NOT per session)
├── plans/<name>.md                          ← saved plans from plan mode
├── shell-snapshots/snapshot-zsh-*.sh        ← captured shell state
├── paste-cache/<contenthash>.txt            ← pasted content cache
├── backups/.claude.json.backup.<ms>         ← rotated config backups
├── history.jsonl                            ← global up-arrow history
├── security_warnings_state_<sessionId>.json ← per-session warning state
├── ide/<pid>.lock                           ← IDE integration locks
├── telemetry/                               ← OpenTelemetry buffer
├── cache/                                   ← runtime caches (changelog, etc.)
├── stats-cache.json                         ← usage/billing accounting
├── mcp-needs-auth-cache.json                ← bundled-MCP auth flags
├── settings.json                            ← user config, NEVER touch
├── .claude.json                             ← in $HOME/, app state, NEVER touch
├── .last-cleanup                            ← Claude's own cleanup marker
├── plugins/                                 ← installed plugins, NEVER touch
├── skills/                                  ← installed skills, NEVER touch
└── downloads/                               ← user-facing downloads, NEVER touch
```

## The encoded-cwd encoding

Per [issue #19972](https://github.com/anthropics/claude-code/issues/19972), the rule is:

```
Replace every character that is not ASCII [A-Za-z0-9-] with -.
```

So `/Users/djbf/Desktop/work/claude-history` becomes `-Users-djbf-Desktop-work-claude-history`. Notes:

- **Dots** become `-`. Path `~/dev/.cache` encodes as `-Users-x-dev--cache` (note the double hyphen).
- **Underscores** become `-`. So `my_app` and `my-app` collide.
- **Unicode** (Chinese, umlauts, emoji) — every byte becomes `-`. Heavily lossy. Cause of the related VS Code extension bug [#35582](https://github.com/anthropics/claude-code/issues/35582).
- **Consecutive non-alphanum runs are NOT collapsed**. `a//b` becomes `a--b`.
- **Encoding is NOT reversible.** A cleaner cannot recover the original cwd from the directory name alone. The canonical path is stored in `~/.claude.json` under the `projects[<cwd>]` key.

Chronicle's `decodeProjectPath` heuristic in `adapters/claude/provider.go` produces a best-effort approximation by replacing `-` with `/`. It does not handle the lossy cases. A future improvement would be to consult `~/.claude.json` for the canonical path.

## Files in detail

### `projects/<encoded-cwd>/<sessionId>.jsonl` — the session

The session content. JSONL with typed records (`user`, `assistant`, `system`, `attachment`, `file-history-snapshot`, `last-prompt`, `permission-mode`, `queue-operation`). Records form a parent-pointer tree via `parentUuid`.

**Cleanup:** Cascade-deleted as the primary item.

### `projects/<encoded-cwd>/<sessionId>/` — per-session companion directory

The session companion directory. Materialized lazily — exists only when the session spawned a subagent or produced large tool output. Two documented subdirectories per the [official `.claude` directory documentation](https://code.claude.com/docs/en/claude-directory):

- `subagents/` — subagent conversation transcripts.
- `tool-results/` — large tool outputs spilled to separate files when they exceed the inline-spill threshold.

**Bug worth knowing:** Issue [#59248](https://github.com/anthropics/claude-code/issues/59248), still open as of v2.1.141, reports that Claude's auto-cleaner deletes the parent `.jsonl` but leaves the `<sessionId>/` companion directory orphaned. On a real install this can accumulate to tens of megabytes per project.

**Cleanup:** Cascade-deleted with the session. `adapters/claude/cleanup.go` adds the companion directory to the delete plan, and `orphans.go` flags it when the `.jsonl` is already gone.

### `projects/<encoded-cwd>/memory/` — per-project auto-memory

Per the [official memory documentation](https://code.claude.com/docs/en/memory), this is "Claude's notes to itself, per project." Written by Claude Code automatically during sessions when the user enables `autoMemoryEnabled` or invokes `/memory`. The contents are plain markdown:

- `MEMORY.md` is the index. The first 200 lines (or 25 KB) are loaded at every session start in this project.
- Topic files like `debugging.md`, `architecture.md` are loaded on demand.
- Filenames are chosen by Claude.

**This is user-facing content the user can edit, but they did not author it.** Treat it as precious-by-default. Deleting it loses Claude's accumulated project knowledge with no undo.

**Stale memory is a real problem.** A user who notices Claude loading outdated information at every session start wants a way to inspect, edit, or selectively prune memory files. `chronicle memory list/show/edit/clean` is that workflow — it manages these files without rummaging the filesystem by hand.

**Cleanup:** Chronicle never auto-deletes memory. `chronicle memory clean` is opt-in and moves files to the recoverable trash rather than removing them.

### `file-history/<sessionId>/` — versioned file backups

Versioned snapshots of files Claude edited during the session. Filenames look like `<hash>@v<n>`.

**Cleanup:** Cascade-deleted with the session. Chronicle includes this.

### `tasks/<sessionId>/` — task state

The state created by `TaskCreate`/`TaskUpdate` calls during a session.

**Cleanup:** Cascade-deleted with the session. Chronicle includes this.

### `session-env/<sessionId>` — captured environment

Captured environment variables for the session.

**Cleanup:** Cascade-deleted with the session. Chronicle includes this.

### `sessions/<pid>.json` — live process tracking (NOT per session)

**Important: this is NOT per-session metadata.** Files here are named after the *process ID* of a live Claude instance. Contents include `pid`, `sessionId`, `cwd`, `startedAt`, `procStart`, `version`, `peerProtocol`, `kind`, `entrypoint`, `status`, `updatedAt`. The `status` field tracks `busy` / `idle`.

**Touching these files while Claude is running could break Claude's own session tracking.** Chronicle never includes this in any cascade or orphan scan.

### `plans/<name>.md` — saved plans from plan mode

Per issues [#57052](https://github.com/anthropics/claude-code/issues/57052), [#54720](https://github.com/anthropics/claude-code/issues/54720), and [#53046](https://github.com/anthropics/claude-code/issues/53046), plan mode persists plans here and resume reads them.

**This is user content.** Chronicle never auto-deletes plans.

### `shell-snapshots/snapshot-zsh-<unix-millis>-<id>.sh` — captured shell state

Captures the user's interactive shell environment at session start (function definitions through `typeset -f`, environment variables, exports). NOT zsh completion state (per issue [#58114](https://github.com/anthropics/claude-code/issues/58114)).

Claude replays the snapshot as a preamble inside the non-interactive `zsh -c` subshells the Bash tool launches. One snapshot per shell init.

**Cleanup:** Safe to remove old ones. Claude regenerates on demand. Chronicle keeps the most recent five.

### `paste-cache/<contenthash>.txt` — pasted content cache

Content-addressed paste payloads referenced by `history.jsonl` records via `pastedContents.contentHash`.

**Cleanup:** Safe to remove orphans (files whose hash is not in `history.jsonl`). The `history.jsonl` reference is the only documented one. Cross-references from session JSONLs in `projects/` were not verified during research, so chronicle's heuristic carries small residual risk.

### `backups/.claude.json.backup.<unix-millis>` — rotated config backups

Timestamped copies of `~/.claude.json` (NOT `~/.claude/.claude.json` — see below). The file contains application state: theme, OAuth session, per-project trust settings, personal MCP servers, UI toggles.

`.claude.json` itself is fragile (non-atomic writes, race corruption documented in issues [#40226](https://github.com/anthropics/claude-code/issues/40226), [#29250](https://github.com/anthropics/claude-code/issues/29250), [#58608](https://github.com/anthropics/claude-code/issues/58608)). Keeping the most recent backups is wise.

**Cleanup:** Chronicle keeps the most recent five.

### `history.jsonl` — global up-arrow history

Global up-arrow prompt history. Append-only JSONL. Records reference paste hashes via `pastedContents.contentHash`.

No evidence of auto-trimming. Grows forever.

**Cleanup:** Chronicle does not touch this today. Trimming requires a partial file rewrite, not a move. A future feature could trim entries by age, by project, or to keep the most recent N.

### `security_warnings_state_<sessionId>.json` — per-session warning state

**Not verified by upstream sources.** Zero hits in upstream issues. The naming pattern strongly implies per-session state for which warning prompts have been shown or dismissed.

**Cleanup:** Chronicle's orphan scan flags files whose session UUID does not match a live session.

### `ide/<pid>.lock` — IDE integration locks

Lock and socket files for the VS Code and JetBrains extensions. Active locks belong to running processes. Stale ones are left over from crashes.

**Cleanup:** Chronicle never touches these today. A future "deep clean" pass could check the PID for liveness before removing.

### `telemetry/` — OpenTelemetry buffer

OpenTelemetry buffer and spool directory (per issues [#56153](https://github.com/anthropics/claude-code/issues/56153), [#46204](https://github.com/anthropics/claude-code/issues/46204), [#32364](https://github.com/anthropics/claude-code/issues/32364)).

**Cleanup:** Safe to delete. Chronicle does not touch this today.

### `cache/` and `stats-cache.json` — runtime caches

`stats-cache.json` is tied to usage and billing accounting (issue [#58786](https://github.com/anthropics/claude-code/issues/58786)). `cache/` contents are runtime caches (the changelog, for example).

**Cleanup:** Both safe to delete. Chronicle does not touch them today.

### `mcp-needs-auth-cache.json` — bundled-MCP auth flags

Per issue [#58607](https://github.com/anthropics/claude-code/issues/58607), this contains only boolean "needs auth" flags for the Anthropic-bundled MCP servers (Gmail, Drive, Calendar). It does NOT store credentials.

**Cleanup:** Safe to delete (forces a re-auth check on next use). Chronicle does not touch this today.

### `.last-cleanup` — Claude's own cleanup marker

A 24-byte file containing an ISO 8601 timestamp. Claude Code reads it at startup to decide whether to run its own cleanup pass.

**Chronicle never touches this.** If we ever maintain our own cleanup marker, we maintain it under our own config directory.

### `settings.json`, `plugins/`, `skills/`, `downloads/`, `~/.claude.json`

User-owned or system-owned content. **Chronicle never touches any of these.**

## Claude's built-in auto-cleaner: `cleanupPeriodDays`

Per the [official settings documentation](https://code.claude.com/docs/en/settings):

- **Default:** 30 days. **Minimum:** 1 day.
- **Setting `0` is rejected** with a validation error in current versions (changed in response to [issue #23710](https://github.com/anthropics/claude-code/issues/23710), where `0` silently disabled all transcript writes). To disable persistence entirely, use the `CLAUDE_CODE_SKIP_PROMPT_HISTORY` env var or `--no-session-persistence` for non-interactive runs.
- **Trigger:** **At startup only.** A `.last-cleanup` marker throttles repeated runs.
- **Age basis:** file modification time (mtime). The cleanup snippet quoted in #23710: `retentionMs = cleanupPeriodDays * 24*60*60*1000`, compared against file timestamps.
- **What it sweeps (officially confirmed):** session `.jsonl` files older than the period, orphaned subagent worktrees older than the period, and leftover `shell-snapshots/` from crashes.
- **What it sweeps (probably, not officially confirmed):** `file-history/`, `plans/`, `debug/`, `paste-cache/`, `image-cache/`, `session-env/`, `tasks/`, `backups/`. The first research pass listed all of these as part of the sweep, but the second pass could not confirm against the current docs. **Treat as unverified.**
- **Manual invocation:** No documented user-facing command.

### Chronicle's relationship to the auto-cleaner

Claude has a built-in cleaner. Chronicle is not the primary janitor. Chronicle complements the auto-cleaner in three scenarios:

1. **The user has set `cleanupPeriodDays` to a large value** and wants to see what would be removed.
2. **The user wants to clean *now*** instead of waiting for the next Claude restart.
3. **The user wants to clean by reference (orphan-status)** instead of by age. A session deleted manually yesterday should have its sibling artifacts removed today, not in 30 days.

These are real use cases, and chronicle's `clean` commands cover them. The cascade and orphan scans follow the sibling references — including the `<sessionId>/` companion directory — and `chronicle memory` inspects and selectively prunes the auto-memory files.

## Cross-reference cascade-delete map

When chronicle deletes a session, the full set of paths to remove is:

1. `projects/<encoded-cwd>/<sessionId>.jsonl` — the session itself
2. `projects/<encoded-cwd>/<sessionId>/` — the companion directory of subagents and tool results
3. `file-history/<sessionId>/` — versioned file backups
4. `tasks/<sessionId>/` — task state
5. `session-env/<sessionId>` — captured environment
6. `security_warnings_state_<sessionId>.json` — warning state (orphan-only, not cascaded)

`history.jsonl` entries with the matching `sessionId` would also need rewriting, but chronicle does not touch `history.jsonl` because partial JSONL rewrites are a different kind of operation than file moves.

## Existing third-party cleaners

| Tool | Approach |
|---|---|
| **claude-code-cleaner** ([garrickz2/claude-code-cleaner](https://github.com/garrickz2/claude-code-cleaner)) | Rust TUI, 5-screen flow. Per-file mtime threshold, default 30 days. Distinguishes "orphan projects" (cwd no longer exists) from "active projects" (only expired files). Touches: `projects/`, `debug/`, `telemetry/`, `shell-snapshots/`, `file-history/`, `paste-cache/`, `transcripts/`, `todos/`, `plans/`, `tasks/`, `usage-data/`. Protects: `settings.json`, `CLAUDE.md`, `skills/`, `commands/`, `agents/`, `ide/`, `credentials.json`. **Does not handle `memory/` or `<sessionId>/{subagents,tool-results}/`.** |
| **CC-Cleaner** ([tk-425/CC-Cleaner](https://github.com/tk-425/CC-Cleaner)) | Web GUI. Cross-references `~/.claude.json` projects list against on-disk dirs to find orphan projects. Moves to system trash. Auto-backs up before destructive ops. |
| **Session Harbor Cleanup** | A skill, not a standalone tool. Archives "low-signal" sessions by age plus user-message count. |

**Lessons we're already applying:** trash before unlink, explicit allowlist of protected paths, split the listing logic between "live" and "orphan."

**Lesson we should adopt:** auto-back up `~/.claude.json` before any destructive operation that touches related state. Chronicle does not modify `~/.claude.json`, so this currently does not apply, but if a future feature ever did, the back-up-first pattern is the right one.

## Sources

- [Explore the .claude directory (official docs)](https://code.claude.com/docs/en/claude-directory)
- [Claude Code settings reference](https://code.claude.com/docs/en/settings)
- [How Claude remembers your project (memory docs)](https://code.claude.com/docs/en/memory)
- [Work with sessions — agent SDK docs](https://code.claude.com/docs/en/agent-sdk/sessions)
- [Issue #59248 — orphan subagent dirs not cleaned (open as of v2.1.141)](https://github.com/anthropics/claude-code/issues/59248)
- [Issue #51779 — cleanupPeriodDays coverage docs](https://github.com/anthropics/claude-code/issues/51779)
- [Issue #23710 — `cleanupPeriodDays: 0` bug](https://github.com/anthropics/claude-code/issues/23710)
- [Issue #19972 — path encoding readability/compat](https://github.com/anthropics/claude-code/issues/19972)
- [Issue #35582 — VS Code extension breaks on Unicode paths](https://github.com/anthropics/claude-code/issues/35582)
- [Issue #58114 — shell snapshot contents and replay](https://github.com/anthropics/claude-code/issues/58114)
- [Issue #58607 — `mcp-needs-auth-cache.json` contents](https://github.com/anthropics/claude-code/issues/58607)
- [Issues #40226, #29250, #58608 — `.claude.json` fragility](https://github.com/anthropics/claude-code/issues/40226)
- [Issues #57052, #54720, #53046 — `plans/` persistence](https://github.com/anthropics/claude-code/issues/57052)
- [Issues #56153, #46204, #32364 — `telemetry/` directory](https://github.com/anthropics/claude-code/issues/56153)
- [Issue #58786 — `stats-cache.json` and metering](https://github.com/anthropics/claude-code/issues/58786)
- [Inside Claude Code: The Session File Format](https://databunny.medium.com/inside-claude-code-the-session-file-format-and-how-to-inspect-it-b9998e66d56b)
