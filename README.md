# chronicle

A local tool for browsing, exporting, and cleaning the on-disk history that AI coding assistants leave behind. Multi-provider by design: works with Claude Code, the GitHub Copilot Chat extension, and the GitHub Copilot agent runtime today, ready for any tool with a similar layout.

Status: under active development. See `docs/superpowers/specs/2026-05-15-chronicle-design.md` for the design contract and `SKILL_PROMPT.md` for the engineering bar.

## Install (from source, during development)

```bash
go build -o chronicle ./cmd/chronicle
./chronicle doctor
```

`chronicle doctor` is the first thing to run. It tells you which providers chronicle detected on your machine, what storage version each is on, and whether anything looks off.

## What chronicle covers

Every command runs read-only by default. Anything destructive defaults to dry-run; pass `--apply` to perform the operation. Deletions go through a recoverable trash, not straight off disk.

### Browse and inspect

- `chronicle list` — list every session across every detected provider, one JSON line per session.
- `chronicle doctor` — show detected providers, their versions, and any format warnings.
- `chronicle stats` — one-screen summary of session counts, message counts, disk usage, and the active date range. `--json` for machine-readable output.
- `chronicle search <query>` — substring search across every session of every provider. Snippet-based results with `--json` for piping.

### Export and copy

- `chronicle export <sessionId> [-o file.md]` — write a filtered Markdown transcript to a file or stdout.
- `chronicle export --bulk <projectId> -o <directory>` — write one Markdown file per session, named with the session date. Streams through the renderer without holding all sessions in memory.
- `chronicle copy <sessionId>` — copy the same transcript to the clipboard via OSC 52.

Filter flags shared across export and copy: `--no-tools`, `--no-thinking`, `--no-meta`.

### Resume

- `chronicle resume <session-id>` — re-open the session in its original tool, in the original working directory. Provider-aware (Claude only today; Copilot returns a clear "this provider does not support resume" message because Copilot Chat lives inside VS Code with no external API).

### Manage memory

Memory here means the per-project files an AI tool reads at every session start (Claude's `projects/<encoded-cwd>/memory/`) and the user-global file every session reads regardless of project (`~/.claude/CLAUDE.md`).

- `chronicle memory list` — list every memory file across providers, both per-project and global.
- `chronicle memory show <project> <file>` — print one file to stdout. Use `--global` to target the user-global memory file (defaults to the active provider's canonical name).
- `chronicle memory edit <project> <file>` — open one file in `$EDITOR`. Same `--global` behavior.
- `chronicle memory clean <project>` — move every memory file in one project into the trash. Use `--global` to target the user-global file. Defaults to dry-run.

### Clean

Each subcommand finds one kind of cruft and (with `--apply`) moves it into the trash. Each defaults to dry-run.

- `chronicle clean abandoned` — sessions with zero real user prompts (the user opened the session, ran a meta command like `/clear`, never typed anything).
- `chronicle clean stale --older-than 30d` — sessions whose last activity is older than the threshold. Default 30 days, matching Claude's own `cleanupPeriodDays` default.
- `chronicle clean orphans` — files left behind after a session is gone (file-history, captured environment, task state) plus floating junk (old shell snapshots, rotated configuration backups, paste-cache entries with no live reference).
- `chronicle clean dangling` — entries in a provider's user-wide config file (Claude's `~/.claude.json` projects map) whose project directory no longer exists on disk. The original config file is backed up before any edit, and the edit is byte-preserving for everything outside the targeted entries.

### Trash

- `chronicle trash list` — list recoverable entries currently in the trash.
- `chronicle trash restore <entry-id>` — move one trashed entry back to its original location.
- `chronicle trash empty [--force]` — permanently remove entries past the retention window. With `--force`, removes everything regardless of age.

### Configure chronicle itself

- `chronicle config show` — print the resolved chronicle config (defaults plus any file overrides) as TOML.
- `chronicle config edit` — open the chronicle config file in `$EDITOR`. Creates the parent directory and an empty file when missing.
- `chronicle config path` — print the absolute path of chronicle's config file.

## Provider capability matrix

Not every provider supports every feature. The base `Provider` interface (read sessions, list projects) is required; everything else is an optional capability discovered by type assertion at runtime. Adding a new provider is one new package under `adapters/` plus one entry in `adapters/all.go`.

GitHub markets several products under the umbrella name "Copilot." Chronicle models each that writes local data as its own adapter, with honest names that reflect the product (`copilot-chat` for the VS Code Chat extension, `copilot-agent` for the `@github/copilot-sdk` runtime). See `docs/provider-surface.md` for the full reasoning.

| Capability | claude | copilot-chat | copilot-agent |
|------------|--------|--------------|---------------|
| List + read sessions | ✓ | ✓ | ✓ |
| Cleanup (abandoned, orphans, stale) | ✓ | ✓ | — |
| Per-project memory | ✓ | — | — |
| User-global memory (CLAUDE.md) | ✓ | — | — |
| Resume in original tool | ✓ | — | — |
| Dangling project entries (~/.claude.json) | ✓ | — | — |

## Recognized storage versions

| Provider | Version | Fingerprint | First seen |
|---|---|---|---|
| Claude Code | `claude-1.0` | `25ce9fd0794c` | 2026-05-15 (Claude Code 2.1.x) |
| GitHub Copilot Chat | `copilot-3` | `2e10591741e1` | 2026-05-15 (VS Code with chat schema v3) |
| GitHub Copilot agent | `copilot-agent-1` | (no fingerprint yet) | 2026-05-15 (`@github/copilot-sdk` LocalSessionManager) |

If `chronicle doctor` shows `Version: unknown`, the fingerprint did not match the table above. The tool still works in read-only mode — the resilience contract is documented in `docs/research/07-schema-resilience.md`.

## Help

Every command has a `--help` flag. The top-level `chronicle --help` lists every subcommand with a one-liner. Each subcommand's `--help` shows its full usage, flags, and description.

```bash
chronicle --help
chronicle clean --help
chronicle clean dangling --help
```
