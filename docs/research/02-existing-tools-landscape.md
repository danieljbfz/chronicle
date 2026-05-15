# Existing tools landscape

What's already out there for managing Claude Code (and GitHub Copilot) history, and where the gap is.

> Source caveat: web search was unavailable during the research pass; the list below was synthesized from training-data knowledge through January 2026. Names should be verified before linking from anywhere user-facing.

## Claude Code

| Tool | Language | Form | Focus |
|---|---|---|---|
| **ccusage** (`ryoppippi/ccusage`) | TypeScript / npm | CLI, plain tables | Token usage and cost per session/day/project. Most popular by far. |
| **claude-trace** (`badlogic/claude-trace`) | Node | wrapper + HTML viewer | Logs HTTP traffic for debugging, not history browsing. |
| **claude-code-log** (`daaain/claude-code-log`) | Python | static HTML export | Converts JSONL → static HTML pages. Read-only. |
| **claudia** (`getAsterisk/claudia`) | Tauri (Rust + React) | Desktop GUI | Project/session browser, checkpoints, agent runner. Heavy. |
| **claude-squad** (`smtg-ai/claude-squad`) | Go (Bubble Tea) | TUI | Orchestrates multiple Claude Code sessions in tmux. Adjacent, not history. |
| **Specstory** (commercial) | VS Code extension | sidecar | Auto-archives sessions to markdown in-repo. Closed source. |
| Various small `claude-code-history` / `cchistory` forks | Python / Node | toy CLIs | `ls` sessions, pretty-print one. Hobbyist. |

## GitHub Copilot Chat

Storage locations:

- **VS Code**: `~/Library/Application Support/Code/User/workspaceStorage/<hash>/` and `.../globalStorage/github.copilot-chat/`. Session transcripts as JSON inside `chatSessions/` and `chatEditingSessions/`. Some metadata in VS Code's `state.vscdb` SQLite (table `ItemTable`).
- **JetBrains**: `~/Library/Caches/JetBrains/<IDE>/copilot/`.

Tools targeting it are scarce — `copilot-chat-export` (VS Code extension), a few one-off scripts on GitHub. No equivalent of ccusage exists. This is a clear secondary opportunity once the Claude side is solid.

## Where the gap is

- ccusage owns **cost analytics**. Don't reimplement.
- claudia owns **heavy desktop GUI**. Different audience.
- claude-code-log owns **static export**. Read-only; one-shot.
- **Nobody owns**: a fast, interactive **TUI** that browses sessions, previews threads, fuzzy-searches across all projects, filters tool noise, exports clean markdown, copies to clipboard, and *cleans up the orphaned sibling folders* (`file-history`, `tasks`, `paste-cache`, etc.) the rest of the tools ignore.

That's the lane.

## Reference UIs (for visual standards)

Tools whose look-and-feel we want to match or beat:

- **lazygit** — multi-pane layout, color-coded diffs, contextual key hints at the bottom.
- **k9s** — dense info, breadcrumb header, command palette (`:`), live updates.
- **yazi** — image previews via Kitty/iTerm graphics protocol, async previews, transparent backgrounds.
- **atuin** — fuzzy shell-history search with inline preview. Direct analogy for our list view.
- **gh dash** — Charm-based dashboard of PRs/issues; YAML-driven custom views.
- **fzf** — the split-pane fuzzy + preview pattern almost everyone copies.
- **superfile**, **television**, **gitui**, **bottom** — solid Bubble Tea / ratatui exemplars.

Common visual ingredients across all of them: three-pane layout (list / detail / preview), Nerd Font icons (optional, with a no-Nerd fallback), soft rounded borders, dimmed metadata, persistent keybinding footer, fuzzy search with live highlight, syntax-highlighted preview pane.

## Mistakes to avoid

1. **Static-only export** (claude-code-log). Users want interactive navigation.
2. **Cost-only focus** (ccusage). Boring; covered.
3. **Electron / Tauri heaviness** (claudia). Wrong audience for a CLI-native tool.
4. **Flat rendering of the thread tree.** Every existing tool collapses the `parentUuid` graph into a line and loses branch/resume structure. Honoring the tree is a differentiator.
5. **Ignoring the sibling folders.** They're the bulk of the disk footprint, and they're nobody's job today.
