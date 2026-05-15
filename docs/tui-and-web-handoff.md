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

Three adapters ship today. The `claude` adapter reads
`~/.claude/projects/<encoded-cwd>/<sessionId>.jsonl`. The
`copilotchat` adapter reads VS Code's `workspaceStorage/<hash>/
chatSessions/` and the parallel `globalStorage/
emptyWindowChatSessions/`. The `copilotagent` adapter reads
`~/.copilot/session-state/<sessionId>/events.jsonl`, which is what
the `@github/copilot-sdk` runtime writes. The two Copilot adapters
are distinct because the data has zero overlap on disk and the two
products are different. Do not collapse them.

The composition layer exposes a small, stable API. The methods you
will rely on are `Listings`, `Search`, `Stats` (with `StatsOptions`
including the by-model breakdown), `Doctor`, `ReadSession`,
`Resume`, `PlanCleanup`, `ExecuteCleanup`, `TrashList`,
`TrashRestore`, `TrashEmpty`, the memory and global-config helpers,
and the bulk-export path. Read the method signatures and
documentation in `composition/` before touching anything. None of
those need to change for either presentation layer. If you find
yourself wanting to change them, surface that thought to the user
before you reach for the editor.

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

- Write **complete sentences** with explicit subjects, verbs, and
  articles. "Returns the value" is wrong for prose. "The function
  returns the value" or "Returns the resolved value when the
  consensus rule produced one" is right. PEP-257-style imperative
  one-liners are fine inside docstrings, because the actor there
  is the function itself.
- No semicolons in prose. Use periods. Long sentences break into
  two. The ban applies to docstrings, comments, error messages,
  Markdown docs, commit messages, and any chat reply. It does not
  apply to Go code, SQL, or other languages where the semicolon
  is syntax.
- Em-dashes are louder than parentheses. Parentheses mark a side
  note the reader can skip. Em-dashes mark an interruption the
  reader must register. Write `—` in prose, never `--`. Three
  em-dashes in one paragraph means the paragraph is doing too
  much. Split it.
- No AI-flavoured filler. Cut "It's worth noting that", "Let me
  walk you through", "In summary", and the rest. Start with the
  substance.
- Spell out referents. When you mention a variable, a column, an
  endpoint, or a file path, give the reader enough context to know
  what the thing is. Quote it with backticks and describe its role
  in the sentence.
- Error messages are user interface. Include the value that caused
  the problem, the operation that failed, and the next step the
  reader can take. "Invalid input" is unacceptable. "The value
  `'2026-13-45'` passed to `--due-by` is not an ISO-format date.
  The parser reported `month must be in 1..12`" is the bar.

The codebase's existing comments are your reference. When in
doubt, open `contracts/conversation.go` or
`composition/stats.go` and read how the working code talks. Match
that voice. Do not invent a new one.

## How to code

- The dependency graph is sacred. Imports flow downhill only. A
  presentation layer never imports an adapter directly. It goes
  through `composition.App`. If you ever find yourself reaching
  for an adapter package from `cmd/chronicle/tui/`, stop and
  rethink.
- Optional capabilities live behind type assertions. The base
  `contracts.Provider` interface is small. Cleanup, memory,
  resume, and global config are optional capabilities each
  adapter opts into. The presentation layers discover those
  capabilities the same way the CLI does: through `composition`,
  not by reaching into adapters.
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

Act like a principal engineer with a small team behind them. The
user does not want shortcuts. The user does not want
brittle hot-fixes. The user does not want a refactor that solves
the symptom without identifying the cause. The user wants stable,
elegant, clean, consistent, as-perfect-as-possible work, and is
willing to spend time on multiple review passes to get there.

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

A terminal UI for browsing sessions, reading transcripts, and
running the cleanup flows the CLI already supports. The user opens
chronicle without arguments and lands in the TUI by default. The
TUI is the friendly face of chronicle for everyday use. The CLI
remains the scripting face.

The TUI's job is presentation. Every screen reads from the
composition API. Every action the user takes (delete a session,
restore from trash, export to Markdown, resume) calls a
composition method that already exists. The TUI does not invent a
new domain model, it surfaces the existing one.

Recommended approach: build it on
[Bubble Tea](https://github.com/charmbracelet/bubbletea) with
[lipgloss](https://github.com/charmbracelet/lipgloss) for styling
and [bubbles](https://github.com/charmbracelet/bubbles) for the
ready-made components (list, viewport, textinput, table). These
are the de-facto-standard Go TUI libraries in 2026 and the
community has built dozens of high-quality apps on them. Read at
least one well-regarded Bubble Tea app end to end before you write
your first screen. `glow` and `gh-dash` are good references.

Open questions to resolve with the user **before** building:

- Should the TUI be the default behaviour of `chronicle` with no
  arguments, or a separate subcommand like `chronicle tui`. The
  feature-roadmap leans toward "default". Confirm before you
  commit.
- The screen list — a session list, a transcript reader, a stats
  view, a doctor view, a trash view, a memory view. The first
  question is what set of screens ships in v1.
- Keyboard model. The user already lives in Vim. Pick a binding
  set that respects that without becoming inaccessible to users
  who do not.
- Theme. One default, or follow the terminal's palette. Be honest
  about how much of this is worth doing in v1.

### 2. The web frontend

After the TUI is stable, build a web app for sharing rendered
transcripts and browsing history in a browser. The web app's main
job is the same as the TUI's: it surfaces composition's existing
data through a friendlier interface. The web app does not duplicate
the cleanup or memory-edit flows in v1, because the destructive
surface is harder to make safe over HTTP.

Recommended approach: a small Go HTTP server in `cmd/chronicle-web/`,
templ for templates, HTMX for interactivity, and Tailwind for
styling. The stack is well-understood in 2026 and matches the
project's preference for boring, server-rendered tools that age
well. Avoid a single-page-app architecture and a separate frontend
build, because chronicle is a single-binary tool and the web app
should not break that property.

Open questions to resolve before building:

- Auth model. The simplest answer is "the server only binds to
  127.0.0.1 and the user is whoever is on the local machine".
  Anything more than that is a real design conversation.
- Sharing. Is the share scope "anyone who has the URL" or
  "anyone authenticated against my GitHub". The simplest answer
  is "the share is a local URL the user copies into Slack or
  email, and the recipient hits the same machine over Tailscale
  or ngrok". Confirm before you build.
- What renders. Probably the same five views as the TUI, plus a
  permalink for one rendered Markdown transcript.

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
