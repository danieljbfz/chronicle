# TUI build audit

This file is the running record of the TUI work. It exists so any
session can pick the thread up cold — read this file, the handoff
in `docs/tui-and-web-handoff.md`, and the latest entry under
"Session log", and you should know exactly where the work stands
and what to do next.

The file is append-mostly. Decisions are not deleted when they are
superseded — the old decision gets a strike-through and the new
one gets its own entry with a reason. The point is to preserve the
reasoning, not just the current state.

Last touched: 2026-05-21.

## Goal

Build the terminal UI described in `docs/tui-and-web-handoff.md`.
The TUI becomes the default behaviour of `chronicle` when the user
runs the binary with no arguments. It surfaces `composition.App`'s
existing read and action methods through an interactive set of
screens. The CLI keeps its current shape for scripting.

## Current phase

Phase 0 (foundation) is complete and on a clean working tree
ready for commit. Phase 1 (session list screen) is next.

## Decisions

### 2026-05-21 — TUI library stack: Charm v2

The TUI is built on the Charm v2 stack:

- `charm.land/bubbletea/v2` for the runtime and message loop.
- `charm.land/bubbles/v2` for `list`, `viewport`, `table`,
  `textinput`, `key`, and `help`.
- `charm.land/lipgloss/v2` for layout and styling.
- `github.com/charmbracelet/glamour` for Markdown rendering
  inside the transcript viewport.

The other candidates considered were `tview` (slower-moving,
retained-mode widget tree that fights the immutability discipline
in `composition/`) and `tcell` used directly (every component the
handoff calls out becomes a piece of work we re-do from scratch).
Both were ruled out in favour of the Charm stack's reusable
components, active maintenance, and the visual bar already cleared
by `glow` and `gh-dash`. The risks are documented in the
"Risks the user should know about" section of session 1.

### 2026-05-21 — File layout: per-screen sub-packages

The TUI lives at `cmd/chronicle/tui/`. The package tree is:

```
cmd/chronicle/tui/
├── tui.go              ← Run(app *composition.App) error, the entry point
├── app.go              ← top-level tea.Model and screen routing
├── app_test.go
├── messages.go         ← cross-screen tea.Msg types
├── keys/               ← package keys, shared key bindings
├── theme/              ← package theme, lipgloss styles
└── screens/
    ├── sessions/       ← phase 1
    ├── transcript/     ← phase 2
    ├── stats/          ← phase 3
    ├── doctor/         ← phase 4
    ├── trash/          ← phase 5
    └── memory/         ← phase 6
```

The dependency graph stays acyclic. The leaf packages (`keys`,
`theme`) depend on nothing else inside the TUI. Each screen
package depends on `keys`, `theme`, and `composition` directly.
Only the top-level `tui` package imports the screens, and it
does so through a `Screen` interface declared in `app.go` so the
routing stays decoupled from any one screen's internals.

The reason for this shape over a single flat package is that
chronicle already uses one-package-per-bounded-context elsewhere
(`adapters/claude/`, `adapters/copilotchat/`,
`adapters/copilotagent/`, `internal/config/`,
`internal/paths/`). The TUI screens are six bounded contexts of
the same flavour. The flat-package alternative is what `glow`
does and would have been workable too, but it surfaces the
screen boundary only through file names rather than through
package boundaries the compiler enforces.

### 2026-05-21 — Open questions from the handoff: defaults adopted

| Question | Resolved as |
|---|---|
| TUI as no-args default vs `chronicle tui` subcommand | No-args default. Running `chronicle` with no arguments launches the TUI. The CLI subcommands are untouched and remain the scripting surface. |
| v1 screen set | Six screens — session list, transcript reader, stats, doctor, trash, memory. |
| Keyboard model | Vim-style bindings (`j`/`k`/`g`/`G`/`/`, etc.) where the Vim equivalent is obvious, arrow keys and Enter as fallbacks so non-Vim users can drive every screen. |
| Theme | Follow the terminal's own palette by default. One opt-in dark theme via a `--theme dark` flag or a config setting. No light-theme variant in v1. |

These match the handoff's defaults. If any one of them is wrong
for the project's direction, the next session can override the
decision here and add a successor entry rather than editing the
original.

## Open questions

The handoff's "Open questions" table is resolved under
"Decisions" above. New questions land here as the build surfaces
them, and each one moves to "Decisions" with a dated entry once
it is answered.

*(none currently open)*

## Session log

The log is per-session, newest entry at the bottom inside each
session. A session ends when the user closes the conversation, and
the next session starts a new dated heading even if it is the same
calendar day.

### 2026-05-21 — Session 1 (handoff acceptance + research kickoff)

- Read `docs/tui-and-web-handoff.md` end to end and walked the
  `composition/`, `cmd/chronicle/`, and `adapters/` layout.
- Ran `make check` against a clean working tree. All ten packages
  pass `gofmt`, `go vet`, `golangci-lint`, and `go test -race`.
- Audited the handoff's composition-API table against the real
  `composition.App` surface. Found four method-name errors and
  one false sort claim, and fixed them in place in the handoff.
  The surface itself was not changed — the doc was wrong, the
  code was right.
- Created this audit file.
- Ran the TUI library survey. The candidates considered were the
  Charm v2 stack (`charm.land/bubbletea/v2` plus `bubbles/v2` plus
  `lipgloss/v2`), `tview` (retained-mode widgets on `tcell`),
  `gocui` (minimalist immediate-mode helper on `tcell`), and
  `tcell` used directly. Cross-language alternatives (Ratatui in
  Rust, OpenTUI in TypeScript/Zig) are off the table because
  chronicle is a single Go binary. Key facts established:
  - Bubble Tea v2 shipped on 2026-02-23 and is production-tested
    (Charm's Crush agent has been on v2 for months). The new
    "Cursed Renderer" is orders of magnitude faster than v1, and
    the Mode 2026 synchronized-output flag cuts screen tearing
    on modern terminals.
  - `bubbles` v2 exposes the components chronicle needs out of the
    box: `list` (with filtering and pagination), `viewport`
    (scrolling with gutters and mouse), `table`, `textinput`,
    `key` (declarative key bindings with help text), and `help`.
  - `tview` is still maintained but moves slower, and its
    retained-mode widget model fights against the Elm-style
    composition the rest of the Charm ecosystem rewards.
  - `glow` (Charm's own Markdown reader) and `gh-dash` (a
    GitHub dashboard) are the two reference apps. Both use
    Bubble Tea plus lipgloss plus `glamour` for Markdown
    rendering, and both organise their UI in an `internal/ui/`
    directory with one file per screen. That layout is the
    obvious template for chronicle, and it lines up with
    chronicle's existing `composition/` and `cmd/chronicle/`
    layering.
- Surfaced the survey as a written recommendation. The user
  approved the Charm v2 stack.
- Recorded the file-layout decision (per-screen sub-packages,
  Option B) under "Decisions" after the user pushed back on a
  flat layout. The shape mirrors chronicle's existing
  `adapters/<provider>/` convention.
- Built phase 0: the foundation.
  - Added the Charm v2 dependencies to `go.mod`:
    `charm.land/bubbletea/v2 v2.0.6`,
    `charm.land/bubbles/v2 v2.1.0`,
    `charm.land/lipgloss/v2 v2.0.3`. Lipgloss v2 is now a
    stable release rather than the beta the recommendation
    flagged as a risk — the "beta" risk in the original
    recommendation no longer applies.
  - Scaffolded `cmd/chronicle/tui/` with `tui.go` (the entry
    point), `app.go` (the top-level model), `messages.go` (a
    placeholder for cross-screen tea.Msg types phase 1 will
    introduce), and the leaf packages `keys/keys.go` and
    `theme/theme.go`.
  - Wired the root cobra command's `RunE` to launch the TUI
    when no subcommand is given. Every existing CLI subcommand
    is unchanged, and `chronicle --help` still prints the help.
  - Discovered three v2 API changes during phase 0 that the
    earlier research had not surfaced. They are recorded under
    "Bubble Tea v2 API notes" below so a future session does
    not relearn them.
  - Wrote unit tests in `cmd/chronicle/tui/app_test.go` that pin
    the welcome screen's contract (renders version, body, and
    help text; alt-screen requested) and the global quit
    behaviour (pressing `q` resolves to a `tea.QuitMsg`).
  - Verified `make check` is green (gofmt, vet, golangci-lint,
    `go test -race ./...`).
  - Live-tested the binary by driving it through `expect`. The
    binary launches, accepts `q`, and exits cleanly with
    status 0. `tmux capture-pane` would be the next step for
    golden-file integration tests, but `tmux` is not installed
    on this machine. Phase 1 will need it or `teatest`.

## Bubble Tea v2 API notes

These are the v2 changes that hit during phase 0 and matter for
every subsequent screen.

1. `View()` returns `tea.View`, not `string`. The `tea.View`
   struct has fields for `Content`, `AltScreen`, `Cursor`,
   `BackgroundColor`, `ForegroundColor`, `WindowTitle`,
   `ProgressBar`, and `OnMouse`. Each frame is self-describing.
   Use `tea.NewView(content)` to build one, then set the fields
   that matter.
2. `tea.WithAltScreen()` is gone. Alt-screen mode is per-frame on
   the `tea.View` value (`view.AltScreen = true`). The top-level
   app model in `cmd/chronicle/tui/app.go` always returns frames
   with `AltScreen = true`.
3. Key messages are `tea.KeyPressMsg`, not `tea.KeyMsg`. The
   underlying type is `tea.Key`, and `bubbles/v2/key.Matches`
   takes any `fmt.Stringer`, which both `KeyPressMsg` and the
   string representation satisfy.

The v2 announcement post called the second point the "declarative
shift" — view fields replace imperative commands. The full
upgrade guide is at
`github.com/charmbracelet/bubbletea/blob/main/UPGRADE_GUIDE_V2.md`.

## How to continue if I am gone

If you are a new session reading this for the first time:

1. Read this file end to end.
2. Read `docs/tui-and-web-handoff.md` end to end.
3. Read `docs/codebase-tour.md` and `docs/feature-roadmap.md`.
4. Run `make check`. If anything is red, that is your first task.
5. Look at the "Current phase" section above. Resume from there.
6. Add a new dated heading under "Session log" before you do
   anything that changes the state.
