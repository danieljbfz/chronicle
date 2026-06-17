# Next session

A standing cold-start prompt for picking chronicle back up. Drop it into a fresh session if the last one is lost. It is short on purpose and points at the living docs, so it stays current as long as those do.

## Contents

- [The project](#the-project)
- [Read first](#read-first)
- [Where the work stands](#where-the-work-stands)
- [How to work](#how-to-work)

## The project

chronicle is a Go tool that browses, exports, and cleans the on-disk history that AI coding assistants leave behind — Claude Code and two GitHub Copilot products today. It is hexagonal: `contracts/` is the leaf, `adapters/` and `steps/` depend on it, `composition/` orchestrates them, and `cmd/chronicle/` holds the CLI and the TUI. Running `chronicle` with no arguments opens the TUI, and the subcommands are the scripting surface.

## Read first

1. [`codebase-tour.md`](codebase-tour.md) — the architecture and every package. Read it end to end.
2. [`backlog.md`](backlog.md) — the concrete work list, including the current priority.
3. [`go-primer.md`](go-primer.md) — the Go idioms and the libraries chronicle uses, if Go or the Charm TUI stack is new to you.

The project's code and prose follow the engineering-and-writing-standards skill. Load it before writing anything.

## Where the work stands

The CLI is feature-complete across the three providers, and the TUI is partway built on top of it. What is built, what is left, and the bugs to fix next are all tracked in [`backlog.md`](backlog.md) — start there.

### What the last session did

The most recent session solved the top performance item: session listing and stats were re-architected to stop fully parsing every file. `Provider.ListSessions` became a parse-free `ListSessionRefs` plus an expensive `SummarizeSession`, and composition gained a persistent per-session cache (`composition/summary_cache.go`, keyed on size, modification time, and storage fingerprint). `chronicle stats` went from ~38 s to 0.10 s warm. The architecture is in [`codebase-tour.md`](codebase-tour.md) and the result is ticked in [`backlog.md`](backlog.md).

Before that, a long bug-fix and audit pass landed sixteen fixes, each grounded against real on-disk data and covered by a regression test, with `make check` green from a clean cache. Every one is ticked in [`backlog.md`](backlog.md) with the root cause written up. The highest-impact were:

- **Copilot-chat sessions rendered empty.** The event-log replayer typed patch paths as `[]string`, but VS Code writes array indices as JSON numbers, so the streaming decoder aborted the whole replay at the first such patch. A second bug treated a kind-2 append value as one element rather than the array of elements to append. Both fixed in `adapters/copilotchat/eventlog.go`; 34 of 72 multi-line sessions on the author's machine were affected, and one now exports 5,359 lines of correct content.
- **The stats loading freeze you may have seen** — switching screens mid-load stranded a background screen's spinner and dropped its result. Fixed in the TUI router (`cmd/chronicle/tui/app.go`): input goes to the focused view, background messages broadcast to every screen.
- **Two data-safety bugs** — a cross-device trash move could delete the source after a failed write, and `chronicle resume` could hang forever on a truncated session line.
- **The render-path cluster** — encrypted/"omitted" thinking blocks, fence-unsafe Markdown, image and tool-reference tool results, blockless messages, and tool-result headings, all settled consistently across the three adapters.

### Two things to know before you start

1. **The working tree is uncommitted and mixed.** Three layers of work sit unstaged at once: the listing-and-cache feature (the `contracts`, adapter, and `composition` changes plus the new `session_ref.go`, `session_summary.go`, `session_list.go`, and `summary_cache.go` files), the earlier sixteen-fix batch, and an older docs reorganization that predates both — for example `steps/filter.go` shows as modified though neither of the later passes touched it. Do not blind-commit `git add -A`. The author sequences version control by hand. Read the diff and confirm with the author before staging anything.
2. **What is ready to pick up next.** With the performance item closed, the ready bugs are the unfiltered TUI transcript (the reader calls `steps.Markdown` directly while every export path filters first), the bare system/meta record rendering, and the small footguns (path-traversal in a trash id, a doc that claims an unwired key binding). The larger remaining build-out is the doctor, trash, and memory TUI screens and the web frontend. All of these are written up in [`backlog.md`](backlog.md) — start there and confirm each entry against the code before acting on it.

## How to work

Research, plan, execute, review. Run `make check` (format, vet, lint, race tests) before every commit, keep commits small, and verify against real data — a green build is the floor, not the ceiling. Push back when a suggestion would make the code worse, with the reason. The full bar is the engineering-and-writing-standards skill.
