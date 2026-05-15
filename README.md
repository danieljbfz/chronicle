# chronicle

A local tool for browsing, exporting, and cleaning the history that AI coding assistants leave on disk.

Status: under active development. See `docs/superpowers/specs/2026-05-15-chronicle-design.md` for the design contract.

## Install (from source, during development)

```bash
go build -o chronicle ./cmd/chronicle
./chronicle doctor
```

## Subcommands

- `chronicle list` — list every session across every detected provider, one JSON line per session.
- `chronicle export <sessionId> [-o file.md]` — write a filtered Markdown transcript.
- `chronicle copy <sessionId>` — copy the same transcript to the clipboard via OSC 52.
- `chronicle doctor` — show detected providers, their versions, and any format warnings.
- `chronicle clean abandoned [--apply]` — find sessions with zero real user prompts and (with `--apply`) move them into the trash. Defaults to dry-run.
- `chronicle trash list` — list recoverable entries currently in the trash.
- `chronicle trash restore <entry-id>` — move one trashed entry back to its original location.
- `chronicle trash empty [--force]` — permanently remove trash entries past the retention window. With `--force`, removes everything regardless of age.

## Recognized storage versions

| Provider | Version | Fingerprint | First seen |
|---|---|---|---|
| Claude Code | `claude-1.0` | `25ce9fd0794c` | 2026-05-15 (Claude Code 2.1.x) |
| GitHub Copilot Chat | `copilot-3` | `2e10591741e1` | 2026-05-15 (VS Code with chat schema v3) |

If `chronicle doctor` shows `Version: unknown`, the fingerprint did not match the table above. The tool still works in read-only mode — the resilience contract is documented in `docs/research/07-schema-resilience.md`.
