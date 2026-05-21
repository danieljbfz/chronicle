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

Phase 3 (stats) is functionally complete and the cross-cutting
design pass that followed it is largely done. Phase 4 (doctor)
is the next screen to build, but two open items from the
phase-3 polish work block the move forward — a scroll-latency
report from the user and the deeper `composition.Stats`
performance gap. Both are recorded under "Open questions"
below. The full test suite is green across every package,
including the new `frame_test.go` that pins the load-bearing
properties of `ui.Frame`.

The chrome the user sees today follows one rule: every
top-level screen renders through `ui.Frame` (loading row
centred, body padded to fill the body region with a divider
above the footer, single-line help row that always ends with
`?` and `q`). The bubbles list's built-in status bar handles
the row count for the session list, because pulling that row
up to a frame-level concept produced a height-accounting bug.
The transcript reader is a drill-down overlay with a
one-line metadata header followed by the same frame body and
footer; the breadcrumb chrome that competed with the metadata
strip is gone.

Phase 4 (doctor) reads `composition.App.Doctor` and renders
one card per detected provider with the root, the version,
the fingerprint, the reachability flag, the session count,
and any warnings or errors. The shape to follow is the stats
screen: a `Source` interface that is the minimal
`composition.App` subset, a `Model` that maps loading/ready/
error states to `ui.Frame` states, content sized to the
frame's body region, and a small `footerBindings` slice the
frame appends `?` and `q` to.

The screen needs one more entry in `app.go::newAppModel`'s
section order, meta, and registry — the tab strip grows
automatically from there.

What is left before phase 4 starts:

1. **Research the doctor layout.** Read
   `composition/doctor.go` for the result shape,
   `cmd/chronicle/doctor.go` for how the CLI already
   renders it, and decide whether one card per provider or
   one table row per provider reads better in the
   terminal. Surface to the user before writing code.
2. **Execute, review, commit** on the same cadence the
   earlier phases used.

The remaining open question is the latent `Stats` perf
hazard (`ListSessions` parses every file). It does not
block phase 4 because lazy section initialisation already
hides the cost from first launch — the user only pays it
when they open stats. The fix is a composition-layer
refactor recorded under "Open questions" below.

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

### 2026-05-21 — Listing identity: Conversation.ListingTitle

The session list showed "(untitled)" rows that the user read as
empty, and the bubbles fuzzy filter promoted those rows to the
top of short-query results. The root cause was not the filter.
Every adapter set `SessionSummary.Title` from
`Conversation.FirstUserPrompt`, which is documented to return
the empty string for sessions whose first turn carried no real
user text — a slash command like `/clear`, a tool result, or an
attached file. `FirstUserPrompt` is correct for the transcript
header (it deliberately skips meta records so the transcript
does not open with `<command-name>/clear</command-name>`), but
the listing has the opposite need: every row wants a
recognizable label even when there is no real user prompt.

The fix is `Conversation.ListingTitle`, a new method in
`contracts/listing_title.go`. It runs a six-rung cascade and
returns the first non-empty value: the upstream-recorded Title,
the first real user prompt, the first slash-command name
(extracted from the `<command-name>` wrapper), the first
assistant reply, the first tool name, and finally a synthetic
identity built from `StartedAt` or `SessionID`. The method
never returns the empty string, and a property test pins that
invariant across five pathological conversation shapes. All
three adapters now set `SessionSummary.Title` from
`ListingTitle`, and the Markdown export heading uses it too.
`FirstUserPrompt` stays unchanged for the transcript header and
for `IsAbandoned`, which both genuinely care about real user
text rather than recognizable identity.

With every row carrying a real name, the bubbles fuzzy filter
was left as-is rather than switched to substring matching. The
"untitled rows float to the top" symptom was a consequence of
the empty titles, not of the matcher, so fixing the data fixed
the filter.

### 2026-05-21 — Glamour style and theme: driven by config

The transcript reader's first cut hard-coded
`glamour.WithStandardStyle("dark")` because glamour v2 dropped
the `WithAutoStyle` helper. The resolved approach reads a new
`ui.tui.glamour_style` key from the chronicle TOML config,
defaulting to `dark`. The accepted values mirror glamour v2's
stylesheets (`ascii`, `dark`, `dracula`, `light`, `notty`,
`pink`, `tokyo-night`). The existing `ui.tui.theme` key, which
the session-1 wiring read into a config field but never
threaded into the runtime, is now plumbed the same way.

Both values are validated at the configuration boundary in
`cmd/chronicle/main.go`. An unknown value falls back to the
documented default with a full-sentence stderr warning that
quotes the value, lists the accepted set, and names the
fallback. The TUI internals trust the values once they cross
that boundary, which matches the project's "validate at the
boundary, trust internal callers" posture. The boundary helpers
(`tui.ParseTheme`, `tui.IsKnownGlamourStyle`, and the
list-formatting helpers the warnings use) all live in the `tui`
package so `main.go` reaches into exactly one tui-tree package.

### 2026-05-21 — Top-level navigation: horizontal tab strip

Phase 3 (stats) is the first screen not reached by drilling
into a session, so the TUI needed a way to move between
top-level sections. After surfacing three candidates (a top
tab strip, a command palette, and Tab-cycling) and a web
search for what experts recommend, the decision is a
**horizontal top tab strip**: a single header line that lists
the implemented sections, the active one painted in the theme
accent, with number keys to jump directly and Tab/Shift-Tab to
cycle. The transcript reader stays a drill-down from the
session list (Enter to open, Esc to return), not a peer tab.

The reasoning, informed by the research:

- The tabs UX guidance is that tabs suit a small fixed set of
  equally-important sections (roughly five or six). Chronicle
  has exactly five top-level sections, which is squarely in
  that range. A sidebar's headline advantage is scaling to
  many sections, which chronicle does not need, so a
  collapsing sidebar would be solving a problem we do not
  have.
- In a terminal, height is abundant and width is scarce.
  Chronicle's content is width-hungry tables (stats, doctor,
  trash). A top strip costs one row of height. A left sidebar
  costs roughly sixteen columns of width permanently, which
  the tables want. Spend the abundant resource, not the
  scarce one.
- A responsive collapsing sidebar is not a free lunch in
  Bubble Tea. The framework gives no layout engine — every
  widget is sized by hand from WindowSizeMsg, and lipgloss has
  known rough edges with width-filling and word-wrap on
  resize. Hand-rolling a collapse threshold and focus hand-off
  is real complexity and bug surface for a v1.
- A command palette is a many-targets tool (k9s uses it
  because Kubernetes has dozens of resource types). Five named
  screens do not justify hiding the whole surface behind a
  prompt.

**Scalability trigger:** if chronicle's top-level section count
ever grows past about six, revisit toward a command palette
(the many-targets winner), not a sidebar (which would still
cost the width the tables need). A `Ctrl-K`-style palette can
also be layered on top of the tab strip later as a power-user
accelerator without redesigning the navigation.

**Responsive design (a first-class requirement, at the user's
explicit request):**

- The tab strip is always exactly one line and never wraps,
  the same single-line invariant the session-list rows hold.
  A wrapping header would break the fixed header height the
  content area's sizing math depends on.
- Two render tiers. When the full labels fit the terminal
  width, the strip shows them ("sessions · stats", active
  accented, with number hints). When they do not, it degrades
  to a compact form — the section numbers with the active one
  accented plus the active section's name — so the strip
  always fits one line and always shows where the user is.
- The content area reserves fixed rows for the header (the
  strip plus a divider) and the footer (the help line), and
  gives the rest to the active screen. The height is clamped
  to at least one row so a tiny terminal still renders.
- Tables size to the available width through
  `lipgloss/v2/table`'s Width, and columns truncate rather
  than wrap so a narrow terminal still produces a single-line
  row per record.

**Internal structure:** `app.go` becomes a router. A `Screen`
interface (`Init`/`Update`/`View`) lets the app hold the
implemented sections in a section-keyed registry and route the
active one, rather than growing one field and one type-switch
branch per screen. This is the `Screen` interface session 1's
audit deferred until "more than two screens make the
type-switch ungainly" — phase 3 is that moment. Only built
sections register a tab, so the strip grows as phases 4 to 6
land rather than showing placeholder tabs for screens that do
not exist yet.

### 2026-05-21 — Screen architecture: ui.Frame is the one render rule

After two rounds of polish-on-top-of-polish the TUI still had
three screens with three different rendering rules, three
different loading states, and a help row that hid bindings
behind ellipsis truncation. The user's feedback was that the
fix-the-symptom pattern was producing drift, and the right
shape was to redesign the screen architecture so the
inconsistencies became structurally impossible. This is that
redesign.

`cmd/chronicle/tui/ui/frame.go` holds the one `Frame`
component every screen renders through. The frame is a
renderer (not a model): it takes the screen's width and
height, a state value (one of `ui.Loading`, `ui.Empty`,
`ui.Error`, `ui.Ready`), an optional muted status row, and
the screen-curated footer bindings, and returns the rendered
string. The body always sits at the top with blank rows
padded down so the footer stays anchored at the bottom of
the frame regardless of how short the body content is. Three
screens, one drawing function — visual drift is now
structurally impossible.

Research from k9s, lazygit, and OpenCode informed the footer
design. The convention every serious TUI converges on is a
short, single-line, context-sensitive footer with the
bindings most useful on the current view, plus a `?` overlay
listing every binding grouped by purpose. Chronicle now
follows the pattern exactly. The footer shows two or three
screen-curated bindings on a single line that never wraps
and never truncates. Pressing `?` opens a full-help modal
through the bubbles help component's `FullHelpView`. The
single-line invariant keeps the body height stable across
resizes — a wrap-to-second-line footer that I considered
first would have shifted the body when the binding count
crossed a threshold, which reads as a layout bug.

Global keys move to the app model so every screen treats
them identically:

- **Esc** is the back-then-quit ladder every serious TUI
  follows. It closes the transcript overlay when one is
  open, lets the session list's filter clear itself when
  capturing input, and otherwise quits the program. Users
  do not have to learn what Esc means per screen.
- **`?`** opens the full-help modal. While the overlay is
  open it is modal: only Esc, `?`, and q reach the app, so
  mashing a number key behind the overlay does not switch
  sections.
- **r** is global refresh. Each refreshable screen exposes
  a `Refresh()` method; the app dispatches the key to the
  active section. Every refresh flows through one path.
- **q** and **ctrl+c** quit, guarded against the session
  list's filter mode so typing the letter does not exit.

The sessions screen drops the bubbles list's
`SetShowHelp(true)` and `SetShowStatusBar(true)`, because
the frame draws both. The session count moves to the
frame's status row above the footer; every screen that
wants a status row gets the same shape. The list naturally
fills the frame's body region (its `View()` already pads to
the height it was given), so the footer-hugging-content
issue disappears without a special case.

**Rule for every later screen:** screens own content, not
chrome. A screen that needs to render a body composes
through the frame; it does not redraw the help row, the
status row, or the loading indicator. If a screen needs a
piece of presentation the frame does not offer, the right
move is to extend the frame (so every screen benefits), not
to hand-roll an inline equivalent. Drift between screens is
the bug that frame design exists to prevent.

### 2026-05-21 — Phase 3 polish pass: consistent chrome, lazy loads, footer anchoring

After phase 3 the user inspected the running TUI on real data
and flagged several issues: the help row at the bottom of the
stats screen overflowed and clipped the trailing entry to
"1-5 s", the loading message was a motionless "Computing the
summary…" with no sign of progress, the help-row style
differed between the session list and the new screens, the
footer snapped up beside the spinner during loading rather
than staying at the bottom of the screen, switching to the
stats section appeared to freeze the program, and an "ago"
reading on Copilot rows showed the value as "106751d ago".

The session-list footer is the **reference design**, not the
shape to replace. The bubbles list's built-in status bar
plus its built-in help row is what every screen should look
like. The first cut of this polish pass added a custom
`ui.HelpBar` and made the session list use it too, which
broke the reference look. The fix is the other direction:
keep the session list as-is and bring the transcript and
stats screens into the same shape by using the bubbles
`help.Model` component directly with `SetWidth` so its
built-in truncation handles the overflow. The custom
`ui.HelpBar` is gone.

A `ui.Spinner` component now renders the loading row with the
bubbles spinner glyph plus a live elapsed-time counter. The
row reads "Loading transcript… (1.2 s)" and updates with
every tick. The format switches to "(Xm YYs)" past a minute
so a chronically slow load stays under ten characters. Each
loading-capable screen holds a Spinner alongside its status
flag and batches the spinner's tick command into Init and
into Refresh.

The footer is now anchored to the bottom of the screen
through `lipgloss.PlaceVertical`. During loading or error
states the spinner sits at the top of the body region and
blank rows fill the space down to the footer, so the help
line stays where the user expects it to be rather than
floating up beside a one-line message.

`composition.HumanAge` now returns the literal "unknown" when
its input is the zero value of `time.Time`. The unguarded
version was rendering missing timestamps as the days since
Go's zero time (the "106751d ago" the user reported). The
zero value arrives at the function whenever an adapter could
not pull a timestamp from its source data, which on real
Copilot Chat sessions is common when `lastMessageDate` is
absent from the snapshot.

The biggest finding from the live test was a real
performance problem. `composition.App.Stats` on the user's
real data takes 14.8 seconds (5.5 s Claude, 9 s Copilot
Chat). The cause is each adapter's `ListSessions` parsing
every session file to fill `SessionSummary.LastActive`,
`TurnCount`, and the other derived fields. The CLI command
inherits this cost too. The contract claimed "bounded by
listing cost rather than parse cost", but the implementation
has been parsing all along. Phase 3 made the TUI worse than
the CLI here by batching every section's Init at startup —
even sessions the user never visits — so chronicle paid the
stats cost before its first frame rendered.

The immediate fix is lazy section initialisation: `app.go`
only inits the active section at startup and inits other
sections the first time the user activates them. Startup is
instant again, and the stats wait happens only when the user
asks for stats. The deeper fix — making `ListSessions` not
parse every file — is recorded as an open question for a
future session, because it touches all three adapters and
the composition contract semantics.

**Principle for every later screen:** the bubbles list's
built-in chrome (status bar plus help row) is the reference
shape. Screens that do not embed a list (the transcript
reader, the stats screen, and the doctor/trash/memory
screens in later phases) use a `help.Model` directly with
`SetWidth` so the rendered line is identical. The
`ui.Spinner` is the canonical loading indicator and lives
behind `lipgloss.PlaceVertical` so the footer stays at the
bottom.

### 2026-05-21 — Transcript SoftWrap: off (resolves scroll latency)

The user reported scroll on the transcript reader felt
queued — keypresses piled up and the viewport moved by
itself after the user stopped pressing. The earlier
hypothesis was the spinner's tick command continuing past
the load, but the profile told a different story. A
benchmark of `View` on a one-megabyte rendered body showed
11.8 milliseconds per frame, 70% of the 16-millisecond
60-fps budget; under that load the runtime drops frames and
keypresses queue.

Ninety percent of the View cost was in the bubbles
viewport's `calculateLine`, which iterates every line of
the content (calling `ansi.StringWidth` per line) when
`SoftWrap = true`. For a 15,000-line body that is 15,000
grapheme-cluster walks per frame.

The transcript does not need SoftWrap. The glamour renderer
already word-wraps the Markdown to wrapWidth during
`renderMarkdown`, so every line the viewport receives is
already at most wrapWidth wide. Setting `SoftWrap = false`
drops the per-frame cost from 11.8 ms to 0.29 ms — a 40x
improvement that puts every render well inside the
60-fps budget.

`TestView_StaysWellUnderTheFrameBudget` pins the property:
View on a 1 MB body must complete in under five
milliseconds under the race detector, which leaves room for
slower CI machines while still catching the eleven-
millisecond regression the user reported. The supporting
benchmarks (`BenchmarkView_RealWorldTranscript`,
`BenchmarkView_HugeRenderedBody`) give the next session a
measurement to start from rather than a guess.

### 2026-05-21 — Codebase-wide review after phase 2

After phase 2 the user asked for a full review of the whole
codebase, not just the session's diff. Six `code-reviewer`
subagents covered the tree by layer (leaves, the three
adapters, composition, the CLI), each reporting findings keyed
by `path:line`. Every finding was verified against the real
code before any fix landed — four higher-severity claims did
not survive verification and were rejected with reasons (an
"orphaned tool-start dropped" claim that the assistant-message
event already covers, an "unknown user parts dropped" claim
that the inline-surfacing path already handles, and two
overstated severities).

The verified findings produced nine small commits. Two were
real correctness fixes: `composition/fsmove.go` now falls back
to copy-and-remove only on a cross-device (`EXDEV`) rename
error rather than on any failure, which previously could merge
a directory tree into an existing destination and then delete
the source, and the Claude adapter now advertises
`ModelMetadata` truthfully (it reads the per-record model and
reports the most-frequent value, so the old hard-coded `false`
was wrong). The rest removed genuine hacks and dead code — a
blank-identifier import hack in `search.go`, the dead
`ErrNoGlobalConfigCapability` sentinel, the now-unreachable
`(untitled)` and `truncateTitle` fallbacks — and unified the
CLI's stdout/stderr handling behind injectable writers so the
apply-path output is testable. The `Detect`-before-read
precondition the adapters rely on is now explicit on the
`contracts.Provider` interface.

## Open questions

The handoff's "Open questions" table is resolved under
"Decisions" above. New questions land here as the build surfaces
them, and each one moves to "Decisions" with a dated entry once
it is answered.

*(scroll latency is resolved — see "Decisions →
Transcript SoftWrap" below)*

### Stats walks every session file (open, latent perf hazard)

`composition.Stats` takes around fifteen seconds on a real
install (Claude ~5.5 s, Copilot Chat ~9 s, copilot-agent
sub-second) because each adapter's `ListSessions` parses
every session file to fill the derived `SessionSummary`
fields. The contract is documented as "bounded by listing
cost rather than parse cost", which the implementation has
been quietly contradicting since the listings were first
written.

The TUI hides the latency on first launch through lazy
section initialisation (the stats screen does not load
until the user activates it), so the immediate user
experience is acceptable. The fix is bigger — either
`ListSessions` reads only enough of each file to fill the
summary fields (file size from `os.Stat`, timestamps from
the first record, model from a shallow scan), or a
session-summary cache lives next to the trash manifests.
The decision touches all three adapters and the
composition contract semantics, so it is recorded here as
the next round of composition work rather than slipped
into a TUI phase.

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

### 2026-05-21 — Session 2 (finish phase 2 + codebase review)

- Read the session-2 handoff, the original brief's "How to
  write/code/work" sections, `SKILL_PROMPT.md`, the build
  audit, and the three first-time-reader docs
  (`codebase-tour`, `feature-roadmap`, `provider-surface`).
  Ran `make check` against a clean tree — green.
- Finished phase 2 (the transcript reader):
  - Wrote the transcript package's unit tests following the
    `sessions_test.go` pattern. A `fakeReader` and a
    `newTestModel` helper drive the Model through loading,
    ready, error, the BackMsg emission on Esc, and a
    WindowSizeMsg re-render at a new wrap width.
  - Fixed the stale `app_test.go` test that still claimed the
    OpenRequestMsg branch published a status message. The
    behaviour is now a screen switch, so the test was renamed
    and re-pointed to assert the active screen flips to the
    transcript.
- Resolved the two phase-2 user observations through the
  `ListingTitle` and glamour-config decisions recorded above.
- Drove the binary through `expect` for a launch/navigate/quit
  smoke test (exit 0). The alt-screen frames are not
  capturable from outside the terminal without `tmux`, which
  is not installed, so the visual review depends on the user
  running `./chronicle`.
- On the user's request, ran a full codebase review with six
  `code-reviewer` subagents partitioned by layer, verified
  every finding against the code, rejected four overstated
  claims with reasons, and landed nine small fixes. See the
  "Codebase-wide review after phase 2" decision above for the
  detail. The two correctness fixes were the `fsmove` EXDEV
  gate and the Claude `ModelMetadata` flag.
- Seventeen commits total this session, each with `make check`
  green. Phase pointer moved to phase 3 (stats).

### 2026-05-21 — Session 2 continued (phase 3: stats)

- Researched what to ship for top-level navigation. A web
  search confirmed the rule of thumb (tabs suit up to five or
  six sections, sidebars are for larger surfaces) and surfaced
  the responsive caveat — Bubble Tea has no layout engine, so
  a collapsing sidebar would be real complexity. The user
  asked for the more modern, scalable, accessible choice; the
  decision landed on a one-line horizontal tab strip with a
  documented scalability trigger toward a command palette
  past about six sections. Recorded under "Decisions →
  2026-05-21 — Top-level navigation: horizontal tab strip".
- Built the navigation foundation. A new Screen interface in
  `cmd/chronicle/tui/screen.go` (Init/Update/View) lives in
  the tui package alongside thin adapters
  (sessionsScreen, statsScreen) that wrap each concrete
  value-type Model into the interface, so the screen
  packages stay free of any dependency on the routing layer.
  The app model gained an order/meta/screens registry and
  routes number keys (1, 2, …) directly to the section in
  that position plus Tab and Shift-Tab to cycle.
- Responsive layout was a first-class requirement. The tab
  strip has two render tiers — a full-label tier with every
  section label, and a compact tier that shows the brand,
  the active label, and the section numbers — so the strip
  always fits one line regardless of terminal width. The
  chrome is exactly two rows (strip plus divider), forwarded
  to every screen as a height reduction so the content area's
  sizing math stays stable.
- Built the stats screen at
  `cmd/chronicle/tui/screens/stats/`. The Source interface
  is the minimal `composition.App` subset (just the Stats
  method). The body renders a totals block followed by the
  per-provider, top-projects, and by-model tables through
  `lipgloss/v2/table`, all inside a viewport that scrolls
  with the same j/k/u/d/g/G keys the transcript reader uses.
  Tables size themselves to the terminal width through the
  table's Width setter so a narrow window truncates cells
  rather than wrapping a row.
- Six unit tests pin the stats screen's contract: loading,
  ready, error, the load command's loadedMsg shape, every
  section header in the rendered output (with "(unknown)"
  for the empty-model bucket), and the same summary
  producing different output at two widths so a width-budget
  regression is caught. The session list's own breadcrumb
  was removed because the app's tab strip is the one source
  of top chrome; the three app-level tests were updated for
  the new section model.
- Live-tested via `expect`: launched the binary, jumped to
  stats with `2`, returned to sessions with `1`, cycled with
  Tab, quit with `q`. Exit status 0, every step accepted.
  The visual review of the rendered tab strip and tables
  depends on the user running `./chronicle` against their
  real data.
- Three commits this phase, each with `make check` green.
  Phase pointer moved to phase 4 (doctor).

### 2026-05-21 — Session 2 continued (phase 3 polish + design pass)

- The user reviewed the running TUI on real data and
  flagged a cluster of issues: the help row overflowed and
  clipped to "1-5 s", the loading state showed a
  motionless message with no progress signal, the session
  list still used the old plain-text loader while
  transcript and stats had a spinner, the footer position
  jittered against the spinner during loading, "ago"
  readings on Copilot rows rendered as "106751d ago",
  separators drifted between the tab strip and the
  bubbles help row, and the chevrons in the transcript
  breadcrumb rendered awkwardly small. The fixes came in
  three waves rather than one because the early rounds
  produced new drift the user had to flag.
- First wave (commit 3bd5c10) was a focused fix for the
  Copilot timestamp regression — current VS Code does not
  write `lastMessageDate`, so the adapter now derives
  `EndedAt` from the latest request timestamp inside the
  snapshot — plus unifying separators on `theme.Separator`
  and `theme.HierarchySeparator` and dropping the
  duplicated transcript breadcrumb.
- Second wave (commit 6832f84) was the architecture
  redesign. Web search confirmed every serious TUI (k9s,
  lazygit, OpenCode) uses a short context-sensitive
  footer plus a `?` overlay; chronicle now does the same.
  `ui.Frame` is the one render rule every screen composes
  through. Three screens reduced to: a model, a `state()`
  method, a `footerBindings` slice, and a `Refresh`
  method. Global keys (Esc, ?, r, q) moved to the app
  model so every screen treats them identically.
- The first cut of the frame had real bugs the user
  surfaced visually: a frame-level status row produced a
  height-accounting bug that pushed the footer off-screen,
  the Ready body did not pad to the body region so the
  footer rode up against short content, the universal `?`
  anchor was missing from the footer (the screens were
  curating it out), the spinner glyph rendered awkwardly,
  and the body and footer ran together with no divider.
  Third wave (commit 5d6d8e8) addressed all of them:
  removed the frame status row (the bubbles list's own
  `SetShowStatusBar(true)` handles the row count), wrap
  every body state in `lipgloss.Place` for defensive
  padding, append `?` and `q` to every footer regardless
  of what the screen passed, switch the spinner from the
  braille Dot to the ASCII Line glyph, and draw a muted
  divider above the footer.
- `cmd/chronicle/tui/ui/frame_test.go` captures the
  rendered output and asserts the load-bearing
  properties: dimensions match the budget, the footer
  carries `?` and `q`, the loading row is centred, the
  error body quotes the error, the divider sits between
  body and footer. The new dimension-fit test caught the
  Ready-body-too-short bug on first run, which is the
  evidence the tests are now worth their weight. The next
  visual regression has a test to catch it before it
  reaches the user's screen.
- Four commits in this round, each with `make check`
  green. Two open questions remain (scroll latency,
  Stats walks every file) and are recorded under "Open
  questions" above. The audit is the source of truth
  going forward — the handoff prompt is no longer needed.

### 2026-05-21 — Session 2 continued (scroll-latency investigation)

- Investigated the scroll-latency report empirically
  rather than by guess. A direct-Update test proved the
  screen's Update path advances the viewport one position
  per keypress with no delay; the app's keypress handler
  is pure logic with no I/O; the bubble tea renderer
  runs at 60 fps by default. None of those were the
  bottleneck.
- A CPU profile of `View` on a one-megabyte rendered
  body showed 11.8 milliseconds per frame, 90% of which
  was in the bubbles viewport's `calculateLine` calling
  `ansi.StringWidth` on every line of the content
  because `SoftWrap = true`. For a 15,000-line body that
  is 15,000 grapheme-cluster walks per frame, every frame.
- The fix is one line: `SoftWrap = false`. The glamour
  renderer already word-wraps the Markdown during
  `renderMarkdown`, so the viewport never sees lines
  wider than the terminal — SoftWrap was redoing the
  same work for no benefit. Cost dropped 40x, from
  11.8 ms to 0.29 ms per frame. Two benchmarks and a
  property test
  (`TestView_StaysWellUnderTheFrameBudget`) lock the
  performance ceiling so a future regression fails the
  test before it reaches the user. One commit, green.

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

This file is the source of truth for the TUI build. The
handoff prompt at `docs/tui-and-web-handoff.md` is the
original brief and is still load-bearing for the
project-wide writing and engineering rules (read its
"How to write", "How to code", and "How to work" sections),
but the day-to-day state of the build lives here. Update
this file every time you change the state — when a phase
transitions, when a decision is made, when an open
question surfaces, when a session ends.

If you are a new session reading this for the first time:

1. Read this file end to end.
2. Read `SKILL_PROMPT.md` sections 1, 3, 4 in full — they
   are the engineering and prose rulebook every commit
   follows.
3. Skim `docs/tui-and-web-handoff.md` for the project
   voice; skim `docs/codebase-tour.md` and
   `docs/feature-roadmap.md` for the layer-by-layer
   walkthrough and what is left to build.
4. Run `make check`. If anything is red, that is your
   first task.
5. Look at the "Current phase" section above. Resume from
   there. Check "Open questions" — anything listed there
   is blocking forward motion and is the right next
   thing.
6. Add a new dated heading under "Session log" before you
   do anything that changes the state. The audit is the
   primer the session after yours reads first.
