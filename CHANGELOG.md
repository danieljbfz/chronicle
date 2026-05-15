# Changelog

All notable user-facing changes to chronicle land here. The format
follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project follows [Semantic Versioning](https://semver.org).

## [Unreleased]

### Added
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
