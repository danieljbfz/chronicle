# Provider Surface Analysis

This document is the authoritative reference for how chronicle reasons
about its supported providers. Every time someone proposes a new
chronicle feature, the question is the same: what does each provider
expose, where do those exposures overlap, and is the new chronicle
surface a provider-agnostic primitive or a provider-specific
capability that some adapters opt into?

The document is a snapshot. AI coding tools evolve quickly. Re-read it
against the current docs before making architectural decisions that
depend on it.

Sources: official Claude Code documentation (`code.claude.com/docs`)
and GitHub Copilot documentation (`docs.github.com/en/copilot`),
December 2026 snapshot. Both verified against the working machine's
on-disk state at the time of writing.

---

## Why this matters

Chronicle started as a Claude Code session browser. The architecture
generalized to multi-provider through optional capability interfaces.
That generalization is structurally correct but the design has drifted
in two ways the work below corrects:

1. **Concept coverage is uneven.** Chronicle models 5 of the ~30
   distinct user-facing concepts Claude Code exposes. Copilot exposes
   a similar number, and chronicle models a different 1-2 of them. The
   uneven coverage is fine, but it should be deliberate, not
   accidental.

2. **The Copilot adapter sees one surface, not all of them.** Copilot
   has at least three distinct local-state surfaces (VS Code chat,
   Copilot CLI sessions, IDE-resident agent state) and chronicle today
   reads only the first. That is a real coverage gap, not a design
   choice.

The work below names every concept on both sides, decides what
chronicle should model, and writes down the reasons for what it
deliberately does not.

---

## The four buckets

Every concept either AI tool exposes falls into one of four buckets.
The bucket determines how chronicle should treat it.

**Bucket 1: Shared concepts with the same shape.**
Both tools have it, with similar enough storage and semantics that one
abstraction fits both. Chronicle's `contracts.Provider` interface
models these. Example: sessions (a sequence of user/assistant turns
on disk).

**Bucket 2: Shared concepts with different shapes.**
Both tools have it, but the storage is dissimilar enough that one
adapter has to translate to the chronicle abstraction. Chronicle's
optional capability interfaces (`Cleaner`, `MemoryStore`,
`GlobalMemoryStore`, `Resumable`, `GlobalConfig`) live here.
Example: per-project memory exists in both tools but with different
file conventions.

**Bucket 3: Provider-specific concepts.**
Only one tool has it. Chronicle either models it as an optional
capability that only one adapter implements (the current pattern) or
deliberately leaves it alone. Example: Claude's `~/.claude.json` is
specific to Claude; a Cursor or Antigravity adapter would have a
different file or no equivalent.

**Bucket 4: Chronicle-added concepts.**
Neither tool exposes it as a first-class concept; chronicle invented
it. Example: cross-provider search, cross-provider stats,
multi-provider unification at the CLI surface.

The architectural rule is the same in every bucket: the contracts
layer never knows the names of any specific provider, the composition
layer never imports any specific adapter, and the CLI never speaks
about "Claude" or "Copilot" except through user-facing examples
clearly marked as such.

---

## Claude Code: user-facing concept inventory

The list below covers concepts that have on-disk state chronicle
could in principle inspect, browse, export, or clean. Runtime-only
concepts (Computer Use, Voice Dictation, Streaming output, observability
exports) are out of scope by definition.

| Concept | On-disk location | What chronicle does today |
| --- | --- | --- |
| Sessions (transcripts) | `~/.claude/projects/<encoded-cwd>/<sessionID>.jsonl` | ✓ list, export, copy, search, resume |
| Session companion artifacts | `~/.claude/projects/<encoded-cwd>/<sessionID>/` (subagents, tool-results) | ✓ cascade-delete via clean abandoned/stale |
| File history | `~/.claude/file-history/<sessionID>/` | ✓ orphan-aware cleanup |
| Captured environment | `~/.claude/session-env/<sessionID>/` | ✓ orphan-aware cleanup |
| Task state | `~/.claude/tasks/<sessionID>/` | ✓ orphan-aware cleanup |
| Paste cache | `~/.claude/paste-cache/<hash>.txt` | ✓ history-cross-referenced cleanup |
| Shell snapshots | `~/.claude/shell-snapshots/` | ✓ keep-recent cleanup |
| Config backups | `~/.claude/backups/.claude.json.backup.<ms>` | ✓ keep-recent cleanup |
| Security warnings state | `~/.claude/security_warnings_state_<id>.json` | ✓ live-session-aware cleanup |
| Auto memory (per-project) | `~/.claude/projects/<encoded-cwd>/memory/MEMORY.md` + topic files | ✓ list, show, edit, clean |
| User-global instructions | `~/.claude/CLAUDE.md` | ✓ via `--global` |
| User-global config | `~/.claude.json` (projects map) | ✓ dangling cleanup |
| Plans | `~/.claude/plans/<slug>.md` | ✗ (audit notes: user-readable, no UUID linkage) |
| Live process descriptors | `~/.claude/sessions/<pid>.json` | ✗ (deliberately untouched, runtime state) |
| Managed-policy instructions | `/Library/Application Support/ClaudeCode/CLAUDE.md` (etc) | ✗ (read-only org policy) |
| Project-root instructions | `<repo>/CLAUDE.md`, `<repo>/CLAUDE.local.md` | ✗ (lives in user's repos, not chronicle's scope) |
| Project rules | `<repo>/.claude/rules/<name>.md` | ✗ (lives in user's repos) |
| Personal skills | `~/.claude/skills/<name>/SKILL.md` | ✗ (out of scope, see below) |
| Project skills | `<repo>/.claude/skills/<name>/SKILL.md` | ✗ (out of scope) |
| Project subagents | `<repo>/.claude/agents/<name>.md` | ✗ (out of scope) |
| User subagents | `~/.claude/agents/<name>.md` | ✗ (out of scope) |
| Project hooks | `<repo>/.claude/hooks.json` or `<repo>/.claude/hooks/` | ✗ (out of scope) |
| Commands (legacy) | `~/.claude/commands/<name>.md` or `<repo>/.claude/commands/` | ✗ (merged into skills upstream) |
| Plugins | `~/.claude/plugins/` | ✗ (managed by `claude plugin`) |
| Settings | `~/.claude/settings.json` | ✗ (managed by `claude config`-equivalent) |
| MCP server config | inside `~/.claude.json` or `~/.claude/settings.json` | ✗ (managed by `claude mcp`) |
| IDE state | `~/.claude/ide/` | ✗ (runtime state) |
| Caches | `~/.claude/cache/`, `~/.claude/stats-cache.json`, etc | ✗ (regenerable) |
| Routines | cloud-stored on Anthropic infrastructure | ✗ (not local state) |
| Telemetry | `~/.claude/telemetry/` | ✗ (out of scope) |

---

## GitHub Copilot: user-facing concept inventory

Copilot has three local-state surfaces that all matter for chronicle's
mission. Two of the three are NOT currently modeled.

### Surface A: VS Code chat sessions

| Concept | On-disk location | What chronicle does today |
| --- | --- | --- |
| Chat sessions | `<vscode>/User/workspaceStorage/<hash>/chatSessions/<id>.jsonl` | ✓ list, export, copy, search |
| Empty-window sessions | `<vscode>/User/globalStorage/.../emptyWindowChatSessions/<id>.jsonl` | ✓ list, export |
| Edit snapshots | `<vscode>/User/workspaceStorage/<hash>/chatEditingSessions/` | ✓ cascade-delete |
| CLI image attachments | `<vscode>/User/globalStorage/github.copilot-chat/copilot-cli-images/<sid>-*` | ✓ orphan-aware cleanup |

### Surface B: Copilot CLI sessions (NOT currently modeled)

| Concept | On-disk location | What chronicle does today |
| --- | --- | --- |
| CLI sessions | `~/.copilot/session-state/<id>/` | ✗ (real coverage gap) |
| VS Code↔CLI handoff metadata | `~/.copilot/vscode.session.metadata.cache.json` | ✗ |
| IDE bridge locks | `~/.copilot/ide/<id>.lock` | ✗ |

The Copilot CLI ships its own `/chronicle` slash command, which
suggests Anthropic and GitHub view chronicle-like inspection as
genuinely useful. The naming collision is unfortunate but unavoidable.

### Surface C: shared and cross-tool concepts

| Concept | Location varies | Notes |
| --- | --- | --- |
| Custom instructions (per-user, per-repo, per-org) | settings store + repo files | similar role to Claude's CLAUDE.md hierarchy |
| MCP servers | IDE settings + org/enterprise registry | shared standard, different config files |
| Plugins | `~/.copilot/plugins/` (CLI) + IDE marketplace | conceptually similar to Claude's plugin system |
| Cloud agents | GitHub cloud, no local state | not chronicle's domain |
| Cloud sessions | GitHub.com chat history | not chronicle's domain |

---

## Cross-provider taxonomy

This is the synthesis the architecture decisions hang on.

### Bucket 1: shared with same shape (base Provider interface)

| Concept | Claude | Copilot | Chronicle modeling |
| --- | --- | --- | --- |
| Sessions as ordered messages | jsonl, per-cwd folder | jsonl, per-workspace + global empty-window | `contracts.Provider` interface |
| Projects as named groupings | encoded-cwd folder name | workspace hash + "no-workspace" | `contracts.Project` |
| Session summaries | parseable from jsonl | parseable from jsonl | `contracts.SessionSummary` |
| Full conversations with blocks | text + tool + thinking | text + tool + ... | `contracts.Conversation` |

This is the foundation. Both tools fit cleanly.

### Bucket 2: shared with different shape (optional capabilities)

| Concept | Claude shape | Copilot shape | Chronicle modeling |
| --- | --- | --- | --- |
| Cleanup of stale data | Claude has its own cleaner with `cleanupPeriodDays` | Copilot has no equivalent | `contracts.Cleaner` (both implement; semantics differ) |
| Per-project memory | `projects/<cwd>/memory/MEMORY.md` + topic files | repo-level custom instructions | `contracts.MemoryStore` (Claude implements; Copilot does not, because per-repo files are user-source not chronicle-state) |
| User-global instructions | `~/.claude/CLAUDE.md` | Copilot personal custom instructions (cloud-stored or IDE settings) | `contracts.GlobalMemoryStore` (Claude implements; Copilot does not, because not on local disk) |
| User-global config with per-project entries | `~/.claude.json` projects map | no direct equivalent (per-project state lives in workspaceStorage, not a single map) | `contracts.GlobalConfig` (Claude implements; Copilot does not) |
| Resume in original tool | `claude --resume <id>` CLI flag | VS Code Chat has no external API to jump to a session by id; Copilot CLI does have session-state but resume semantics are not yet stable | `contracts.Resumable` (Claude implements; Copilot CLI candidate for later) |

The pattern: every optional capability that exists today is also a
real candidate for another adapter to implement, once we read that
adapter's docs and confirm the semantics line up. The capability
interfaces are not Claude-specific; they are concept-specific.

### Bucket 3: provider-specific concepts

These exist in one tool but not the other and chronicle correctly does
not abstract over them.

**Claude-specific:**
- `cleanupPeriodDays` setting and its semantics
- Skills with the SKILL.md format (Claude pioneered, now an open
  standard, but Copilot's plugin format diverges)
- `.claude/rules/` directory with `paths` frontmatter
- Claude's subagent system with custom agents
- Computer Use, routines, channels (runtime/cloud concerns)

**Copilot-specific:**
- VS Code workspace storage hash mechanism
- GitHub cloud-stored chat history
- The `gh copilot` CLI (separate codebase from `~/.copilot/`)
- Org/enterprise centrally-managed MCP registry
- Copilot Spaces

Chronicle should not invent abstractions over these unless a third
provider with the same concept arrives.

### Bucket 4: chronicle-added concepts

These exist only because chronicle invented them. They are the value
proposition of having a multi-provider history manager that is
neither tool itself.

- Cross-provider session listing
- Cross-provider stats and disk-usage breakdown
- Cross-provider search
- Cross-provider export-to-Markdown
- Cross-provider clipboard via OSC 52
- Recoverable trash for any deletion the tools do not offer
- Inspection of historic memory state across tools
- Inspection of user-global config across tools

Each of these is a value add that neither upstream tool provides
because each upstream tool only sees its own data. Chronicle's job is
to be the one tool that sees them all.

---

## Architecture decisions this analysis forces

The CLI is stable. The architecture is stable. But this analysis
surfaces three real gaps and three deferred decisions.

### Gap 1: Copilot CLI sessions are not modeled

`~/.copilot/session-state/<id>/` contains real session data on the
working machine right now. Chronicle does not see it. The right fix is
either:

(a) Extend the existing Copilot adapter to read both VS Code Chat and
    Copilot CLI surfaces. Each becomes a "project" or a sibling root
    inside one adapter.

(b) Split Copilot into two adapters: `copilot-vscode` and
    `copilot-cli`, each modeling its own surface independently.

The architecture supports both. The right call depends on whether the
two Copilot surfaces share schemas (favoring option a) or differ
substantially (favoring option b). A small research pass on the
on-disk layout of `~/.copilot/session-state/<id>/` resolves the
question. **This is a real coverage gap worth filing as the next
piece of architectural work.**

### Gap 2: cross-provider concepts that exist but chronicle ignores

**Project-level custom instructions** live in `<repo>/CLAUDE.md` (or
`<repo>/.copilot-instructions.md` for Copilot). Chronicle does not
model them because they live in the user's git repos, not in any
tool's local-state directory. The audit pass concluded this was the
right choice: chronicle's mission is local-state management, not
"every file an AI tool reads." Sticking with that decision.

**MCP server configurations** exist in both tools but are managed by
each tool's own CLI (`claude mcp`, `gh copilot mcp`). Chronicle could
offer a cross-provider listing view, but it cannot meaningfully edit
the configs without becoming a thin lossy wrapper. The right
chronicle surface, if any, is read-only inspection. **Filed as a
future "Maybe."**

### Gap 3: cross-tool open standards exist but chronicle does not lean on them

`AGENTS.md` is the cross-tool equivalent of `CLAUDE.md`. The Agent
Skills standard at agentskills.io defines a portable skill format. If
chronicle ever expands its inspection surface to skills or
project-level instructions, it should model them through these
standards, not through Claude's or Copilot's specific shapes. **This
is an architectural principle, not a feature decision.**

### Deferred decision 1: model in the SessionSummary

The user's session JSONL files now carry a `model` field (Claude
sessions identify the underlying model used, including third-party
models like MiniMax routed through ANTHROPIC_BASE_URL). Chronicle
currently does not surface this in `SessionSummary`. Adding it would
let `chronicle stats --by-model` work and let `chronicle list --json`
expose it. **This is the smallest real follow-up worth doing**, and
because the field's semantics are uniform across providers (the
upstream tool always knows what model served the request), the
chronicle abstraction is just "a string the user might want to filter
on." Not a new capability, just a new field on an existing struct.

### Deferred decision 2: skills as a chronicle surface

Skills as a portable, cross-tool concept are real. Each tool has its
own skills directory. A `chronicle skills list` command that walks
both tools' directories is a defensible product idea. But the audit
principle holds: each tool already has its own UI for listing
installed skills, and chronicle being a wrapper buys little. The
cross-tool angle is real value, but small. **Filed as a "Maybe" that
needs a real user need before we build it.**

### Deferred decision 3: the TUI surface

Now that the full concept surface is mapped, the TUI design becomes
tractable. The TUI must wrap every chronicle command without
proliferating into a tool that tries to do what `claude` and `gh
copilot` do. The right TUI scope is exactly what chronicle's CLI
already does: browse, search, stats, export, clean, memory, resume,
config, dangling — six to eight pages, each wrapping one CLI
subcommand family. **The TUI is the next big strategic piece, and
this analysis confirms it is the right next piece because the CLI
surface is stable.**

---

## Provider capability matrix

This table is the user-facing version of the bucket analysis above.
The README references it.

| Capability | Claude Code | GitHub Copilot Chat (VS Code) | GitHub Copilot CLI |
| --- | --- | --- | --- |
| Base `Provider` (list, read sessions) | ✓ | ✓ | ✗ (not yet modeled) |
| `Cleaner` (delete sessions, scan orphans) | ✓ | ✓ | ✗ |
| `MemoryStore` (per-project memory) | ✓ | ✗ (no per-project memory in VS Code Chat) | ✗ |
| `GlobalMemoryStore` (user-wide instructions) | ✓ | ✗ (cloud-stored, not local) | ✗ |
| `Resumable` (re-open in original tool) | ✓ | ✗ (no external API) | ✗ (candidate, semantics not yet stable) |
| `GlobalConfig` (per-project config entries) | ✓ | ✗ (no single global config with project map) | ✗ |

The asymmetry is real and intentional. Each row reflects what the
underlying tool actually exposes, not chronicle's preferences.

---

## Decision principles, summarized

1. **The base Provider is required**. Optional capabilities are
   discovered by type assertion. New providers ship only what their
   tool supports.

2. **The contracts layer never names a provider**. Adapter-specific
   constants (filenames, directory layouts, executable names) stay
   inside the adapter package.

3. **The composition layer never imports an adapter**. It iterates
   `a.providers` and type-asserts to capability interfaces. The
   registry in `adapters/all.go` is the only place that knows the
   adapter package names.

4. **The CLI never speaks about a provider by name**. Provider
   examples in help text route through `chronicle doctor` so the
   user sees what is actually registered on their machine.

5. **Adding a new provider is one new package + one entry in the
   registry**. The config layer's `map[string]ProviderConfig`
   already supports unknown provider names round-tripping through
   TOML.

6. **Skip-by-default for concepts the upstream tool already manages
   well**. Plugins, MCP servers, IDE settings, cloud sessions. The
   chronicle value is in the gap the upstream tools leave, not in
   wrapping what they already do.

7. **No preferential treatment**. Every feature decision is
   structured to fit any number of future adapters. Claude is not the
   canonical provider. Copilot is not the test case. They are two
   instances of the abstract "AI coding tool with on-disk history."

---

## What to read next when this document is stale

- Claude Code documentation index: `code.claude.com/docs/llms.txt`
- Copilot documentation entry: `docs.github.com/en/copilot`
- Agent Skills standard: `agentskills.io`
- `AGENTS.md` standard: search GitHub for repos using it
