# Feature ideas

Brainstormed inventory. Not the spec — many of these will get cut. Marked:

- **[CORE]** = part of the v1 minimum
- **[NICE]** = obvious next step
- **[LATER]** = real value, but out of scope for v1
- **[OPEN]** = needs user input before classifying

## Browse

- **[CORE]** Three-pane layout (project list ▸ session list ▸ preview pane). Reference: atuin, fzf, gh dash.
- **[CORE]** Group by project (decoded from the `-Users-djbf-…` folder name) with session count and total size.
- **[CORE]** Each session row shows: started-at relative ("2h ago"), first user prompt (truncated), turn count, size on disk, branch.
- **[CORE]** Preview pane renders the session's conversation as markdown, scrollable, with code blocks syntax-highlighted.
- **[CORE]** Fuzzy filter that searches across all sessions (project name, first prompt, full text). Live preview as you type.
- **[NICE]** Bookmark / star sessions you want to keep forever.
- **[NICE]** Group by week, by git branch, or by project — toggle.
- **[NICE]** Show the conversation as a tree honoring `parentUuid` resumes (instead of flat). Unique differentiator per `02-existing-tools-landscape.md`.
- **[LATER]** Saved views ("show only abandoned", "show last 7 days in WIT-BOT", etc.).

## Filters and clean preview

- **[CORE]** Toggle: hide tool outputs (drop `tool_use` + `tool_result`).
- **[CORE]** Toggle: hide thinking blocks.
- **[CORE]** Toggle: hide meta records (`isMeta: true` — local commands, hooks, slash-command echoes).
- **[NICE]** Toggle: hide sidechain (`isSidechain: true`) sub-agent traffic.
- **[NICE]** Per-tool filter ("hide only Read/Edit, keep Bash").

## Search

- **[CORE]** Fuzzy match on prompt text + first prompt across all sessions.
- **[NICE]** Full-text search across every message in every session, with snippet highlight in the preview.
- **[LATER]** Search inside tool outputs separately.

## Export and share

- **[CORE]** Export current session to markdown (filters applied), copy to clipboard via OSC52 (works over SSH).
- **[CORE]** Export to file, with a sensible filename (`<project>-<date>-<first-prompt-slug>.md`).
- **[NICE]** Export selection (a range of messages, not the whole session).
- **[NICE]** Export multiple sessions to a folder.
- **[LATER]** Upload to gist / pastebin (asks for auth, prompts on first use).

## Clean up

- **[CORE]** **Detect abandoned sessions** (zero real user prompts) and list them with total size. Confirm-then-delete.
- **[CORE]** **Cascade delete**: removing a session also removes `file-history/<sessionId>/`, `tasks/<sessionId>/`, `session-env/<sessionId>`, `sessions/<sessionId>.json`, matching `security_warnings_state_*.json`, and rewrites `history.jsonl` to drop entries with that `sessionId`. Orphan paste-cache cleanup follows.
- **[CORE]** **Orphan scanner**: find `file-history/`, `tasks/`, `session-env/`, `paste-cache/` entries whose owning session no longer exists. Show size, confirm, delete.
- **[NICE]** Trim `history.jsonl` by age or by project.
- **[NICE]** Trim `backups/.claude.json.backup.*` to N most recent.
- **[NICE]** Dry-run mode for every cleanup, always default-on.
- **[NICE]** "Disk usage" page showing the size breakdown from `04-local-data-observations.md` live, with one-key actions per row.

## Safety

- **[CORE]** Trash, not delete: move to `~/.claude/.trash/<timestamp>/` first; "empty trash" is a separate action.
- **[CORE]** Never touch `plugins/`, `skills/`, `ide/`, `settings.json`, `mcp-needs-auth-cache.json`.
- **[NICE]** Undo last cleanup (since the trash exists).

## Other

- **[NICE]** Resume a session: hotkey runs `claude --resume <sessionId>` in a new pane.
- **[NICE]** Open the session's `cwd` in `$EDITOR`.
- **[LATER]** Diff view for `file-history` snapshots — see what Claude changed in a given file over a session.
- **[LATER]** Stats page (turns, tool counts, top tools, time-of-day heatmap). Don't reimplement ccusage's cost analytics; link to it.

## Extensibility (multi-provider)

- **[OPEN]** Do we ship v1 as Claude-only and add Copilot as a follow-up, or design the abstraction from day one?
- The schema gap is real: Claude Code's JSONL is conversational-trees-with-tool-calls; Copilot's storage is VS Code workspaceStorage JSON + SQLite. An adapter interface (`Provider` → list projects, list sessions, read transcript, delete) makes sense, but YAGNI says don't build it until we have the second provider in sight.
- Recommendation to discuss: **build the abstraction shape from day one, implement only Claude.** Adding Copilot becomes a new module rather than a refactor.

## Out of scope

- Editing conversations.
- Replaying / re-running prompts (that's Claude Code's job).
- Analytics dashboards beyond a simple stats page (ccusage's lane).
- Sync / cloud backup.
