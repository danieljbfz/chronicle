# Changelog

All notable user-facing changes to chronicle land here. The format
follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project follows [Semantic Versioning](https://semver.org).

## [Unreleased]

### Added
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
