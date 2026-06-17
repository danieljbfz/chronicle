# Feature roadmap

The wide-angle view of what chronicle could become, organized by user value and implementation cost. This document is opinionated and strategic. The concrete, decided work items live in [`backlog.md`](backlog.md), and the cross-provider reasoning that the directions below depend on lives in [`provider-surface.md`](provider-surface.md).

The mental frame is that **chronicle is a multi-provider history manager**. The user's history is sessions, but it is also memory, plans, settings, plugins, and the artifacts every AI coding assistant writes alongside its conversations. Chronicle can be useful in any of those.

## Contents

- [Where things stand](#where-things-stand)
- [Directions worth weighing](#directions-worth-weighing)
  - [More provider adapters](#more-provider-adapters)
  - [The web frontend](#the-web-frontend)
  - [MCP server inspection](#mcp-server-inspection)
  - [Plugin inspection](#plugin-inspection)
- [Directions to skip](#directions-to-skip)
- [Recommended direction](#recommended-direction)

## Where things stand

The CLI surface is feature-complete across three providers (Claude Code, the VS Code Copilot Chat extension, and the Copilot agent runtime). Every command runs read-only by default, every destructive command defaults to dry-run, and every deletion goes through the recoverable trash — or, for `clean dangling`, through a backup-first surgical edit. The capabilities are: multi-provider listing, doctor, stats with a per-model breakdown, substring search, single and bulk export, clipboard copy, resume, per-project and user-global memory management, four cleanup categories, the trash lifecycle, and chronicle's own config commands.

The terminal UI is in progress on top of that stable surface. The web frontend has not started. The concrete in-flight and to-do items for both are tracked in [`backlog.md`](backlog.md). This document is about the directions beyond that list.

## Directions worth weighing

### More provider adapters

A new tool — Cursor, Antigravity, or any other assistant that persists conversations locally — is the most natural way to grow chronicle's value, because the multi-provider story is the whole point. The optional-capability architecture makes it additive: a new package under `adapters/`, one entry in `adapters/all.go`, and no changes anywhere else. Cursor is the obvious next target. It is a VS Code fork that stores chat in `state.vscdb` rather than per-session JSONL, so it needs a SQLite reader the other adapters do not.

The cost is medium, because each tool needs its own parser. The risk is low, because the work is read-only and follows the same pattern as the existing adapters.

### The web frontend

A local, single-binary web app for browsing sessions and sharing rendered transcripts in a browser. Read-only in its first cut: browse, preview, search, export. The destructive flows stay in the TUI, because they are harder to make safe over HTTP. The value is sharing a clean transcript with a teammate without an export step, and viewing on a second screen while working in a terminal.

The cost is medium to large. The web app adds a new entrypoint layer, server-rendered HTML, and a small set of routes over the same composition API the TUI already uses.

### MCP server inspection

MCP servers are configured per-project and globally in a tool's config files, and both Claude and Copilot have their own commands for managing them. Chronicle could offer a read-only cross-provider listing (`chronicle mcp list`) and a reachability check (`chronicle mcp doctor`). The useful piece is the cross-provider view that neither tool gives on its own. Editing the configs is out — chronicle would become a thin, lossy wrapper over commands the tools already do well.

The cost is medium, because each tool's config file needs careful parsing. The verdict is a maybe, and it should wait for a concrete user need before anyone builds it.

### Plugin inspection

Claude Code has a plugin system. Chronicle could list installed plugins and check each for breakage. The cross-provider angle is hollow, because Copilot has no equivalent plugin system, so chronicle would mostly duplicate Claude's own plugin commands.

The verdict is a maybe that leans toward skip, until a specific need shows up.

## Directions to skip

- **A daemon that watches for new sessions.** It would add a background process and the operational cost that comes with it, and Claude Code already runs its own cleanup at startup.
- **A share-to-gist command.** It pulls chronicle into auth-token management. The composition `chronicle export -o foo.md && gh gist create foo.md` is worth more than a baked-in command.
- **Plugin or MCP installation.** Adding extensions is exactly what the upstream tools' own commands are for. Translating the concept across providers is a coordination headache for a job the tools already do well.
- **Cloud sync or backup.** Out of scope. It would require server-side state and accounts. The user can archive their chronicle config and sync it themselves.

## Recommended direction

The CLI is done and the capability surface is stable. The strategic choices left are about which presentation layer or which new provider comes next.

1. **Finish the TUI.** It is the everyday face of chronicle for interactive use, and it wraps a frozen feature set rather than chasing a moving target. The remaining screens are tracked in the backlog.
2. **The web frontend**, if sharing rendered transcripts with teammates becomes the priority. Larger scope than the TUI.
3. **A third-party provider adapter** (Cursor, Antigravity), if widening the multi-provider story is the next frontier.

The MCP and plugin inspection ideas stay Maybes for the reason noted above: they would mostly duplicate what the upstream tools already do well.
