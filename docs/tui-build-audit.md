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

Phase 2 (transcript reader) is in progress. The code compiles
and the existing test suite is green, but the phase is not done.
What is left:

1. **Write unit tests for the transcript package.** The pattern
   to follow is `sessions_test.go`: define a Reader fake, drive
   the Model through Update messages, assert the state. Pin
   loading, ready, error, and the BackMsg emission on Esc.
2. **Live-test against real data.** Run `./chronicle`, press
   Enter on a session, confirm the transcript renders with
   glamour's `dark` style, confirm Esc goes back to the list,
   confirm scrolling and g/G work.
3. **Consider switching glamour to terminal-aware styling.**
   The first cut hard-codes `WithStandardStyle("dark")` because
   glamour v2 dropped `WithAutoStyle`. A future pass should
   either detect the terminal background colour or expose the
   choice through the user's chronicle config.
4. **Filter behaviour question.** The user observed that the
   session list's filter "only applies after 3 letters" and
   shows sessions with empty names before that. This is the
   bubbles list's default fuzzy filter being permissive on
   short queries combined with the fact that many real sessions
   have empty titles (the first user message had no text — only
   a tool result or an attached file). The renderer turns those
   into "(untitled)". Decisions to make:
   - Switch to substring (non-fuzzy) filtering. The bubbles
     list accepts `Filter list.FilterFunc`; `list.UnsortedFilter`
     is one option, a custom substring matcher is another.
   - Or set a minimum filter length so 1-2 character queries
     match nothing and the user sees the full list back.
   - Or surface the "(untitled)" label more clearly so it does
     not read as "empty."
5. **Commit and update the audit** once 1, 2, and 3 are done.

## Decisions

### 2026-05-21 — UI bar: intuitive and accessible by default

Every screen is designed against an explicit accessibility and
usability bar, set by the user during the session-1 build:

- **Keyboard-first, mouse-optional.** Every action has a key
  binding. Mouse wheel scrolling is enabled on lists and
  viewports, but no action is reachable only by mouse.
- **Vim AND arrow keys.** The default key bindings cover both
  conventions so a user who lives in Vim and a user who does
  not can each drive every screen without learning the other.
- **No multi-key chords.** A single keypress maps to a single
  action. Compound bindings (`g g`, `d d`) are not used.
- **Colour PLUS text.** Provider badges, status indicators, and
  error markers carry a text label as well as a colour, so a
  user with limited colour vision can still distinguish them.
- **Always-visible help bar.** The bottom of every screen shows
  the short help for the currently active bindings. The `?`
  key opens a full-help overlay with the same bindings grouped
  by purpose.
- **Full-sentence loading, empty, and error states.** A screen
  that is loading says so, a screen with no data explains why
  and what to do, a screen that errored quotes the underlying
  error and points the user at the next step.
- **High-contrast focused row.** The currently selected row in
  every list uses a reverse-video paint (or a strong background
  in the dark theme), not just bold or colour.
- **Refresh on demand.** Every screen that reads data exposes
  `r` to re-fetch, so a user who knows the data changed under
  them can update without quitting.
- **Predictable exit.** `q` quits the program from anywhere.
  `esc` cancels the current focus state (closes an overlay,
  clears a filter, leaves a transcript and returns to the
  session list).

These principles bind every screen, not just the first one.
Each phase's review pass checks the new screen against this
list.

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

### 2026-05-21 — Session 1 continued (phase 1: session list)

- Built phase 1: the session list screen.
  - Created `cmd/chronicle/tui/screens/sessions/`, the first
    real screen package. The Lister interface inside it is the
    minimal subset of composition.App the screen reads
    (ListSessionsAll only), so tests pass a fakeLister and
    production passes *composition.App.
  - The screen wraps each `composition.SessionListing` in a
    sessionItem and feeds it through a custom delegate that
    renders two lines per row: a header with the provider
    badge, the title (truncated to fit), and the relative time
    since last active, plus a subtitle with the project path
    in a muted style. The selected row uses the theme's
    reverse-paint Highlight so focus stays readable on
    terminals without rich colour.
  - Loading, empty, and error states are full sentences. The
    empty state points at `chronicle doctor`. The error state
    quotes the underlying error and also points at doctor.
  - The screen emits `sessions.OpenRequestMsg` on Enter. The
    app model logs it as a transient status line above the
    screen content. Phase 2 will replace the log with a real
    switch to the transcript reader.
  - `IsFiltering()` on the sessions Model lets the app model
    skip the global quit binding while the user is typing in
    the filter input. A user filtering for the string "quit"
    no longer triggers the program quit on the "q" keystroke.
  - The app model now reserves two terminal rows for the
    header it draws above the screen content. WindowSizeMsg is
    forwarded to the screen with `Height: msg.Height - 2`.
  - Six unit tests pin the contract: starts in loading state,
    Init returns a load command, loadedMsg populates and
    sorts, errMsg shows the error, empty result names the next
    step, Enter on a populated row emits OpenRequestMsg, and
    sessionItem.FilterValue covers title, project, and
    provider for the filter search.
  - `make check` is green.
  - Live-tested through `expect`: binary launches against the
    user's real Claude data, accepts j/k navigation, opens the
    filter and accepts typed input, escapes the filter, quits
    cleanly with status 0. The rendered alt-screen output is
    not capturable from outside the terminal without `tmux`
    capture-pane, so the visual review depends on the user
    actually running `./chronicle` and looking.

### 2026-05-21 — Session 1 continued (phase 1 fix: layout collapse)

- The user inspected the first phase-1 cut and flagged two
  blocking issues: the layout looked like a mess of randomly
  spread text, and items below the viewport could be focused
  but never came into view. The root cause was that session
  titles can carry raw newlines from the user's first pasted
  message, and the row delegate painted those newlines into
  the row. The bubbles list trusts the delegate to paint
  exactly Height() lines per item, so a row that painted ten
  or thirty lines broke pagination, scrolling, and the help
  bar.
- The fix introduces sanitizeSingleLine, the one helper that
  enforces the row's single-line invariant. Every title and
  every project path passes through it before reaching the
  delegate or the filter.
- displayProjectPath decodes Claude's dash-separated project
  identifiers back into absolute paths, then collapses the
  home prefix to "~". The Copilot adapters use opaque hashes
  that have no decoded form, and those pass through unchanged.
- The bubbles list's default purple "Sessions" title block is
  disabled. The screen now renders its own breadcrumb header
  ("chronicle · sessions") with a horizontal divider beneath
  it, and the app model's earlier header is gone — the screen
  owns its full layout.
- The selected row uses an accent-coloured bar marker plus a
  bold accent title rather than a full-row reverse paint.
  Focus reads through colour and weight together, which
  satisfies the accessibility bar without overpainting the
  row.
- Pressing Enter on a session pushes a transient notice into
  the list's built-in status bar via PublishStatusMessage,
  rather than pushing a status banner above the screen. No
  visual jitter.
- The app model shrinks to a router — Init forwards to
  sessions, Update handles globals and the OpenRequestMsg,
  View wraps the screen's content in a tea.View. The Screen
  interface lands in phase 2 when there is a second screen to
  abstract.
- Tests updated. Eight unit tests now cover the sessions
  package, including the sanitiser's invariants and the
  project-path decoder's cases.

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
