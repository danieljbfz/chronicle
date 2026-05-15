# Project docs

This project is in the **brainstorming / research** phase. No code yet.

## Scope (as of 2026-05-15)

A local tool that browses, exports, cleans, and shares the on-disk history of AI coding assistants. Targets two frontends and two providers from day one.

- **Providers (v1):** Claude Code (`~/.claude/`) and GitHub Copilot Chat / Copilot CLI (`~/Library/Application Support/Code/User/...`). Cursor and JetBrains shipped as separate adapters when a user asks.
- **Frontends (v1):** a terminal UI (Bubble Tea, single static binary) and a local web UI (the same binary launches `localhost:<port>`). Both consume the same composition layer.
- **Format resilience:** every adapter detects the storage version, falls back gracefully on unknown shapes, and writes a structured format-report when it sees something new. See `research/07-schema-resilience.md`.

The engineering bar is `SKILL_PROMPT.md` at the repository root — read that first if you have not.

## Layout

```
docs/
├── README.md                                  ← you are here
├── go-primer.md                               ← Go idioms for this project (new contributors start here)
├── naming-conventions.md                      ← canonical names + Go-vs-SKILL_PROMPT.md resolutions
├── research/                                  ← findings (what we learned)
│   ├── 01-claude-code-storage.md              ← ~/.claude folder + JSONL format
│   ├── 02-existing-tools-landscape.md         ← competitor analysis, gaps
│   ├── 03-tui-libraries.md                    ← language + library survey
│   ├── 04-local-data-observations.md          ← stats from this user's data
│   ├── 05-feature-ideas.md                    ← feature inventory
│   ├── 06-copilot-storage.md                  ← VS Code Copilot + CLI + Cursor + JetBrains
│   └── 07-schema-resilience.md                ← surviving format churn (the contract)
└── superpowers/
    ├── specs/                                 ← design docs (one per topic)
    └── plans/                                 ← implementation plans
```

## Reading order

For a reader joining the project today:

1. `SKILL_PROMPT.md` (repo root) — the engineering contract.
2. `go-primer.md` — Go idioms used in this codebase. Skip if you already know Go well.
3. `naming-conventions.md` — canonical names for every concept.
4. `research/04-local-data-observations.md` — concrete numbers from real usage.
5. `research/01-claude-code-storage.md` — Claude data model.
6. `research/06-copilot-storage.md` — Copilot data model.
7. `research/07-schema-resilience.md` — how we survive format changes.
8. `research/02-existing-tools-landscape.md` — the gap we are filling.
9. `research/03-tui-libraries.md` — recommended stack (Go + Charm).
10. `research/05-feature-ideas.md` — feature inventory tagged `[CORE / NICE / LATER / OPEN]`.

The current design spec lives at `superpowers/specs/2026-05-15-chronicle-design.md` — read that for the system contract. When the spec is approved, the implementation plan lands in `superpowers/plans/`.

## Open meta-questions for the next round

- **Repo / binary name.** `claude-history` describes the current directory but signals Claude-only. Candidates: `convo`, `threads`, `transcripts`, `chatlog`, `histo`, `loom`. Recommendation withheld until we hear preferences.
- **Web frontend scope.** Read-only viewer first, or read-write parity with the TUI from day one? Defaults to read-only because most cleanup actions are easier to confirm in a terminal.
- **v1 cut.** The feature inventory has more than v1 can carry. The proposed minimum cut is in `05-feature-ideas.md` under `[CORE]`. The list is open for redirection.
