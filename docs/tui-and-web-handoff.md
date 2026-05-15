# Handoff prompt: TUI and web frontend for chronicle

You are stepping into a Go project called **chronicle**. It reads
the on-disk history of AI coding assistants (Claude Code, the two
GitHub Copilot products), renders sessions as Markdown, and helps
the user prune the resulting data on disk. The CLI surface is
stable. The next two layers are a terminal UI and a web app, in
that order. This document is the brief.

## What you are inheriting

The codebase is hexagonal. The dependency graph runs one way only,
from leaf to root: `contracts/` (pure types) → `adapters/` (one
package per upstream tool) → `steps/` (pure transforms) →
`composition/` (the application core, the only layer that touches
disk) → `cmd/chronicle/` (the CLI binary). Every new surface you
add belongs at the entrypoint layer, on top of `composition.App`,
not inside the layers below it.

Three adapters ship today.

| Adapter | Reads from | Storage shape |
|---|---|---|
| `claude` | `~/.claude/projects/<encoded-cwd>/<sessionId>.jsonl` | The file is one JSONL record per turn, with each record carrying the role, the content blocks, and the metadata that describes them. |
| `copilotchat` | `workspaceStorage/<hash>/chatSessions/` and `globalStorage/emptyWindowChatSessions/` under the VS Code user directory | The file is an event log. The first line is a full snapshot of the session, and every later line is a small patch that mutates the snapshot in place. |
| `copilotagent` | `~/.copilot/session-state/<sessionId>/events.jsonl` | Each line is a typed event envelope written by the `@github/copilot-sdk` runtime, with a `type` field and a `data` payload whose shape depends on the type. |

The two Copilot adapters are distinct because the data has zero
overlap on disk and the two products are different. Do not collapse
them.

The composition layer exposes a small, stable API. The methods you
will rely on:

| Method | What you use it for |
|---|---|
| `Listings` | This is the master session list. It walks every provider, collects one `SessionListing` per session, and sorts the results so the most recently active session is at the top. |
| `Search` | This finds sessions whose content matches a substring. The result carries the matching sessions together with the snippets that surround each match. Pass `SearchOptions.Provider` to narrow the search to one adapter. |
| `Stats` | This is the one-screen summary the `chronicle stats` command renders. The result has the totals, the per-provider rows, the top-N projects, and the by-model breakdown. |
| `Doctor` | This is the per-provider health report. Each `ProviderHealth` value carries the root directory, the detected version, the fingerprint, whether the root was reachable, the session count, and any warnings the adapter produced along the way. |
| `ReadSession` | This loads one full conversation by id. The export, copy, and resume paths all start here. |
| `Resume` | This tells the caller how to relaunch a session in its original tool. The result has the argv to run and the working directory to run it from. |
| `PlanCleanup`, `ExecuteCleanup` | `PlanCleanup` walks the providers and builds the dry-run plan for one or more cleanup categories without touching disk. `ExecuteCleanup` takes that plan and actually moves the items into the trash. The CLI's `--apply` flag is what gates the second call. |
| `TrashList`, `TrashRestore`, `TrashEmpty` | These three drive the trash subsystem. `TrashList` reads the manifests and returns one entry per trashed item. `TrashRestore` puts an entry back at its original location. `TrashEmpty` purges entries older than the retention window from the user's config. |
| `ListMemories`, `ReadMemory`, `EditMemoryPath`, `CleanProjectMemory` | These four cover the per-project memory surface. Only the Claude adapter implements it today, so a call against a Copilot-only install comes back empty rather than erroring. |
| `ListGlobalMemory`, `CleanGlobalMemory` | These two cover the user-global memory surface, which Claude exposes through `~/.claude/CLAUDE.md`. The Copilot adapters do not have anything equivalent yet. |
| `ListConfigProjectEntries`, `CleanConfigProjects` | These two find stale entries inside Claude's global config file (`~/.claude.json`) and remove the ones whose project directory has gone. |
| `BulkExport` | This writes one Markdown transcript per session inside a project, all in one call, to a directory the caller chooses. |

Read the method signatures and documentation in `composition/`
before touching anything. None of those need to change for either
presentation layer. If you find yourself wanting to change them,
surface that thought to the user before you reach for the editor.

The TUI and web frontend are presentation layers. They consume the
composition API, they do not modify it. The only legitimate reason
to add a new method on `composition.App` is a piece of read-only
data the existing methods do not expose. Even then, prefer adding
fields to existing return values over inventing a new method.

## Read these before you write any code

1. `README.md` is the user-facing pitch and the command index. Read
   it to know what the CLI offers today.
2. `docs/codebase-tour.md` is the file-by-file walkthrough. Read
   it end to end. The repository layout, the dependency graph, and
   the contract surface are all explained there.
3. `docs/feature-roadmap.md` is the "what is left, in what order,
   why" document. The "Now next" section ends at the TUI and the
   web frontend. Read why those two are in that order. The author's
   reasoning matters because it tells you which trade-offs are
   already made and which are still yours to make.
4. `docs/provider-surface.md` is the cross-provider taxonomy. Read
   it to understand which capabilities every adapter does and does
   not implement today. Your UIs have to handle that asymmetry
   gracefully.
5. `docs/naming-conventions.md` and `docs/go-primer.md` are the
   shorter style references. Skim them before naming anything new.
6. `SKILL_PROMPT.md` is the writing and engineering rulebook the
   whole project follows. Read sections 1, 3, and 4 in full. The
   short version is below, but the long version is load-bearing.
7. `CHANGELOG.md` shows the recent shape of the project. The
   Unreleased section is what shipped in the most recent round of
   work.

## How to write

The codebase has a voice. Every reader who opens any file at any
time should hear the same voice. The rules that matter most:

- **Complete sentences with explicit subjects, verbs, and
  articles.** Prose that reads "Returns the value" is wrong
  because the actor is missing. The right shape spells the actor
  out, as in "The function returns the resolved value when the
  consensus rule produced one". PEP-257-style imperative
  one-liners are fine inside Go docstrings, because the actor
  there is the function itself.
- **No semicolons in prose.** Use periods. A long sentence with
  a semicolon nearly always reads better as two short sentences.
  The ban covers docstrings, comments, error messages, Markdown
  docs, commit messages, and any chat reply. It does not apply
  to code, where the semicolon is syntax.
- **Em-dashes are louder than parentheses.** Parentheses mark a
  side note the reader can skip without losing the sentence, and
  the voice steps offstage briefly. Em-dashes mark an
  interruption the reader must register, and the voice raises
  briefly. The test is to read the sentence aloud and notice
  which way the voice moves.
- **Use a real em-dash, never `--`, in prose.** The two-hyphen
  form is for shell examples. If three em-dashes appear in one
  paragraph, the paragraph is doing too much. Split it into two.
- **Cut AI-flavoured filler.** Phrases like "It's worth noting
  that", "Let me walk you through", and "In summary" add nothing
  and signal generated prose. Start with the substance instead.
- **Spell out the referents.** When the comment names a variable,
  a column, an endpoint, or a file path, quote the name with
  backticks and add a clause that describes its role in the
  sentence. The reader should not need a second screen open to
  understand the comment.

Error messages are part of the user interface, so every message
should include the value that caused the problem, the operation
that failed, and the next step the reader can take.

- Wrong: `Invalid input.`
- Right: `The value '2026-13-45' passed to --due-by is not an ISO-format date in the form YYYY-MM-DD. The underlying parser reported "month must be in 1..12".`

The codebase's existing comments are your reference. When in
doubt, open `contracts/conversation.go` or
`composition/stats.go` and read how the working code talks. Match
that voice. Do not invent a new one.

## How to code

- The dependency graph is strict, and you should treat it as a
  hard rule rather than a guideline. Imports flow downhill only.
  A presentation layer never imports an adapter directly, because
  every read and every action the presentation layer wants to
  perform already has a method on `composition.App`. If you find
  yourself reaching for an adapter package from inside
  `cmd/chronicle/tui/`, stop and look for the composition method
  you missed.
- The optional capabilities are reached through type assertions.
  The base `contracts.Provider` interface is small on purpose.
  Cleanup, memory, resume, and global config are each their own
  optional interface that an adapter opts into when it supports
  the surface. The presentation layers discover those capabilities
  the same way the CLI does, by asking composition rather than
  by reaching into the adapter packages.
- Errors are typed. Every adapter has its own `Error` value with
  `Op`, `Path`, and `Err` fields, and a constructor `newError`.
  Wrap at the public boundary. Return the unwrapped sentinel
  (`fs.ErrNotExist`) where the contract says you should, so
  callers can `errors.Is` against it.
- Multi-stage functions get numbered step comments. Look at
  `composition/trash.go::Trash` or
  `adapters/claude/parse.go::parseStream` for the shape. The
  numbered headers are part of the project's documentation
  discipline, not optional decoration.
- Tests pin user-visible contracts. Every new public surface gets
  a test that exercises the contract a downstream caller would
  rely on. The tests use `testing/fstest.MapFS` and small
  fixtures, not the real filesystem. Look at
  `composition/search_test.go` and
  `adapters/claude/parse_test.go` for the patterns.
- The build pipeline is `make check`, which runs `gofmt`,
  `go vet`, `golangci-lint`, and `go test -race ./...`. Run it
  before every commit, and never commit anything that fails any
  of those steps.

## How to work

Act like a principal engineer with years of experience, the kind
who could call on a few equally-senior peers when a second
opinion would help. The user does not want shortcuts. The user
does not want brittle hot-fixes. The user does not want a
refactor that papers over the symptom without finding the cause.
The user wants stable, elegant, clean, consistent,
as-perfect-as-possible work, and is willing to spend time on
multiple review passes to get there.

The shape of the work is **research, plan, execute, review,
repeat**.

1. **Research.** Before writing code, read what is already there.
   Read the relevant adapter, the relevant composition method,
   the relevant CLI subcommand. Read at least one similar project
   on the open web for context (Bubble Tea apps for the TUI, Go
   templ-or-HTMX style web apps for the frontend), but treat
   those as inspiration, not as templates to copy.
2. **Plan.** Surface the plan to the user before you implement.
   Name the files you will create, the methods you will add, the
   contracts you will rely on. Identify the trade-offs you are
   making. Ask for direction when two reasonable approaches differ
   only in taste.
3. **Execute.** Small, frequent commits. Each commit is one
   logically complete change with a message that explains the
   *why*, not the *what*. The diff is already the *what*. Run
   `make check` before every commit. Live-test the feature
   against the user's real data before declaring it done.
4. **Review.** After execution, run review passes with one lens
   at a time. The lenses the user expects you to run are:
   - **Writing pass.** Every prose comment, docstring, README,
     and doc. Match the project's voice. See the rules above.
   - **Code-smell pass.** Dead code, magic strings, missed step
     patterns, duplicated logic, suboptimal implementations.
   - **Consistency pass.** Do similar pieces of code look similar.
     Do test names follow one pattern. Do the new files match the
     existing files in shape.
   - **Bug and edge-case pass.** What happens on empty input. What
     happens on permission denial. What happens on a missing
     dependency. What happens when the user resizes the terminal.
     What happens on a slow disk.
   - **Cosmetic and UX pass.** Live-run every command and every
     screen, look for rough edges. Pluralisation, error message
     format, help-text wording, focus behaviour, keyboard
     accessibility.
   Each pass produces a list of findings. Each finding gets
   resolved — fixed, deferred with a written reason, or rejected
   with a written reason. Only after every pass produces zero new
   findings is the work done.

Do not declare the work finished after the first green build. A
green build is the floor, not the ceiling. The bar is "would a
senior reviewer at Stripe or Anthropic flag this."

## What you are building, in order

### 1. The TUI

The first piece is a terminal UI that lets the user browse
sessions, read transcripts, and run the cleanup flows the CLI
already supports. When the user runs `chronicle` with no
arguments, they should land in the TUI by default. The TUI
becomes the everyday face of chronicle for interactive use, and
the CLI keeps being the face for scripts.

The TUI's job is presentation, and only presentation. Every
screen reads from the composition API. Every action the user
takes from inside the TUI — deleting a session, restoring from
the trash, exporting to Markdown, resuming — calls a composition
method that already exists. The TUI does not invent a new domain
model. It surfaces the existing one in a more interactive shape.

Recommended stack:

| Library | Role |
|---|---|
| [Bubble Tea](https://github.com/charmbracelet/bubbletea) | This is the runtime that wraps every screen. It uses the Elm pattern, so every screen you write is a `tea.Model` value with its own `Init`, `Update`, and `View` methods. |
| [lipgloss](https://github.com/charmbracelet/lipgloss) | We use this for the styling. It gives us a clean way to set colours, borders, padding, and layout without scattering ANSI escape codes through the rendering code. |
| [bubbles](https://github.com/charmbracelet/bubbles) | This package contains the ready-made components we compose into each screen. The ones we will reach for first are the list, the viewport, the text input, the table, the spinner, and the help footer. |

These three libraries are the de facto standard for Go terminal
applications in 2026, and the community has built dozens of polished
apps on top of them. Before you write your first screen, read at
least one well-regarded Bubble Tea app from start to finish so you
have a working mental model of how the pieces fit together. The
`glow` Markdown reader and the `gh-dash` GitHub dashboard are both
good references.

Open questions to resolve **before** building:

| Question | Default leaning |
|---|---|
| Is the TUI the behaviour of `chronicle` with no arguments, or a separate `chronicle tui` subcommand? | The TUI should be the default behaviour when the user runs `chronicle` with no arguments. The feature-roadmap leans this way, but confirm before you commit to the change. |
| Which screens ship in v1? | The first cut is the session list, the transcript reader, the stats view, the doctor view, the trash view, and the memory view. Confirm the set before you build any of them. |
| Which keyboard model should the TUI use? | The default bindings should be Vim-style for the keys that have an obvious Vim equivalent, with arrow keys and Enter as fallbacks so users who do not live in Vim can still drive every screen. |
| Should the TUI ship with a theme system? | The default rendering should follow the terminal's own palette, and one opt-in dark theme is enough variety for v1. |

### 2. The web frontend

After the TUI is stable, build a web app for sharing rendered
transcripts and browsing history in a browser. The web app's main
job is the same as the TUI's: it surfaces composition's existing
data through a friendlier interface. The web app does not duplicate
the cleanup or memory-edit flows in v1, because the destructive
surface is harder to make safe over HTTP.

Recommended stack:

| Piece | Choice |
|---|---|
| Binary | We add a new `cmd/chronicle-web/` directory next to the existing `cmd/chronicle/`. Each binary does one thing, and the project has followed that rule from the start. |
| Templates | We use [templ](https://github.com/a-h/templ). It gives us type-checked Go templates that compile alongside the rest of the source, so the Go compiler catches a typo in a template the same way it catches a typo in regular code. |
| Interactivity | We use [HTMX](https://htmx.org/). The server renders small HTML fragments, the browser swaps them into the page, and we never need a separate JavaScript build to keep the UI lively. |
| Styling | We use Tailwind. The binary embeds a pre-built CSS file, so the user does not need Node or any other JavaScript toolchain on their machine to run the web app. |

Avoid a single-page-app architecture and a separate frontend
build. Chronicle is a single-binary tool, and the web app should
not break that property.

Open questions to resolve **before** building:

| Question | Default leaning |
|---|---|
| Which auth model should the server use? | The server should bind to `127.0.0.1` only and trust whoever is logged in to the local machine. Anything beyond that is a real design conversation, and the user is the one who decides how it goes. |
| What is the sharing scope? | A share is a local URL the user copies into Slack or email, and the recipient reaches the same machine over Tailscale, ngrok, or an SSH tunnel. The chronicle binary does not host any public surface itself. |
| Which views render? | The web app renders the same set of views as the TUI v1, plus a permalink route for one rendered Markdown transcript so the user can link a teammate to a single conversation. |

## Verification bar

Before you tell the user the work is done, verify these:

1. `make check` is green. `gofmt`, `go vet`, `golangci-lint`,
   `go test -race ./...` all pass.
2. The new screens or pages have unit tests for the pieces that
   carry logic, not just the rendering. Test the model that drives
   each Bubble Tea screen, the HTTP handler for each web route.
3. You have run the binary against the user's real data, not just
   in-memory fixtures. The slash-command title detection in
   `adapters/claude/parse.go` is an example of a fix the test
   suite missed and the live run caught. Plan for the same kind
   of finding.
4. The README and the feature roadmap reflect what shipped. The
   CHANGELOG has an entry under "Unreleased" that names the new
   surface.
5. The doc files you wrote read the same way as the doc files you
   inherited. Full sentences, no semicolons, no AI filler. Read
   them aloud in your head before committing. If a sentence sounds
   stripped or notes-to-self, rewrite it.

## When to stop and check in

Check in with the user when:

- Two reasonable approaches differ only in taste. Surface the
  trade-off, do not pick silently.
- A piece of work would force a change inside an adapter or in
  `contracts/`. Those are stable. A change there is a design
  conversation.
- You are about to take an action that is hard to reverse (force
  push, rewrite history, drop test fixtures, modify configuration
  outside the repo). Ask first.
- The user says something that contradicts an earlier instruction.
  Do not silently follow the latest. Surface the contradiction.

Otherwise, work continuously. The user does not want "should I
continue" prompts between tasks. They have asked for the work,
the next step is to do it, and a status update belongs at the end
of a logical chunk, not after every commit.

## One last thing

The user's writing reviews are unsparing. The user reads every
comment you write aloud in their head and will flag every sentence
that sounds compressed, academic, or stripped of subject. Default
to *more words*, not fewer, when a sentence is hard to follow. A
sentence that reads like a note to yourself is a sentence to
rewrite. Read every replacement aloud before you commit it.

That is the bar. Good luck.
