# TUI library survey

Goal: production-grade, modern-looking TUI. Single-binary distribution preferred. macOS first, Linux a close second, Windows nice-to-have.

## Stacks evaluated

### Rust — **ratatui**

- Active fork of the abandoned `tui-rs`, ~10k stars, monthly releases. Pair with `crossterm` (backend), `arboard` (clipboard), `syntect` (syntax highlighting), `pulldown-cmark` (markdown parsing).
- Real apps: **gitui**, **yazi**, **atuin**, **bottom**, **television**, **gpg-tui**.
- Design ceiling: very high (yazi looks genuinely modern).
- Markdown: no first-class lib — shell out to `mdcat` or roll your own with `pulldown-cmark` + ANSI. **Plumbing required.**
- Distribution: single static binary. Gold standard.
- Performance: excellent. Immediate-mode rendering means you redraw every frame and manage state yourself.
- Learning curve: **steep**. No reactive model out of the box.

### Go — **Bubble Tea + Lip Gloss + Bubbles + Glamour** (Charm stack)

- Bubble Tea = Elm-architecture event loop. Lip Gloss = styling (flexbox-ish). Bubbles = ready-made components (list, viewport, textinput, paginator, spinner, table). Glamour = Markdown → ANSI, the **best terminal markdown renderer available**, uses Chroma for ~200-language syntax highlighting.
- Real apps: **gh dash**, **glow**, **soft-serve**, **gum**, **freeze**, **superfile**, **lazysql**, **opencode**, **claude-squad**.
- Design ceiling: **highest of any ecosystem.** The Charm team are designers first — their apps set the visual bar.
- Distribution: single static binary via `go build`. macOS arm64/amd64, Linux, Windows trivially.
- Performance: very good. Viewport virtualization handles huge lists.
- Learning curve: moderate. Elm architecture clicks fast.
- OSC52 clipboard, mouse, keyboard all first-class.

### Python — **Textual** (+ Rich)

- CSS-based styling, reactive, async, animations, even runs in a browser via `textual-web`.
- Real apps: **harlequin** (SQL IDE), **posting** (HTTP client), **dolphie**, **memray** TUI, **toolong**.
- Design ceiling: very high, arguably matches Charm.
- Markdown: Rich handles it well; Textual has a `Markdown` widget.
- Distribution: **the weak spot.** `pipx`/`uv tool install` works fine in 2026, but no true single binary. PyInstaller/Nuitka produce 30-80 MB bundles with cold-start latency.
- Performance: good, occasionally janky on huge updates.

### TypeScript/Node — **Ink**

- React for terminals. **Claude Code, Gemini CLI, GitHub Copilot CLI** are all Ink apps.
- Real apps: above + Prisma CLI, Gatsby CLI.
- Design ceiling: high, but constrained by terminal text-flow rendering — visible flicker on rapid updates vs Rust/Go.
- Markdown: `marked-terminal` / `cli-markdown` — mediocre vs Glamour.
- Distribution: `npm install -g` (requires Node) or `bun build --compile` / `vercel/pkg` for ~50 MB single binaries.
- Performance: **slowest of the four.** GC pauses visible in dense lists.

### Avoid

- `tview` / `gocui` (Go) — dated visuals.
- `cursive` (Rust) — retro look.
- `blessed` / `neo-blessed` / `terminal-kit` (Node) — dated.
- `prompt-toolkit` for full apps (great for REPLs, not pages).

## Cross-cutting concerns

| Concern | Best option per language |
|---|---|
| Markdown → terminal | Glamour (Go) > Rich / Textual (Python) > `mdcat` (Rust standalone) > `marked-terminal` (Node) |
| Syntax highlighting | Chroma (Go), syntect (Rust), Pygments (Python), highlight.js (Node) |
| Clipboard + OSC52 (works over SSH) | Go: `golang.design/x/clipboard` + `aymanbagabas/go-osc52` · Rust: `arboard` + osc52 crate · Python: `pyperclip` + manual escape · Node: `clipboardy` + manual escape |

OSC52 is trivial: emit `\x1b]52;c;<base64>\x07`. Works in every modern terminal.

## Recommendation

| Stack | Verdict |
|---|---|
| **Go + Bubble Tea / Lip Gloss / Bubbles / Glamour** | **Recommended.** Best design ceiling, single binary, batteries-included Markdown + clipboard, mature. Used by `claude-squad` already, so there's precedent in the Claude-tooling space. |
| Rust + ratatui | Strong second. Pick if performance or a Rust-first team matters. More plumbing for Markdown. |
| Python + Textual | Pick only if the team is Python-first. Distribution remains the weak spot. |
| Node + Ink | Pick only to share React components with a web app. Heaviest runtime, weakest Markdown. |

Reasoning: Glamour alone is worth choosing Go over Rust — it renders Claude transcripts (markdown with fenced code) beautifully out of the box. Re-implementing that quality in Rust would be a multi-week side quest. Single-binary `go build` matches the user's "works like Claude Code" install expectation.
