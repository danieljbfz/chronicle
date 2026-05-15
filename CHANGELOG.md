# Changelog

All notable user-facing changes to chronicle land here. The format
follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project follows [Semantic Versioning](https://semver.org).

## [Unreleased]

### Added
- `chronicle stats --by-model` adds a per-model breakdown to
  the stats summary. All three adapters now populate the
  `Model` field on `SessionSummary`. Claude reports the most-
  frequent per-message model, the Copilot agent runtime reads
  `selectedModel` from `session.start`, and the Copilot Chat
  extension reads `inputState.selectedModel.identifier`.
  Sessions whose adapter could not determine the model land
  in the `(unknown)` bucket.
- `adapters/copilotagent/` is a new read-only adapter for the
  GitHub Copilot agent runtime (the `@github/copilot-sdk`
  `LocalSessionManager` at `~/.copilot/`). The data has zero
  overlap with the Copilot Chat extension, so it lives as a
  separate adapter rather than a version of `copilotchat`.
- `chronicle search` finds substring matches across every
  session of every provider, with snippet-based results and
  a `--json` flag for piping. The search runs serially today
  and finishes in a few hundred milliseconds for typical
  installs.
- `chronicle resume` reopens a Claude session in its
  original tool, with the right working directory and the
  right `--resume` argument.
- `chronicle memory` lists, edits, and prunes per-project and
  user-global memory files. The Claude adapter surfaces both
  per-project memories (`projects/<encoded-cwd>/memory/`) and
  the user-global `~/.claude/CLAUDE.md`.
- `chronicle config show / edit / path` for inspecting and
  editing chronicle's own TOML configuration.
- `chronicle clean stale` removes sessions older than a given
  age, expressed in days or any Go duration.
- `chronicle clean dangling` removes stale entries from
  Claude's global `~/.claude.json` projects subsection
  when the directory they point at is gone.
- `chronicle export --bulk` writes one Markdown file per
  session in a project to a directory in a single call.
- `chronicle clean abandoned` finds sessions with zero real
  user prompts and (with `--apply`) moves them into the trash.
  Defaults to dry-run, so the user always sees the plan before
  any file moves.
- `chronicle trash list/restore/empty` manages trashed entries.
  Restore puts an entry back where it came from. Empty removes
  entries past the retention window from the user's config (30
  days by default), with a `--force` flag for the rare case
  when the user wants to clear everything immediately.
- Cascade-aware deletion. Removing a Claude session also removes
  its `file-history/`, `tasks/`, `session-env/`, and `sessions/`
  metadata. Removing a Copilot session also removes its
  `chatEditingSessions/` directory.
- Trash subsystem with manifest files, atomic per-entry moves
  with rollback on failure, and cross-filesystem support
  through a copy+remove fallback when `os.Rename` cannot work.
- Both adapters now satisfy `contracts.Cleaner` with
  `PlanDelete` and `PlanOrphanScan`.

### Changed
- The single Copilot adapter split into `copilotchat`
  (for the VS Code Copilot Chat extension under
  `workspaceStorage/`) and `copilotagent` (for the
  `@github/copilot-sdk` agent runtime under `~/.copilot/`).
  These are two distinct products with non-overlapping data
  on disk, and the adapter packages now reflect that. Config
  uses the registered names `copilot-chat` and
  `copilot-agent`.
- Provider-level error wrapping is now uniform across all
  three adapters. Every public Provider method wraps its
  underlying error with an operation-context `newError`
  value, so the caller sees `claude: list sessions: <path>:
  <underlying>` instead of a bare `*fs.PathError`.
- Sessions that begin with a Claude Code slash command
  (`/clear`, `/compact`, ...) no longer surface the
  `<command-name>` markup as their title. The parser
  recognizes those records and marks them `IsMeta`, so the
  title fallback reads through to the next real prompt.
- `chronicle list` truncates session titles at 200 runes
  with an ellipsis. Real sessions sometimes use a multi-page
  pasted specification as the first user message, and the
  uncapped title made the JSONL output unusable for shell
  pipelines.
- `chronicle doctor` always prints the `Fingerprint:` line,
  using an em-dash placeholder when the adapter does not
  compute one. Every provider block now reads with the same
  shape.

### Added (earlier)
- Read-only Claude Code adapter. Detects sessions in `~/.claude`,
  parses JSONL session files, and surfaces unknown record types
  through the resilience contract.
- Read-only GitHub Copilot Chat adapter. Reads VS Code's per-workspace
  chat sessions, replays the snapshot-and-mutation event log, and
  decodes markdown, thinking blocks, tool invocations, and
  inlined `MarkdownString` values.
- `chronicle list`, `chronicle export`, `chronicle copy`, and
  `chronicle doctor` subcommands.
- OSC 52 clipboard copy that works over SSH.
- Hexagonal architecture (Ports and Adapters) with strict
  dependency-graph rules between contracts, adapters, steps,
  composition, and entrypoints.
- Tolerant parsers that surface unknown content as `UnknownBlock`
  values, both for record types (Claude) and for response part
  kinds (Copilot).
- Schema fingerprinting: detection produces a short hex hash of the
  storage shape, mapped to internal version codes via a lookup
  table that grows as we encounter new releases of upstream tools.
