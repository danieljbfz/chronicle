# chronicle

A local tool for browsing, exporting, and cleaning the history that AI coding assistants leave on disk.

Status: under active development. See `docs/superpowers/specs/2026-05-15-chronicle-design.md` for the design contract.

## Install (from source, during development)

```bash
go build -o chronicle ./cmd/chronicle
./chronicle doctor
```

## Subcommands (Plan A scope)

- `chronicle list` — list Claude Code sessions, one JSON-line per session.
- `chronicle export <sessionId> [-o file.md]` — write a filtered Markdown transcript.
- `chronicle copy <sessionId>` — copy the same transcript to the clipboard via OSC52.
- `chronicle doctor` — show detected providers, their versions, and any format warnings.
