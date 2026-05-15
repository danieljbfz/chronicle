# Feature roadmap

This document is the principal-engineer's view of what chronicle could do, organized by user value and implementation cost. The list is opinionated. Items are tagged:

- **Ship now** — high value, low risk, fits the existing architecture cleanly.
- **Ship soon** — high value, real complexity, worth doing but worth thinking through first.
- **Maybe** — useful in some scenarios but not obviously worth the cost.
- **Skip** — looks tempting but the cost-benefit does not work out.

The mental frame is: **chronicle is a multi-provider history manager**. The user's history is sessions, but it is also memory, plans, settings, plugins, and the artifacts every AI coding assistant writes alongside its conversations. We can be useful in any of those.

## Already shipped

- Multi-provider session listing (`chronicle list`).
- Session export (`chronicle export`) and clipboard copy (`chronicle copy`).
- Doctor view (`chronicle doctor`).
- Cascade-aware delete with trash (`chronicle clean abandoned`).
- Orphan scan with cascade plus floating-junk heuristics (`chronicle clean orphans`).
- Trash management (`chronicle trash list/restore/empty`).
- Two providers (Claude Code, GitHub Copilot Chat).

## Ship now

These are immediately addressable, fit the existing architecture without redesign, and unlock real user value.

### `chronicle memory` — manage per-project auto-memory

The user explicitly raised this. Claude's auto-memory directory at `~/.claude/projects/<encoded-cwd>/memory/` holds markdown files Claude loads at every session start in that project. Stale memory becomes wrong information that pollutes every new conversation. The user has no easy way today to inspect, edit, or selectively prune these files without rummaging the filesystem.

Subcommands:

- `chronicle memory list` — list every per-project memory file across all projects, with size and last-modified date.
- `chronicle memory show <project> [--file MEMORY.md]` — dump the contents to stdout, paged.
- `chronicle memory edit <project> [--file MEMORY.md]` — open in `$EDITOR`.
- `chronicle memory clean <project>` — delete every memory file in the project (with confirmation, into trash).

**Cost: small.** New `composition/memory.go`, new CLI subcommand. The existing trash subsystem handles the destructive side. The `<project>` argument is the same `ProjectID` we already use elsewhere.

**Risk: low.** Memory files are markdown the user can already edit through `/memory` in Claude itself. We just give them a faster surface.

### `chronicle clean` for the per-machine `.claude.json`

The `~/.claude.json` config file at the user's home directory holds OAuth tokens, per-project trust settings, and personal MCP server configs. It is fragile (issues #40226, #29250, #58608 document race-condition corruption). chronicle could:

- Verify the file parses as valid JSON.
- List the projects it knows about.
- Find projects in `.claude.json` whose on-disk directory under `~/.claude/projects/<encoded-cwd>/` has been deleted, and offer to remove the stale `.claude.json` entry.

**Cost: small to medium.** Editing `.claude.json` is a partial-rewrite operation, not a file move, so we cannot route it through the existing trash. We need an atomic-write helper (write to temp, rename) and a backup-first pattern (write `~/.claude.json` itself to `~/.claude/backups/` before mutating).

**Risk: medium.** A bad write to `.claude.json` could log the user out or wipe per-project trust. The atomic-write + backup-first pattern keeps the blast radius small.

### `chronicle stats` — disk usage breakdown

The user already has size data per provider in the `doctor` view. A `stats` view would extend this:

- Total disk usage per provider.
- Breakdown by category (sessions, file-history, paste-cache, memory, etc.).
- Top N largest sessions across providers.
- Distribution of session sizes (how many tiny sessions vs huge ones?).
- Age distribution (how many sessions older than 30 days?).

**Cost: small.** Pure computation over what `ListSessions` already returns. New `composition/stats.go` plus a CLI subcommand.

**Risk: none.** Read-only.

## Ship soon

Real value, real cost, worth the careful thought.

### `chronicle search` — full-text search across all sessions

Today the user can `chronicle list` and grep titles, but the title is just the first user prompt. Searching the body of every session is what they actually want when they ask "where did I discuss X?".

Subcommands:

- `chronicle search <query>` — full-text search, ranked results, snippet preview.
- `chronicle search <query> --provider claude` — limit to one provider.
- `chronicle search <query> --since 7d --project myproject` — filter by age and project.

**Cost: medium.** A naive grep across every session file works for hundreds of sessions but slows down at thousands. The right design is a small index file (BadgerDB or SQLite) maintained incrementally. The index needs to invalidate on file mtime, which is one stat call per session per query.

**Risk: low.** Read-only. The only concern is the index getting out of sync; we can solve that with a `chronicle search --rebuild` flag.

### `chronicle resume <session-id>`

The user finds an interesting old session through `chronicle list` or `chronicle search` and wants to pick up where they left off. Today they have to figure out the session ID from chronicle's output and pass it to Claude themselves. We can shortcut that.

```
chronicle resume <session-id>
```

This invokes `claude --resume <session-id>` for Claude sessions, with the right working directory set automatically. For Copilot, the equivalent might involve opening VS Code at the workspace and selecting the chat from the panel — harder, possibly worth shipping for Claude only at first.

**Cost: small for Claude.** Just an `os/exec.Command` invocation. The session is already in our index because `ListSessions` knows about it.

**Risk: none.** We are just running another tool the user could have run manually.

### `chronicle export --bulk` — export many sessions at once

Today `chronicle export` handles one session at a time. Power users want to export every session in a project, or every session matching a search, into a directory of markdown files.

```
chronicle export --bulk --project myproject -o ./exported/
chronicle export --bulk --since 30d -o ./recent-sessions/
```

**Cost: small.** Loop around the existing single-session export.

**Risk: none.** Read-only.

### `chronicle config edit/show/get/set`

Today the user has to know that the config lives at `~/.config/chronicle/config.toml` and edit it manually. A small wrapper would be nicer.

```
chronicle config show          # print current effective config
chronicle config get trash.retention_days
chronicle config set trash.retention_days 60
chronicle config edit          # open in $EDITOR
```

**Cost: small.** Wraps the existing config loader plus a tiny TOML write helper.

**Risk: low.** Atomic write + the config loader already handles missing/malformed files gracefully.

### `chronicle clean stale [--older-than 30d]`

A by-age cleanup category, parallel to `clean abandoned` and `clean orphans`. For users who want chronicle to do what Claude's auto-cleaner does, but on demand and across all providers. Would be especially useful for Copilot, which has no equivalent auto-cleaner.

**Cost: small.** New `CleanCategory` and a per-category builder. The existing trash subsystem handles the rest.

**Risk: low** if the default `--older-than` value is conservative (90 days?).

## Maybe

These could be useful but the cost-benefit is unclear. Worth discussing before building.

### MCP server management

The user asked specifically about this. MCP (Model Context Protocol) servers are configured in `~/.claude.json` at the project level and globally. Today the user adds them with `claude mcp add` and removes them with `claude mcp remove`. They also live in Copilot's settings.

What chronicle could add:

- `chronicle mcp list` — list every configured MCP server across providers, marking which are global and which are per-project.
- `chronicle mcp doctor` — check whether each one is reachable or broken.
- `chronicle mcp clean` — find MCP server entries in `~/.claude.json` whose project no longer exists and offer to remove.

**Why this is a "Maybe":** MCP server configuration is editing a JSON file Claude already manages through its own `claude mcp` commands. We would be a thin wrapper. The orphan-scan idea (entries pointing at gone projects) is the most useful piece, but it is also the smallest, so the value-to-effort ratio of the whole feature is questionable. Probably ship `chronicle mcp list` and `chronicle mcp doctor` as read-only views, skip the rest.

**Cost: medium.** Parsing `~/.claude.json` carefully, the same atomic-write concerns as `.claude.json` cleanup.

### Plugin management

Claude Code has a plugin system (`~/.claude/plugins/`). Plugins are extensions that add commands, skills, or other behaviour. Chronicle could:

- `chronicle plugin list` — list installed plugins.
- `chronicle plugin doctor` — check each one for breakage.
- `chronicle plugin remove <name>` — uninstall.

**Why this is a "Maybe":** Claude Code has its own plugin commands and the marketplace tooling around them. We would duplicate. Real value would be cross-provider plugin awareness, but Copilot does not have an equivalent plugin system, so the cross-provider angle is hollow. Probably skip unless a specific user need shows up.

**Cost: medium.** Reading the plugin manifest, understanding the install/uninstall lifecycle.

### Web frontend (Plan E in the original spec)

The original feature ideas included a local web frontend at `chronicle serve`. Read-only viewer for browsing sessions in a browser instead of the terminal. Useful for sharing rendered transcripts with teammates without needing an export step.

**Why this is a "Maybe":** Real value but real cost. We would be building a small web app: htmx + templ for server-rendered HTML, a session-detail page, a search page. The TUI would address the same UX concerns with less code.

**Cost: medium-large.** New `composition/serve.go`, the templ layer, basic auth (or bind to localhost only).

### TUI (Plan D in the original spec)

A Bubble Tea + Lip Gloss interactive shell for browsing, searching, and cleaning. Three pages (Browse, Cleanup, Doctor). Glamour for syntax-highlighted markdown rendering.

**Why this is a "Maybe":** This is what the user originally asked for in the spec. The CLI we have today actually covers most of the same ground. The TUI's value is more about *presentation* than *capability*. If the user wants a beautiful interactive experience, this is the highest-impact item. If the user is fine with the CLI, it is the largest cost.

**Cost: large.** Several days of work, a new entire layer (`internal/ui/tui/`), keymaps, viewport scrolling, async rendering.

## Skip

These have come up but I would not build them.

### A daemon that watches for new sessions

Could trigger cleanup automatically. Adds complexity (a background process, a launchd or systemd unit, error-handling for misbehavior). Claude Code already does its own cleanup at startup. Not worth the operational cost.

### A "share to gist" command

Interesting but pulls chronicle into the auth-token-management business. Better solved by `chronicle export -o foo.md && gh gist create foo.md` once. The composition is worth more than a baked-in command.

### Plugin / MCP installation

Adding plugins or MCP servers is what the upstream tools' own commands are for. Chronicle being a multi-provider tool means we would need to translate concepts across providers, which is a coordination headache for a feature the upstream tools already do well.

### Cloud sync / backup

Out of scope. Would require server-side state, accounts, the whole shebang. The user can already `tar` their chronicle config and sync that themselves.

## My recommended order

If I were running this project as a tech lead, the next four items in this order would be:

1. **`chronicle memory` workflow.** Direct response to the user's stated pain about stale memory. Small cost, high value.
2. **`chronicle stats`.** Pure read-only addition. Reuses what we have. Lets the user see at a glance how big each piece of their AI-history footprint is.
3. **`chronicle search`.** Real value gap today. The main reason to keep history at all is to find things later, and finding things means search.
4. **`chronicle clean stale`.** Rounds out the cleanup story with a third category alongside abandoned and orphans.

After those four, the next-bigger choice is **TUI vs Web vs more providers**, which is more about strategic direction than tactical work.
