# Local data observations

Concrete numbers from the actual `~/.claude/` directory on 2026-05-15.
These motivate which features matter and which are theoretical.

## Disk usage

| Path | Size |
|---|---|
| `projects/` | **376 MB** (the conversations themselves) |
| `file-history/` | **67 MB** (versioned file backups, orphaned by deletion) |
| `plugins/` | 10 MB (out of scope — don't touch) |
| `history.jsonl` | 2.0 MB |
| `paste-cache/` | 712 KB |
| `tasks/` | 416 KB |
| `cache/` | 296 KB |
| `backups/` | 280 KB (5 backups of `.claude.json`) |
| `shell-snapshots/` | 148 KB |
| `plans/` | 36 KB |

**Total reclaimable by us (everything except plugins/skills/ide/settings): ~450 MB.**

## Sessions

- **66 session JSONL files** across **10 projects**.
- **287.7 MB** total in `projects/` (the rest of the 376 MB is folder overhead and metadata files).

Distribution of real user prompts per session (a "real" prompt = non-meta, non-tag-only user message):

| Prompts | Sessions |
|---|---|
| **0 (abandoned)** | **12** |
| 1 | 1 |
| 2–5 | 8 |
| 6–20 | 22 |
| 21–50 | 16 |
| 50+ | 7 |

**12 of 66 = 18% of sessions are abandoned** — created (often via `/clear` or by accident) and never used. They still hold ~18 KB each of session-start hooks and metadata. Direct confirmation of the user's complaint.

## The largest sessions

| Size | Prompts | Project |
|---|---|---|
| 22.3 MB | 86 | AGENTIC-POC-FINANCE |
| 21.0 MB | 41 | AGENTIC-POC-FINANCE |
| 16.0 MB | 33 | agent-claudia |
| 13.5 MB | 29 | agent-claudia |
| 12.4 MB | 15 | agent-claudia |

A 12 MB conversation with only 15 user prompts is almost entirely tool output. The "export without tool outputs" filter typically shrinks transcripts by **10–30x**.

## Content shape of a representative session (1336 lines, 5.8 MB)

Record type counts:
- `assistant`: 667
- `user`: 365
- `system`: 55
- `attachment`: 75
- `file-history-snapshot`: 36
- `last-prompt`: 96
- `queue-operation`: 42

Assistant content blocks:
- `thinking`: 290
- `tool_use`: 325
- `text`: **52**

User content blocks:
- `tool_result`: 325
- `string`: 28
- `text`: 14
- `image`: 7

**Read like this:** the assistant's 667 messages only produced 52 plain-text replies. Everything else is reasoning (`thinking`) and tool invocations. Of the user's 365 messages, 325 were tool results — only ~42 were actual human input.

Tool calls in that one session:
- `Bash` × 110, `Read` × 90, `Edit` × 65, `WebFetch` × 13, `MiniMax__web_search` × 13, `Write` × 12, `context7__query-docs` × 11, `understand_image` × 7, …

## Implications for design

1. **Filtering matters more than viewing.** The default preview should hide `tool_use` / `tool_result` / `thinking` and show conversation text. Toggling them back on is a single keypress.
2. **Abandoned-session cleanup is a real feature, not a nice-to-have.** 18% of sessions on this machine alone.
3. **Cleanup must follow sibling references.** Deleting just the JSONL leaves `file-history/<sessionId>/` (67 MB worth!), `tasks/`, `session-env/`, and `history.jsonl` entries dangling.
4. **First-prompt preview is the right list anchor.** "What did I ask?" is how users will recognize sessions. The very first non-meta user message is usually the prompt that started the work.
5. **Sessions are long.** Pagination / virtualized rendering is required from the first iteration — a 22 MB JSONL won't fit in memory naively.
