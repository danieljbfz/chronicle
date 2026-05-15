# Feature roadmap

This document is the principal-engineer's view of what chronicle could do, organized by user value and implementation cost. The list is opinionated. Items are tagged:

- **Ship now** — high value, low risk, fits the existing architecture cleanly.
- **Ship soon** — high value, real complexity, worth doing but worth thinking through first.
- **Maybe** — useful in some scenarios but not obviously worth the cost.
- **Skip** — looks tempting but the cost-benefit does not work out.

The mental frame is: **chronicle is a multi-provider history manager**. The user's history is sessions, but it is also memory, plans, settings, plugins, and the artifacts every AI coding assistant writes alongside its conversations. We can be useful in any of those.

## Already shipped

The CLI surface is feature-complete. Every command runs read-only by
default, every destructive command defaults to dry-run, and every
deletion goes through the recoverable trash (or, for `clean dangling`,
through a backup-first surgical edit).

### Browse and inspect

- Multi-provider session listing (`chronicle list`).
- Doctor view (`chronicle doctor`).
- Stats summary across providers, projects, time, and disk usage (`chronicle stats`).
- Substring search across every session of every provider (`chronicle search`).

### Export and copy

- Single-session export (`chronicle export`).
- Bulk export of every session in one project (`chronicle export --bulk`).
- Clipboard copy via OSC 52 (`chronicle copy`).

### Resume

- Re-open a session in its original tool, in the original working directory (`chronicle resume`). Provider-aware: Claude only, with a clear "this provider does not support resume" message for Copilot.

### Memory

- List, show, edit, and clean per-project memory files (`chronicle memory list/show/edit/clean`).
- Same four operations against the user-global memory file (`~/.claude/CLAUDE.md`) via `--global`.

### Cleanup

Four categories, each defaulting to dry-run, each routing through the recoverable trash (or, for dangling, through a backup-first edit):

- `chronicle clean abandoned` — sessions with zero real user prompts.
- `chronicle clean stale --older-than 30d` — sessions older than the threshold (default matches Claude's `cleanupPeriodDays`).
- `chronicle clean orphans` — sibling files left behind plus floating junk (paste cache, shell snapshots, rotated config backups).
- `chronicle clean dangling` — `~/.claude.json` project entries whose directory has gone, edited byte-preservingly with a backup.

### Trash

- `chronicle trash list/restore/empty`.

### Configure chronicle itself

- `chronicle config show/edit/path`.

### Architecture

- Two providers (Claude Code, GitHub Copilot Chat) plus a registry pattern for adding more.
- Optional capability interfaces (`Cleaner`, `MemoryStore`, `GlobalMemoryStore`, `Resumable`, `GlobalConfig`) discovered by type assertion.
- Provider-supplied defaults wherever a default would otherwise leak provider names into the CLI or composition layer.
- Provider-agnostic config: `Providers map[string]ProviderConfig` keyed by adapter name, no typed Claude/Copilot fields.

## Real coverage gaps surfaced by the December 2026 provider audit

See `docs/provider-surface.md` for the full analysis. These are the
pieces that cleanly extend the existing architecture without
re-opening any design questions.

### Copilot CLI session surface (real coverage gap)

The current Copilot adapter reads only the VS Code Chat side
(`<vscode>/User/workspaceStorage/.../chatSessions/`). It does not see
the Copilot CLI's session-state directory at `~/.copilot/session-state/`.
Real session data on the working machine lives there today and chronicle
is blind to it.

The right architectural question is whether to extend the existing
Copilot adapter to read both surfaces, or split into two adapters
(`copilot-vscode` and `copilot-cli`). Resolving that is one focused
research pass on the on-disk layout of the CLI sessions.

**Cost: medium.** New parsing logic for the Copilot CLI's session
schema, possibly a new adapter package.

**Risk: low.** Read-only by default. Same architectural pattern as the
existing adapter.

### `chronicle stats --by-model`

The user's session JSONL files carry a `model` field. Sessions routed
through MiniMax (or any other Anthropic-API-compatible backend) carry
a different value than native Claude sessions. A `--by-model`
breakdown would tell the user how their session count, message count,
and disk usage split across models.

The provider audit confirmed this is a uniform concept across both
tools (every session knows what model served it), so the chronicle
abstraction is just "a string the user might want to filter on" — a
new field on `SessionSummary`, not a new capability interface.

**Cost: small.** Add the field to `SessionSummary`, populate it in
each adapter's parse path, render the breakdown in stats.

**Risk: none.** Read-only addition.

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

The CLI tier (every "Ship now" and "Ship soon" item from earlier
revisions of this document) is done. The provider-surface audit at
`docs/provider-surface.md` revealed one real coverage gap and one
small follow-up. The remaining choices are about strategic direction.

The recommended order:

1. **Copilot CLI session surface.** This is a real coverage gap:
   chronicle today sees only the VS Code Chat side of Copilot, not
   the `~/.copilot/session-state/` side. Closing this is the most
   provider-agnostic thing we can do because it brings Copilot's
   coverage closer to Claude's.
2. **`chronicle stats --by-model`.** Small, surfaces the
   MiniMax-vs-Claude split, principled (it adds a field, not a
   capability).
3. **TUI** as the next big presentation layer over a now-stable
   capability surface. A TUI built today wraps a frozen feature set
   rather than chasing a moving target.
4. **Web frontend** if sharing rendered transcripts with teammates
   ever becomes the priority. Larger scope than TUI.
5. **A third provider adapter** (Cursor, Antigravity) if the
   multi-provider story is the next frontier. The optional-capability
   architecture is set up to make this additive: a new package under
   `adapters/`, one entry in `adapters/all.go`, no changes anywhere
   else.

The MCP and plugin management items below remain "Maybe" for the same
reasons noted at the time of writing: they would mostly duplicate what
the upstream tools already do well.
