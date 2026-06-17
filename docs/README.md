# Documentation

The reference and planning docs for chronicle, a local tool that browses, exports, and cleans the on-disk history that AI coding assistants leave behind. The user-facing overview is the [repository README](../README.md). This folder is for the people and agents who work on the code.

The project's code and prose follow the engineering-and-writing-standards skill, which is the single source of truth for style, taste, and conventions.

## Contents

- [Layout](#layout)
- [Reading order](#reading-order)

## Layout

```
docs/
├── README.md                ← you are here
├── next-session.md          ← standing cold-start prompt for a fresh session
├── codebase-tour.md         ← architecture + a file-by-file walkthrough
├── go-primer.md             ← Go idioms and the libraries chronicle uses
├── naming-conventions.md    ← the one canonical name per concept, applied to Go
├── provider-surface.md      ← the cross-provider taxonomy and capability matrix
├── feature-roadmap.md       ← the wide-angle view of what chronicle could become
├── backlog.md               ← the concrete, decided work list
└── research/
    ├── claude-code-storage.md   ← the ~/.claude layout and the cleanup rationale
    ├── copilot-storage.md       ← the VS Code Copilot and agent-runtime layout
    └── schema-resilience.md     ← surviving upstream format churn (the contract)
```

## Reading order

For someone joining the project today:

1. [`../README.md`](../README.md) — what chronicle does and the command surface.
2. [`codebase-tour.md`](codebase-tour.md) — the architecture, the dependency graph, and every package. Read this end to end.
3. [`go-primer.md`](go-primer.md) — the Go idioms and the libraries chronicle depends on. Skip the idioms if you know Go well, but the libraries section is worth a skim.
4. [`naming-conventions.md`](naming-conventions.md) — the canonical name for every concept and where Go style qualifies the rule.
5. [`provider-surface.md`](provider-surface.md) — what each provider exposes and how chronicle decides what to model.
6. [`research/claude-code-storage.md`](research/claude-code-storage.md) and [`research/copilot-storage.md`](research/copilot-storage.md) — the on-disk layouts the adapters and the cleanup logic trace back to.
7. [`research/schema-resilience.md`](research/schema-resilience.md) — how chronicle keeps working when an upstream tool changes its format.

For where the work is heading, read [`feature-roadmap.md`](feature-roadmap.md) for the strategic view and [`backlog.md`](backlog.md) for the concrete task list. To pick the work back up cold — in a fresh session, or after a lost one — start from [`next-session.md`](next-session.md).
