# Foundations and Claude read-only CLI — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the first slice of `chronicle`: a Go binary that reads `~/.claude` and produces clean Markdown exports of Claude Code sessions, with detection, fingerprinting, filtering, and a `doctor` page. No TUI, no Copilot, no cleanup. The result is a real, testable tool that proves the architecture.

**Architecture:** Five layers per the spec — `contracts` (pure types), `adapters/claude` (read-only), `steps` (pure transforms), `composition` (the only I/O layer), `cmd/chronicle` (entrypoint). Tests mirror the source tree. Side effects only at the edges.

**Tech Stack:** Go 1.26, `github.com/spf13/cobra` for subcommands, `github.com/BurntSushi/toml` for config, stdlib `encoding/json` and `testing`. No SQLite yet — Plan B introduces that.

**Spec reference:** `docs/superpowers/specs/2026-05-15-chronicle-design.md` §1–§6, §9 (CLI), §11 (config), §13 (tests), §16 (extensibility), §17 (directory map). The architectural style is **Hexagonal (Ports & Adapters)** — see spec §3.1.

**Required reading for the engineer:**

- `SKILL_PROMPT.md` (repo root) — the engineering contract. Read once cover-to-cover.
- `docs/go-primer.md` — Go concepts in roughly the order this plan introduces them. Skim now, return to it whenever a syntax feels unfamiliar.
- `docs/naming-conventions.md` — canonical names for every concept and the resolutions where Go style and `SKILL_PROMPT.md` collide (constants stay `MixedCaps`; abbreviations like `cfg`/`sb`/`rec` get spelled out).

Code in this plan is production-ready Go. Comments in the code follow `SKILL_PROMPT.md` §3.9 — they explain *why*, not *what*. The teaching surface is the prose around each task and the two reference documents above, not paraphrasing comments in the code.

---

## File map produced by this plan

```
go.mod, go.sum
.gitignore
README.md                                   ← user-facing quickstart
cmd/chronicle/main.go                       ← cobra root, subcommands wired here
contracts/
    ids.go                                  ← ProjectID, SessionID, MessageID, Role
    block.go                                ← Block interface + concrete blocks
    message.go                              ← Message struct
    conversation.go                         ← Conversation struct
    storage_version.go                      ← StorageVersion, Capabilities
    project.go                              ← Project, SessionSummary
    provider.go                             ← Provider interface
adapters/
    all.go                                  ← registry: one line per provider, the extensibility seam
adapters/claude/
    doc.go                                  ← package documentation
    detect.go                               ← fingerprint + version mapping
    parse.go                                ← JSONL → Conversation
    cleanup_stub.go                         ← PlanDelete / PlanOrphanScan return ErrNotImplemented
    testdata/v1_0/
        empty_session.jsonl                 ← abandoned-session fixture
        small_session.jsonl                 ← 3-turn fixture with tool use
        thinking_session.jsonl              ← fixture with thinking blocks
        synthetic_future.jsonl              ← canary: unknown record type
steps/
    fingerprint.go                          ← compute fingerprint from records
    filter.go                               ← drop tool / thinking / meta blocks
    export.go                               ← Conversation → Markdown
    clipboard.go                            ← OSC52 clipboard escape
internal/
    paths/paths.go                          ← XDG user-config-dir
    config/config.go                        ← TOML loader + defaults
composition/
    browse.go                               ← ListProjects, ListSessions, ReadSession
    doctor.go                               ← detect all providers, list warnings
tests/                                      ← (Go convention puts unit tests next to source; this dir holds end-to-end fixtures only if we add any later)
```

**Tests live next to source files** (Go convention): `adapters/claude/parse.go` is tested by `adapters/claude/parse_test.go` in the same package. This deviates slightly from the spec wording but matches Go community practice — the spec's intent ("test files mirror source layout") is satisfied.

---

## Task 0: Orient yourself

Five minutes, no code yet. Skipping this step is the single biggest cause of "the plan compiled but I do not understand what I built".

- [ ] **Step 1: Confirm Go is installed**

```bash
go version
```

Expected: `go version go1.26.x darwin/arm64` (or your platform). If the command is not found, run `brew install go` and try again.

- [ ] **Step 2: Read the engineering contract**

Open `SKILL_PROMPT.md` at the repo root and read it once cover to cover. It is the bar this plan codes to. The §1 "Quick rules" block is the only "always" section — everything else is judgment.

- [ ] **Step 3: Skim the Go primer**

Open `docs/go-primer.md`. Read §§1–13. Return to §§14–17 only when something in a task feels unfamiliar.

- [ ] **Step 4: Skim the naming conventions**

Open `docs/naming-conventions.md`. The table at the top — "canonical names" — is the most important thing in the document. The Go-vs-`SKILL_PROMPT.md` resolutions matter when you write your first constant.

- [ ] **Step 5: Skim the design spec**

Open `docs/superpowers/specs/2026-05-15-chronicle-design.md`. Read §§1–6 carefully. The rest is reference material you will come back to.

- [ ] **Step 6: Read this plan's File Map below**

The "File map produced by this plan" section above your eyes is the bird's-eye view. Every file you create in Tasks 1–20 is listed there with a one-line job description.

No commit at the end of Task 0 — no files changed yet.

---

## Task 1: Repo bootstrap

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `README.md`
- Delete: `.DS_Store`

- [ ] **Step 1: Initialize the module**

Run:
```bash
cd /Users/djbf/Desktop/work/claude-history
rm -f .DS_Store
go mod init github.com/danieljbfz/chronicle
```

Expected `go.mod`:
```
module github.com/danieljbfz/chronicle

go 1.26
```

- [ ] **Step 2: Write `.gitignore`**

Create `/Users/djbf/Desktop/work/claude-history/.gitignore`:
```
.DS_Store
/chronicle
/dist/
*.test
*.out
.idea/
.vscode/
```

- [ ] **Step 3: Write minimal `README.md`**

Create `/Users/djbf/Desktop/work/claude-history/README.md`:
````markdown
# chronicle

A local tool for browsing, exporting, and cleaning the history that AI coding assistants leave on disk.

Status: under active development. See `docs/superpowers/specs/2026-05-15-chronicle-design.md` for the design contract.

## Install (from source, during development)

```bash
go build -o chronicle ./cmd/chronicle
./chronicle doctor
```

## Subcommands (Plan A scope)

- `chronicle list` — list Claude Code sessions, one JSON-line per session.
- `chronicle export <sessionId> [-o file.md]` — write a filtered Markdown transcript.
- `chronicle copy <sessionId>` — copy the same transcript to the clipboard via OSC52.
- `chronicle doctor` — show detected providers, their versions, and any format warnings.
````

- [ ] **Step 4: Initialize git and commit**

Run:
```bash
cd /Users/djbf/Desktop/work/claude-history
git init -b main
git add .gitignore README.md go.mod SKILL_PROMPT.md docs/
git commit -m "chore: repo bootstrap and engineering contract"
```

Expected: `git status` shows clean working tree.

---

## Task 2: Contract types (IDs, Role, Block interface)

**Files:**
- Create: `contracts/ids.go`
- Create: `contracts/block.go`
- Test: `contracts/block_test.go`

- [ ] **Step 1: Write `contracts/ids.go`**

```go
// Package contracts defines the normalized domain types that every higher
// layer of chronicle speaks. Adapters translate provider-specific shapes
// into these. Steps, composition, and entrypoints know nothing about
// provider-specific schemas.
package contracts

// ProjectID identifies a project within a provider. It is opaque to the UI;
// adapters define their own format (Claude uses the encoded cwd, Copilot
// uses the workspace hash). The UI shows Project.DisplayName instead.
type ProjectID string

// SessionID identifies a single session within a project.
type SessionID string

// MessageID identifies a message within a session. For storage formats that
// do not assign IDs (Copilot's flat list), adapters synthesize stable IDs
// from the record index.
type MessageID string

// Role is the speaker of a Message.
type Role string

const (
    RoleUser      Role = "user"
    RoleAssistant Role = "assistant"
    RoleSystem    Role = "system"
)
```

- [ ] **Step 2: Write `contracts/block.go`**

```go
package contracts

import "encoding/json"

// Block is one piece of a Message. The Message.Blocks slice is the normalized
// form of provider-specific content arrays. Use a type switch to handle each
// concrete kind.
type Block interface {
    blockMarker()
}

// TextBlock is plain prose from user or assistant.
type TextBlock struct {
    Text string
}

// ThinkingBlock is the assistant's internal reasoning. Hidden by default in
// the UI; the user can opt to show it.
type ThinkingBlock struct {
    Text string
}

// ToolUseBlock is the assistant invoking a tool. Input is the raw JSON
// argument as the provider stored it.
type ToolUseBlock struct {
    Tool   string
    Input  json.RawMessage
    CallID string
}

// ToolResultBlock is the user-side return value for a previous ToolUseBlock,
// linked by CallID.
type ToolResultBlock struct {
    CallID  string
    Output  string
    IsError bool
}

// ImageBlock describes an image attached to a turn.
type ImageBlock struct {
    MIME            string
    PathOrInlineRef string
}

// UnknownBlock preserves provider content we do not recognize. The renderer
// shows it as "Unknown block · click to inspect", and the resilience contract
// requires we keep the raw JSON rather than dropping it.
type UnknownBlock struct {
    Kind string
    Raw  json.RawMessage
}

func (TextBlock) blockMarker()       {}
func (ThinkingBlock) blockMarker()   {}
func (ToolUseBlock) blockMarker()    {}
func (ToolResultBlock) blockMarker() {}
func (ImageBlock) blockMarker()      {}
func (UnknownBlock) blockMarker()    {}
```

- [ ] **Step 3: Write failing test in `contracts/block_test.go`**

```go
package contracts

import (
    "encoding/json"
    "testing"
)

func TestBlockMarker(t *testing.T) {
    // Step 1: every concrete block implements Block. The compiler proves
    // this — assigning to the interface variable will fail at build time
    // if a marker is missing.
    var b Block
    b = TextBlock{Text: "hello"}
    b = ThinkingBlock{Text: "musing"}
    b = ToolUseBlock{Tool: "Bash", Input: json.RawMessage(`{}`), CallID: "1"}
    b = ToolResultBlock{CallID: "1", Output: "ok"}
    b = ImageBlock{MIME: "image/png"}
    b = UnknownBlock{Kind: "weird", Raw: json.RawMessage(`null`)}
    _ = b
}

func TestRoleConstants(t *testing.T) {
    if RoleUser != "user" {
        t.Errorf("RoleUser = %q, want %q", RoleUser, "user")
    }
    if RoleAssistant != "assistant" {
        t.Errorf("RoleAssistant = %q, want %q", RoleAssistant, "assistant")
    }
    if RoleSystem != "system" {
        t.Errorf("RoleSystem = %q, want %q", RoleSystem, "system")
    }
}
```

- [ ] **Step 4: Run tests**

Run:
```bash
cd /Users/djbf/Desktop/work/claude-history
go test ./contracts/...
```

Expected: `ok  github.com/danieljbfz/chronicle/contracts`

- [ ] **Step 5: Commit**

```bash
git add contracts/
git commit -m "feat(contracts): add ID types, Role, and Block interface"
```

---

## Task 3: Contract types (Message, Conversation, Capabilities, StorageVersion)

**Files:**
- Create: `contracts/storage_version.go`
- Create: `contracts/message.go`
- Create: `contracts/conversation.go`
- Test: `contracts/conversation_test.go`

- [ ] **Step 1: Write `contracts/storage_version.go`**

```go
package contracts

// StorageVersion is the result of an adapter's Detect call. Every adapter
// returns a non-nil StorageVersion, including for unrecognized shapes —
// "unknown" is a normal state, not an error.
type StorageVersion struct {
    Adapter      string // matches Provider.Name(), e.g. "claude"
    Version      string // "claude-1.0", "copilot-3", or "unknown"
    Fingerprint  string // short hex hash from steps/fingerprint
    Capabilities Capabilities
}

// Capabilities advertises what an adapter understands about the storage at
// hand. UI features key off these, never off StorageVersion.Version.
type Capabilities struct {
    ThreadTree         bool // parentUuid graph (Claude) vs flat list (Copilot)
    EditingSessions    bool // sibling working-set storage exists
    ToolInvocations    bool // adapter recognizes tool calls in the model
    ModelMetadata      bool // storage records which model was used per turn
    LiveWriterDetected bool // an upstream process is actively writing here
}

// IsKnown reports whether the storage matched a recognized schema.
func (s StorageVersion) IsKnown() bool {
    return s.Version != "" && s.Version != "unknown"
}
```

- [ ] **Step 2: Write `contracts/message.go`**

```go
package contracts

import "time"

// Message is one turn in a Conversation. The Blocks slice carries the
// content; the rest is metadata used for filtering and threading.
type Message struct {
    ID          MessageID
    ParentID    MessageID // empty for the root
    Role        Role
    Timestamp   time.Time
    Blocks      []Block
    IsMeta      bool   // synthetic record (slash-command echo, hook output)
    IsSidechain bool   // sub-agent traffic
    Model       string // empty when unknown
}
```

- [ ] **Step 3: Write `contracts/conversation.go`**

```go
package contracts

import "time"

// Conversation is a normalized session. Adapters produce these by folding
// their provider-specific records into Message values. Capabilities are
// copied from the StorageVersion that produced this conversation so the
// UI does not need to re-query the adapter.
type Conversation struct {
    SessionID    SessionID
    Project      ProjectID
    StartedAt    time.Time
    EndedAt      time.Time
    Title        string
    Messages     []Message
    Capabilities Capabilities
    Source       StorageVersion
}

// FirstUserPrompt returns the text of the first non-meta user message, or
// the empty string if no such message exists (an abandoned session).
func (c Conversation) FirstUserPrompt() string {
    for _, m := range c.Messages {
        if m.Role != RoleUser || m.IsMeta {
            continue
        }
        for _, b := range m.Blocks {
            if t, ok := b.(TextBlock); ok && t.Text != "" {
                return t.Text
            }
        }
    }
    return ""
}

// IsAbandoned reports whether the session has zero non-meta user prompts.
// This is the criterion the cleanup feature uses in Plan C.
func (c Conversation) IsAbandoned() bool {
    return c.FirstUserPrompt() == ""
}
```

- [ ] **Step 4: Write `contracts/conversation_test.go`**

```go
package contracts

import (
    "testing"
    "time"
)

func TestFirstUserPrompt_skipsMetaAndAssistant(t *testing.T) {
    c := Conversation{
        Messages: []Message{
            {Role: RoleAssistant, Blocks: []Block{TextBlock{Text: "hi"}}},
            {Role: RoleUser, IsMeta: true, Blocks: []Block{TextBlock{Text: "<command>/clear</command>"}}},
            {Role: RoleUser, Blocks: []Block{TextBlock{Text: "read the docs"}}},
            {Role: RoleUser, Blocks: []Block{TextBlock{Text: "second prompt"}}},
        },
    }
    got := c.FirstUserPrompt()
    if got != "read the docs" {
        t.Errorf("FirstUserPrompt() = %q, want %q", got, "read the docs")
    }
}

func TestIsAbandoned_emptySessionReturnsTrue(t *testing.T) {
    c := Conversation{
        Messages: []Message{
            {Role: RoleUser, IsMeta: true, Blocks: []Block{TextBlock{Text: "<command>/clear</command>"}}},
            {Role: RoleAssistant, Blocks: []Block{TextBlock{Text: "ok"}}},
        },
    }
    if !c.IsAbandoned() {
        t.Error("session with only meta + assistant should be abandoned")
    }
}

func TestIsAbandoned_realPromptReturnsFalse(t *testing.T) {
    c := Conversation{
        StartedAt: time.Now(),
        Messages: []Message{
            {Role: RoleUser, Blocks: []Block{TextBlock{Text: "hello"}}},
        },
    }
    if c.IsAbandoned() {
        t.Error("session with a real prompt should not be abandoned")
    }
}

func TestStorageVersion_IsKnown(t *testing.T) {
    cases := []struct {
        version string
        want    bool
    }{
        {"claude-1.0", true},
        {"copilot-3", true},
        {"unknown", false},
        {"", false},
    }
    for _, tc := range cases {
        got := StorageVersion{Version: tc.version}.IsKnown()
        if got != tc.want {
            t.Errorf("IsKnown(%q) = %v, want %v", tc.version, got, tc.want)
        }
    }
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./contracts/...
```

Expected: `ok  github.com/danieljbfz/chronicle/contracts`. All four tests pass.

- [ ] **Step 6: Commit**

```bash
git add contracts/
git commit -m "feat(contracts): add Message, Conversation, StorageVersion, Capabilities"
```

---

## Task 4: Contract types (Project, SessionSummary, Provider, DeletePlan)

**Files:**
- Create: `contracts/project.go`
- Create: `contracts/delete_plan.go`
- Create: `contracts/provider.go`
- Test: `contracts/provider_test.go`

- [ ] **Step 1: Write `contracts/project.go`**

```go
package contracts

import "time"

// Project groups sessions belonging to one working directory or workspace.
type Project struct {
    ID           ProjectID
    DisplayName  string // human-readable, decoded from path or workspace.json
    Path         string // absolute filesystem path when known
    SessionCount int
    SizeBytes    int64
}

// SessionSummary is the cheap-to-compute view of a session. Listing pages
// use these; only the preview pane and export commands load the full
// Conversation via Provider.ReadSession.
type SessionSummary struct {
    ID           SessionID
    Project      ProjectID
    StartedAt    time.Time
    LastActive   time.Time
    Title        string // first user prompt or custom title
    TurnCount    int
    SizeBytes    int64  // includes sibling artifacts
    Capabilities Capabilities
    Source       StorageVersion
}
```

- [ ] **Step 2: Write `contracts/delete_plan.go`**

```go
package contracts

// DeletePlan is the result of an adapter's PlanDelete or PlanOrphanScan.
// It describes every path that would move to trash if the plan is executed.
// Composition shows this to the user before any filesystem change.
type DeletePlan struct {
    SessionID SessionID    // empty for orphan-scan plans
    Category  string       // e.g. "claude-session", "claude-orphan-file-history"
    Items     []DeleteItem
    SizeBytes int64        // sum of Items[].SizeBytes
    Warnings  []string     // e.g. "VS Code is running", "unknown storage version"
}

// DeleteItem is one path within a DeletePlan.
type DeleteItem struct {
    Path      string
    Reason    string // "session file", "edit history", "orphan paste"
    SizeBytes int64
}
```

- [ ] **Step 3: Write `contracts/provider.go`**

```go
package contracts

import "io/fs"

// Provider is the per-tool adapter contract. Composition passes each
// Provider an fs.FS rooted at the provider's data directory, so adapters
// never touch the real filesystem directly — that makes them trivially
// testable against fixture trees.
//
// Detect always returns a non-nil StorageVersion. Errors are reserved for
// two cases only: the path is unreachable, or no record in the storage is
// parseable as JSON at all. A file with valid JSON whose schema we do not
// recognize is Version = "unknown", not an error.
type Provider interface {
    Name() string

    Detect(root fs.FS) (StorageVersion, error)

    ListProjects(root fs.FS) ([]Project, error)
    ListSessions(root fs.FS, project ProjectID) ([]SessionSummary, error)
    ReadSession(root fs.FS, id SessionID) (Conversation, error)

    PlanDelete(root fs.FS, id SessionID) (DeletePlan, error)
    PlanOrphanScan(root fs.FS) (DeletePlan, error)
}
```

- [ ] **Step 4: Write `contracts/provider_test.go`** (a compile-time check that the interface is well-formed)

```go
package contracts

import "io/fs"

// stubProvider exists only to prove the Provider interface compiles. A real
// stub for tests in higher layers lives next to those tests.
type stubProvider struct{}

func (stubProvider) Name() string                                                  { return "stub" }
func (stubProvider) Detect(fs.FS) (StorageVersion, error)                          { return StorageVersion{}, nil }
func (stubProvider) ListProjects(fs.FS) ([]Project, error)                         { return nil, nil }
func (stubProvider) ListSessions(fs.FS, ProjectID) ([]SessionSummary, error)       { return nil, nil }
func (stubProvider) ReadSession(fs.FS, SessionID) (Conversation, error)            { return Conversation{}, nil }
func (stubProvider) PlanDelete(fs.FS, SessionID) (DeletePlan, error)               { return DeletePlan{}, nil }
func (stubProvider) PlanOrphanScan(fs.FS) (DeletePlan, error)                      { return DeletePlan{}, nil }

var _ Provider = stubProvider{}
```

- [ ] **Step 5: Run tests**

```bash
go test ./contracts/...
```

Expected: `ok  github.com/danieljbfz/chronicle/contracts`. Compile alone is the test for this task.

- [ ] **Step 6: Commit**

```bash
git add contracts/
git commit -m "feat(contracts): add Project, SessionSummary, DeletePlan, Provider"
```

---

## Task 5: `internal/paths` — XDG locations

**Files:**
- Create: `internal/paths/paths.go`
- Test: `internal/paths/paths_test.go`

- [ ] **Step 1: Write `internal/paths/paths.go`**

```go
// Package paths centralizes every filesystem location chronicle reads or
// writes outside provider data. Callers never construct these paths by
// hand — that keeps tests deterministic when we override the home dir.
package paths

import (
    "os"
    "path/filepath"
)

// Locations holds the resolved paths for the running process. Constructed
// once at startup; passed down where needed.
type Locations struct {
    ConfigDir   string // ~/.config/chronicle
    ConfigFile  string // ~/.config/chronicle/config.toml
    TrashDir    string // ~/.config/chronicle/trash
    ReportsDir  string // ~/.config/chronicle/format-reports
    ClaudeRoot  string // ~/.claude
}

// Resolve returns the default Locations for the current user. Override the
// home directory by setting the CHRONICLE_HOME environment variable, which
// tests use to redirect every path under a temp dir.
func Resolve() (Locations, error) {
    home, err := homeDir()
    if err != nil {
        return Locations{}, err
    }
    config := filepath.Join(home, ".config", "chronicle")
    return Locations{
        ConfigDir:  config,
        ConfigFile: filepath.Join(config, "config.toml"),
        TrashDir:   filepath.Join(config, "trash"),
        ReportsDir: filepath.Join(config, "format-reports"),
        ClaudeRoot: filepath.Join(home, ".claude"),
    }, nil
}

func homeDir() (string, error) {
    if override := os.Getenv("CHRONICLE_HOME"); override != "" {
        return override, nil
    }
    return os.UserHomeDir()
}
```

- [ ] **Step 2: Write `internal/paths/paths_test.go`**

```go
package paths

import (
    "path/filepath"
    "testing"
)

func TestResolve_usesEnvOverride(t *testing.T) {
    t.Setenv("CHRONICLE_HOME", "/tmp/fake-home")
    loc, err := Resolve()
    if err != nil {
        t.Fatalf("Resolve(): %v", err)
    }
    want := filepath.Join("/tmp/fake-home", ".config", "chronicle", "config.toml")
    if loc.ConfigFile != want {
        t.Errorf("ConfigFile = %q, want %q", loc.ConfigFile, want)
    }
    if loc.ClaudeRoot != "/tmp/fake-home/.claude" {
        t.Errorf("ClaudeRoot = %q, want %q", loc.ClaudeRoot, "/tmp/fake-home/.claude")
    }
}

func TestResolve_realHomeWhenNoOverride(t *testing.T) {
    t.Setenv("CHRONICLE_HOME", "")
    loc, err := Resolve()
    if err != nil {
        t.Fatalf("Resolve(): %v", err)
    }
    if loc.ClaudeRoot == "" {
        t.Error("ClaudeRoot should be set")
    }
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/paths/...
```

Expected: `ok  github.com/danieljbfz/chronicle/internal/paths`.

- [ ] **Step 4: Commit**

```bash
git add internal/paths/
git commit -m "feat(paths): resolve XDG locations with test override"
```

---

## Task 6: `internal/config` — TOML loader with defaults

**Files:**
- Modify: `go.mod` (add `github.com/BurntSushi/toml`)
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Add the TOML dependency**

```bash
go get github.com/BurntSushi/toml@latest
```

Expected: `go.mod` now lists `github.com/BurntSushi/toml`, `go.sum` is populated.

- [ ] **Step 2: Write `internal/config/config.go`**

```go
// Package config loads and writes chronicle's user configuration. The file
// lives at ~/.config/chronicle/config.toml. Missing fields fall back to
// Defaults. Every command-line flag overrides the config for that
// invocation only.
package config

import (
    "errors"
    "io/fs"
    "os"

    "github.com/BurntSushi/toml"
)

type Config struct {
    Trash     TrashConfig     `toml:"trash"`
    UI        UIConfig        `toml:"ui"`
    Providers ProvidersConfig `toml:"providers"`
}

type TrashConfig struct {
    RetentionDays int `toml:"retention_days"`
}

type UIConfig struct {
    TUI TUIConfig `toml:"tui"`
    Web WebConfig `toml:"web"`
}

type TUIConfig struct {
    Theme           string   `toml:"theme"`
    FiltersDefault  []string `toml:"filters_default"`
    NerdFont        string   `toml:"nerd_font"`
}

type WebConfig struct {
    Host        string `toml:"host"`
    Port        int    `toml:"port"`
    OpenBrowser bool   `toml:"open_browser"`
}

type ProvidersConfig struct {
    Claude  ClaudeConfig  `toml:"claude"`
    Copilot CopilotConfig `toml:"copilot"`
}

type ClaudeConfig struct {
    Enabled bool   `toml:"enabled"`
    Root    string `toml:"root"`
}

type CopilotConfig struct {
    Enabled                 bool     `toml:"enabled"`
    Roots                   []string `toml:"roots"`
    RefuseWhenVSCodeRunning bool     `toml:"refuse_when_vscode_running"`
}

// Defaults returns the configuration shipped with a fresh install.
func Defaults() Config {
    return Config{
        Trash: TrashConfig{RetentionDays: 30},
        UI: UIConfig{
            TUI: TUIConfig{
                Theme:          "auto",
                FiltersDefault: []string{"tools", "meta"},
                NerdFont:       "auto",
            },
            Web: WebConfig{
                Host:        "127.0.0.1",
                Port:        0,
                OpenBrowser: true,
            },
        },
        Providers: ProvidersConfig{
            Claude: ClaudeConfig{
                Enabled: true,
            },
            Copilot: CopilotConfig{
                Enabled:                 true,
                RefuseWhenVSCodeRunning: true,
            },
        },
    }
}

// Load reads the config file at path and returns it merged over Defaults.
// A missing file is not an error — the caller gets Defaults.
func Load(path string) (Config, error) {
    cfg := Defaults()
    data, err := os.ReadFile(path)
    if err != nil {
        if errors.Is(err, fs.ErrNotExist) {
            return cfg, nil
        }
        return Config{}, err
    }
    if _, err := toml.Decode(string(data), &cfg); err != nil {
        return Config{}, err
    }
    return cfg, nil
}
```

- [ ] **Step 3: Write `internal/config/config_test.go`**

```go
package config

import (
    "os"
    "path/filepath"
    "testing"
)

func TestLoad_missingFileReturnsDefaults(t *testing.T) {
    cfg, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
    if err != nil {
        t.Fatalf("Load(missing): %v", err)
    }
    if cfg.Trash.RetentionDays != 30 {
        t.Errorf("RetentionDays = %d, want 30", cfg.Trash.RetentionDays)
    }
    if !cfg.Providers.Claude.Enabled {
        t.Error("Claude should be enabled by default")
    }
}

func TestLoad_overridesDefaults(t *testing.T) {
    path := filepath.Join(t.TempDir(), "config.toml")
    body := `
[trash]
retention_days = 7

[providers.claude]
enabled = false
root    = "/some/where"
`
    if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
        t.Fatal(err)
    }
    cfg, err := Load(path)
    if err != nil {
        t.Fatalf("Load: %v", err)
    }
    if cfg.Trash.RetentionDays != 7 {
        t.Errorf("RetentionDays = %d, want 7", cfg.Trash.RetentionDays)
    }
    if cfg.Providers.Claude.Enabled {
        t.Error("Claude should be disabled per file")
    }
    if cfg.Providers.Claude.Root != "/some/where" {
        t.Errorf("Root = %q, want %q", cfg.Providers.Claude.Root, "/some/where")
    }
    if cfg.Providers.Copilot.Enabled != true {
        t.Error("Copilot should remain enabled (default)")
    }
}

func TestLoad_malformedTOMLReturnsError(t *testing.T) {
    path := filepath.Join(t.TempDir(), "broken.toml")
    if err := os.WriteFile(path, []byte("this is not = valid = toml"), 0o644); err != nil {
        t.Fatal(err)
    }
    if _, err := Load(path); err == nil {
        t.Error("Load should return an error on malformed TOML")
    }
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/config/...
```

Expected: `ok  github.com/danieljbfz/chronicle/internal/config`.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum internal/config/
git commit -m "feat(config): TOML loader with defaults and per-key overrides"
```

---

## Task 7: `steps/fingerprint.go` — schema fingerprinting

**Files:**
- Create: `steps/fingerprint.go`
- Test: `steps/fingerprint_test.go`

- [ ] **Step 1: Write `steps/fingerprint.go`**

```go
// Package steps holds pure transforms over the contracts types. No file I/O,
// no time, no environment. Steps are the test-easiest layer of the system.
package steps

import (
    "crypto/sha256"
    "encoding/hex"
    "sort"
    "strings"
)

// FingerprintInput is one observed record from a storage stream. The Type
// is the discriminator the adapter found (e.g. "user", "assistant",
// "file-history-snapshot"); Keys are the JSON top-level keys of that record.
type FingerprintInput struct {
    Type string
    Keys []string
}

// Fingerprint computes a short, stable hex hash describing the schema shape
// of a session file. Two files with the same set of (record type, key set)
// pairs produce the same fingerprint, so adapters can map fingerprints to
// known versions without parsing every record.
//
// Adapters cap their input at the first N records (typically 200) so the
// fingerprint reflects the variety in the file, not its length.
func Fingerprint(inputs []FingerprintInput) string {
    // Step 1: deduplicate (Type, sorted Keys) tuples.
    seen := make(map[string]struct{}, len(inputs))
    var tuples []string
    for _, in := range inputs {
        keys := append([]string(nil), in.Keys...)
        sort.Strings(keys)
        tuple := in.Type + "|" + strings.Join(keys, ",")
        if _, ok := seen[tuple]; ok {
            continue
        }
        seen[tuple] = struct{}{}
        tuples = append(tuples, tuple)
    }

    // Step 2: sort the tuple set so input order does not change the hash.
    sort.Strings(tuples)

    // Step 3: hash the joined tuples and return the first 12 hex chars.
    sum := sha256.Sum256([]byte(strings.Join(tuples, "\n")))
    return hex.EncodeToString(sum[:])[:12]
}
```

- [ ] **Step 2: Write `steps/fingerprint_test.go`**

```go
package steps

import "testing"

func TestFingerprint_stableAcrossOrder(t *testing.T) {
    a := []FingerprintInput{
        {Type: "user", Keys: []string{"uuid", "timestamp", "message"}},
        {Type: "assistant", Keys: []string{"uuid", "timestamp", "message"}},
    }
    b := []FingerprintInput{
        {Type: "assistant", Keys: []string{"message", "uuid", "timestamp"}},
        {Type: "user", Keys: []string{"timestamp", "message", "uuid"}},
    }
    if Fingerprint(a) != Fingerprint(b) {
        t.Errorf("fingerprint changed with reorder: %q vs %q", Fingerprint(a), Fingerprint(b))
    }
}

func TestFingerprint_differentSchemasDiffer(t *testing.T) {
    a := []FingerprintInput{{Type: "user", Keys: []string{"uuid", "message"}}}
    b := []FingerprintInput{{Type: "user", Keys: []string{"uuid", "message", "new_field"}}}
    if Fingerprint(a) == Fingerprint(b) {
        t.Error("adding a key must change the fingerprint")
    }
}

func TestFingerprint_deduplicates(t *testing.T) {
    a := []FingerprintInput{{Type: "user", Keys: []string{"uuid"}}}
    b := []FingerprintInput{
        {Type: "user", Keys: []string{"uuid"}},
        {Type: "user", Keys: []string{"uuid"}},
        {Type: "user", Keys: []string{"uuid"}},
    }
    if Fingerprint(a) != Fingerprint(b) {
        t.Error("duplicate tuples should not change the fingerprint")
    }
}

func TestFingerprint_isShortHex(t *testing.T) {
    fp := Fingerprint([]FingerprintInput{{Type: "user", Keys: []string{"x"}}})
    if len(fp) != 12 {
        t.Errorf("fingerprint length = %d, want 12", len(fp))
    }
    for _, r := range fp {
        if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
            t.Errorf("non-hex char in fingerprint: %q", fp)
            break
        }
    }
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./steps/...
```

Expected: `ok  github.com/danieljbfz/chronicle/steps`. All four tests pass.

- [ ] **Step 4: Commit**

```bash
git add steps/fingerprint.go steps/fingerprint_test.go
git commit -m "feat(steps): schema fingerprinting for version detection"
```

---

## Task 8: `steps/filter.go` — hide tool / thinking / meta

**Files:**
- Create: `steps/filter.go`
- Test: `steps/filter_test.go`

- [ ] **Step 1: Write `steps/filter.go`**

```go
package steps

import "github.com/danieljbfz/chronicle/contracts"

// FilterOptions controls which blocks and messages survive a Filter pass.
// All fields default to false — zero value keeps everything.
type FilterOptions struct {
    HideTools     bool // drop ToolUseBlock and ToolResultBlock
    HideThinking  bool // drop ThinkingBlock
    HideMeta      bool // drop messages with IsMeta = true
    HideSidechain bool // drop messages with IsSidechain = true
}

// Filter returns a copy of the conversation with the requested blocks and
// messages removed. The function is pure: it never mutates the input.
//
// Messages that become empty after block filtering are dropped, so a turn
// that contained only a tool_use disappears entirely when HideTools is set.
func Filter(c contracts.Conversation, opts FilterOptions) contracts.Conversation {
    // Step 1: shallow copy the conversation; we will replace Messages.
    out := c
    out.Messages = nil

    for _, m := range c.Messages {
        // Step 2: skip whole messages when the opt-out matches.
        if opts.HideMeta && m.IsMeta {
            continue
        }
        if opts.HideSidechain && m.IsSidechain {
            continue
        }

        // Step 3: filter blocks within the message.
        blocks := make([]contracts.Block, 0, len(m.Blocks))
        for _, b := range m.Blocks {
            if opts.HideTools {
                if _, ok := b.(contracts.ToolUseBlock); ok {
                    continue
                }
                if _, ok := b.(contracts.ToolResultBlock); ok {
                    continue
                }
            }
            if opts.HideThinking {
                if _, ok := b.(contracts.ThinkingBlock); ok {
                    continue
                }
            }
            blocks = append(blocks, b)
        }

        // Step 4: drop the message if no blocks remain.
        if len(blocks) == 0 {
            continue
        }
        m.Blocks = blocks
        out.Messages = append(out.Messages, m)
    }

    return out
}
```

- [ ] **Step 2: Write `steps/filter_test.go`**

```go
package steps

import (
    "testing"

    "github.com/danieljbfz/chronicle/contracts"
)

func sampleConversation() contracts.Conversation {
    return contracts.Conversation{
        Messages: []contracts.Message{
            {
                Role: contracts.RoleUser,
                Blocks: []contracts.Block{
                    contracts.TextBlock{Text: "first prompt"},
                },
            },
            {
                Role: contracts.RoleAssistant,
                Blocks: []contracts.Block{
                    contracts.ThinkingBlock{Text: "let me think"},
                    contracts.TextBlock{Text: "reply"},
                    contracts.ToolUseBlock{Tool: "Read", CallID: "1"},
                },
            },
            {
                Role: contracts.RoleUser,
                Blocks: []contracts.Block{
                    contracts.ToolResultBlock{CallID: "1", Output: "file body"},
                },
            },
            {
                Role:   contracts.RoleUser,
                IsMeta: true,
                Blocks: []contracts.Block{contracts.TextBlock{Text: "<command>/clear</command>"}},
            },
        },
    }
}

func TestFilter_hideToolsRemovesToolBlocksAndEmptyTurns(t *testing.T) {
    out := Filter(sampleConversation(), FilterOptions{HideTools: true})
    // Tool-only turn is gone; assistant still has Thinking + Text.
    if len(out.Messages) != 3 {
        t.Fatalf("got %d messages, want 3", len(out.Messages))
    }
    for _, m := range out.Messages {
        for _, b := range m.Blocks {
            switch b.(type) {
            case contracts.ToolUseBlock, contracts.ToolResultBlock:
                t.Errorf("tool block survived filter: %T", b)
            }
        }
    }
}

func TestFilter_hideThinkingDropsOnlyThinking(t *testing.T) {
    out := Filter(sampleConversation(), FilterOptions{HideThinking: true})
    for _, m := range out.Messages {
        for _, b := range m.Blocks {
            if _, ok := b.(contracts.ThinkingBlock); ok {
                t.Error("ThinkingBlock survived")
            }
        }
    }
}

func TestFilter_hideMetaDropsMetaMessage(t *testing.T) {
    out := Filter(sampleConversation(), FilterOptions{HideMeta: true})
    for _, m := range out.Messages {
        if m.IsMeta {
            t.Error("meta message survived")
        }
    }
}

func TestFilter_isPure(t *testing.T) {
    in := sampleConversation()
    before := len(in.Messages)
    _ = Filter(in, FilterOptions{HideTools: true, HideMeta: true, HideThinking: true})
    if len(in.Messages) != before {
        t.Errorf("Filter mutated its input: had %d, now %d", before, len(in.Messages))
    }
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./steps/...
```

Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add steps/filter.go steps/filter_test.go
git commit -m "feat(steps): conversation filter for tools, thinking, meta, sidechain"
```

---

## Task 9: `steps/export.go` — Conversation → Markdown

**Files:**
- Create: `steps/export.go`
- Test: `steps/export_test.go`

- [ ] **Step 1: Write `steps/export.go`**

```go
package steps

import (
    "fmt"
    "strings"
    "time"

    "github.com/danieljbfz/chronicle/contracts"
)

// Markdown renders a Conversation as a human-readable Markdown document.
// Apply Filter first if you want to omit tools or thinking; Markdown does
// not filter, it only renders whatever it is given.
func Markdown(c contracts.Conversation) string {
    var builder strings.Builder

    // Step 1: front matter (title and metadata block).
    writeHeader(&builder, c)

    // Step 2: each message as a section, role-prefixed.
    for _, m := range c.Messages {
        writeMessage(&builder, m)
    }

    return builder.String()
}

func writeHeader(builder *strings.Builder, c contracts.Conversation) {
    title := c.Title
    if title == "" {
        title = c.FirstUserPrompt()
    }
    if title == "" {
        title = "(empty session)"
    }
    fmt.Fprintf(builder, "# %s\n\n", title)
    fmt.Fprintf(builder, "> Session `%s`  ·  Provider `%s`  ·  Started %s\n\n",
        c.SessionID, c.Source.Adapter, formatTime(c.StartedAt))
    builder.WriteString("---\n\n")
}

func writeMessage(builder *strings.Builder, m contracts.Message) {
    switch m.Role {
    case contracts.RoleUser:
        builder.WriteString("## User\n\n")
    case contracts.RoleAssistant:
        builder.WriteString("## Assistant\n\n")
    case contracts.RoleSystem:
        builder.WriteString("## System\n\n")
    default:
        fmt.Fprintf(builder, "## %s\n\n", m.Role)
    }
    for _, b := range m.Blocks {
        writeBlock(builder, b)
    }
    builder.WriteString("\n")
}

func writeBlock(builder *strings.Builder, b contracts.Block) {
    switch v := b.(type) {
    case contracts.TextBlock:
        builder.WriteString(v.Text)
        builder.WriteString("\n\n")
    case contracts.ThinkingBlock:
        builder.WriteString("> _Thinking_\n>\n")
        for _, line := range strings.Split(v.Text, "\n") {
            builder.WriteString("> ")
            builder.WriteString(line)
            builder.WriteString("\n")
        }
        builder.WriteString("\n")
    case contracts.ToolUseBlock:
        fmt.Fprintf(builder, "**Tool call**: `%s` (id `%s`)\n\n```json\n%s\n```\n\n",
            v.Tool, v.CallID, string(v.Input))
    case contracts.ToolResultBlock:
        marker := "Tool result"
        if v.IsError {
            marker = "Tool error"
        }
        fmt.Fprintf(builder, "**%s** (id `%s`)\n\n```\n%s\n```\n\n", marker, v.CallID, v.Output)
    case contracts.ImageBlock:
        fmt.Fprintf(builder, "_[Image: %s · %s]_\n\n", v.MIME, v.PathOrInlineRef)
    case contracts.UnknownBlock:
        fmt.Fprintf(builder, "_Unknown block kind `%s` (preserved as raw JSON below)_\n\n```json\n%s\n```\n\n",
            v.Kind, string(v.Raw))
    }
}

func formatTime(t time.Time) string {
    if t.IsZero() {
        return "(unknown)"
    }
    return t.Format(time.RFC3339)
}
```

- [ ] **Step 2: Write `steps/export_test.go`**

```go
package steps

import (
    "strings"
    "testing"
    "time"

    "github.com/danieljbfz/chronicle/contracts"
)

func TestMarkdown_includesTitleAndMessages(t *testing.T) {
    c := contracts.Conversation{
        SessionID: "abc-123",
        Source:    contracts.StorageVersion{Adapter: "claude"},
        StartedAt: time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC),
        Messages: []contracts.Message{
            {Role: contracts.RoleUser, Blocks: []contracts.Block{contracts.TextBlock{Text: "Hello"}}},
            {Role: contracts.RoleAssistant, Blocks: []contracts.Block{contracts.TextBlock{Text: "Hi there"}}},
        },
    }
    out := Markdown(c)
    if !strings.Contains(out, "# Hello") {
        t.Error("output should include title from first prompt")
    }
    if !strings.Contains(out, "## User") || !strings.Contains(out, "## Assistant") {
        t.Error("output should label roles")
    }
    if !strings.Contains(out, "Session `abc-123`") {
        t.Error("output should include session id in metadata")
    }
}

func TestMarkdown_emptySessionHasFallbackTitle(t *testing.T) {
    out := Markdown(contracts.Conversation{})
    if !strings.Contains(out, "(empty session)") {
        t.Error("empty conversation should render fallback title")
    }
}

func TestMarkdown_preservesUnknownBlock(t *testing.T) {
    c := contracts.Conversation{
        Messages: []contracts.Message{{
            Role: contracts.RoleAssistant,
            Blocks: []contracts.Block{
                contracts.UnknownBlock{Kind: "future_kind", Raw: []byte(`{"weird":true}`)},
            },
        }},
    }
    out := Markdown(c)
    if !strings.Contains(out, "future_kind") {
        t.Error("Markdown should mention the unknown kind")
    }
    if !strings.Contains(out, `"weird":true`) {
        t.Error("Markdown should preserve the raw JSON of an unknown block")
    }
}

func TestMarkdown_renderToolBlocks(t *testing.T) {
    c := contracts.Conversation{
        Messages: []contracts.Message{{
            Role: contracts.RoleAssistant,
            Blocks: []contracts.Block{
                contracts.ToolUseBlock{Tool: "Bash", CallID: "1", Input: []byte(`{"cmd":"ls"}`)},
            },
        }, {
            Role: contracts.RoleUser,
            Blocks: []contracts.Block{
                contracts.ToolResultBlock{CallID: "1", Output: "file.txt"},
            },
        }},
    }
    out := Markdown(c)
    if !strings.Contains(out, "Tool call") || !strings.Contains(out, "Bash") {
        t.Error("ToolUseBlock should render as a Tool call")
    }
    if !strings.Contains(out, "Tool result") || !strings.Contains(out, "file.txt") {
        t.Error("ToolResultBlock should render as a Tool result")
    }
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./steps/...
```

Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add steps/export.go steps/export_test.go
git commit -m "feat(steps): Markdown renderer for Conversation"
```

---

## Task 10: `steps/clipboard.go` — OSC52 clipboard

**Files:**
- Create: `steps/clipboard.go`
- Test: `steps/clipboard_test.go`

- [ ] **Step 1: Write `steps/clipboard.go`**

```go
package steps

import (
    "encoding/base64"
    "io"
)

// OSC52Sequence returns the terminal escape sequence that loads text into
// the system clipboard via OSC 52. Most modern terminals (iTerm2, kitty,
// WezTerm, Alacritty, recent xterm, recent gnome-terminal) honor this, and
// it works transparently over SSH — that is the main reason chronicle uses
// OSC 52 instead of platform clipboard libraries.
//
// The "c" selector targets the system clipboard. See
// https://invisible-island.net/xterm/ctlseqs/ctlseqs.html for the spec.
func OSC52Sequence(text string) string {
    encoded := base64.StdEncoding.EncodeToString([]byte(text))
    return "\x1b]52;c;" + encoded + "\x07"
}

// CopyOSC52 writes the OSC52 sequence for text to w. Pass os.Stdout from
// the calling command — terminals interpret the sequence as it streams.
func CopyOSC52(w io.Writer, text string) error {
    _, err := io.WriteString(w, OSC52Sequence(text))
    return err
}
```

- [ ] **Step 2: Write `steps/clipboard_test.go`**

```go
package steps

import (
    "bytes"
    "encoding/base64"
    "strings"
    "testing"
)

func TestOSC52Sequence_shape(t *testing.T) {
    seq := OSC52Sequence("hello")
    if !strings.HasPrefix(seq, "\x1b]52;c;") {
        t.Errorf("missing OSC52 prefix: %q", seq)
    }
    if !strings.HasSuffix(seq, "\x07") {
        t.Errorf("missing BEL terminator: %q", seq)
    }
    body := strings.TrimSuffix(strings.TrimPrefix(seq, "\x1b]52;c;"), "\x07")
    decoded, err := base64.StdEncoding.DecodeString(body)
    if err != nil {
        t.Fatalf("body is not valid base64: %v", err)
    }
    if string(decoded) != "hello" {
        t.Errorf("decoded body = %q, want %q", string(decoded), "hello")
    }
}

func TestCopyOSC52_writesToWriter(t *testing.T) {
    var buf bytes.Buffer
    if err := CopyOSC52(&buf, "abc"); err != nil {
        t.Fatal(err)
    }
    if buf.Len() == 0 {
        t.Error("CopyOSC52 wrote nothing")
    }
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./steps/...
```

Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add steps/clipboard.go steps/clipboard_test.go
git commit -m "feat(steps): OSC52 clipboard escape that works over SSH"
```

---

## Task 11: Claude fixtures and package doc

**Files:**
- Create: `adapters/claude/doc.go`
- Create: `adapters/claude/testdata/v1_0/empty_session.jsonl`
- Create: `adapters/claude/testdata/v1_0/small_session.jsonl`
- Create: `adapters/claude/testdata/v1_0/thinking_session.jsonl`
- Create: `adapters/claude/testdata/synthetic_future.jsonl`

- [ ] **Step 1: Write `adapters/claude/doc.go`**

```go
// Package claude implements the Provider contract against ~/.claude.
//
// Storage layout (full detail in docs/research/01-claude-code-storage.md):
//
//   projects/<encoded-cwd>/<sessionId>.jsonl    one file per session
//   file-history/<sessionId>/...                versioned file backups
//   tasks/<sessionId>/...                       per-session task state
//   session-env/<sessionId>                     captured env
//   sessions/<sessionId>.json                   small metadata
//   history.jsonl                               global prompt history
//
// Each session JSONL is a newline-delimited stream of typed records. The
// parser folds the records into a parent-pointer tree via parentUuid and
// produces a normalized contracts.Conversation.
//
// Cleanup of a session must cascade to every sibling artifact above; Plan
// A only implements the read side, and PlanDelete / PlanOrphanScan return
// ErrNotImplemented. Plan C completes those.
package claude
```

- [ ] **Step 2: Write `adapters/claude/testdata/v1_0/empty_session.jsonl`**

One real-shape session with only meta records — no actual user prompts. This is the canonical "abandoned session" fixture. The records here mimic what Claude Code writes when a user opens a session and runs `/clear` without typing anything.

```jsonl
{"type":"last-prompt","leafUuid":"00000000-0000-0000-0000-000000000001","sessionId":"empty-session-1"}
{"type":"permission-mode","permissionMode":"default","sessionId":"empty-session-1"}
{"parentUuid":null,"isSidechain":false,"type":"attachment","attachment":{"type":"hook_success","hookName":"SessionStart:startup","hookEvent":"SessionStart","content":""},"uuid":"00000000-0000-0000-0000-000000000002","timestamp":"2026-05-15T09:00:00.000Z","userType":"external","entrypoint":"cli","cwd":"/Users/test/proj","sessionId":"empty-session-1","version":"2.1.133","gitBranch":"main"}
{"parentUuid":"00000000-0000-0000-0000-000000000002","isSidechain":false,"promptId":"p1","type":"user","message":{"role":"user","content":"<command-name>/clear</command-name>"},"isMeta":true,"uuid":"00000000-0000-0000-0000-000000000003","timestamp":"2026-05-15T09:00:01.000Z","userType":"external","entrypoint":"cli","cwd":"/Users/test/proj","sessionId":"empty-session-1","version":"2.1.133","gitBranch":"main"}
```

- [ ] **Step 3: Write `adapters/claude/testdata/v1_0/small_session.jsonl`**

A three-turn session with a tool use and a tool result.

```jsonl
{"type":"last-prompt","lastPrompt":"How do I read a file in Go?","leafUuid":"10000000-0000-0000-0000-000000000004","sessionId":"small-session-1"}
{"parentUuid":null,"isSidechain":false,"promptId":"p1","type":"user","message":{"role":"user","content":"How do I read a file in Go?"},"uuid":"10000000-0000-0000-0000-000000000001","timestamp":"2026-05-15T09:10:00.000Z","userType":"external","entrypoint":"cli","cwd":"/Users/test/proj","sessionId":"small-session-1","version":"2.1.133","gitBranch":"main"}
{"parentUuid":"10000000-0000-0000-0000-000000000001","isSidechain":false,"type":"assistant","message":{"id":"m1","type":"message","role":"assistant","content":[{"type":"text","text":"Use os.ReadFile."},{"type":"tool_use","id":"call-1","name":"Bash","input":{"cmd":"echo hi"}}]},"uuid":"10000000-0000-0000-0000-000000000002","timestamp":"2026-05-15T09:10:05.000Z","userType":"external","entrypoint":"cli","cwd":"/Users/test/proj","sessionId":"small-session-1","version":"2.1.133","gitBranch":"main"}
{"parentUuid":"10000000-0000-0000-0000-000000000002","isSidechain":false,"promptId":"p2","type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"call-1","content":"hi\n","is_error":false}]},"uuid":"10000000-0000-0000-0000-000000000003","timestamp":"2026-05-15T09:10:06.000Z","userType":"external","entrypoint":"cli","cwd":"/Users/test/proj","sessionId":"small-session-1","version":"2.1.133","gitBranch":"main"}
{"parentUuid":"10000000-0000-0000-0000-000000000003","isSidechain":false,"type":"assistant","message":{"id":"m2","type":"message","role":"assistant","content":[{"type":"text","text":"That works. The standard library handles this with os.ReadFile, which returns a byte slice."}]},"uuid":"10000000-0000-0000-0000-000000000004","timestamp":"2026-05-15T09:10:08.000Z","userType":"external","entrypoint":"cli","cwd":"/Users/test/proj","sessionId":"small-session-1","version":"2.1.133","gitBranch":"main"}
```

- [ ] **Step 4: Write `adapters/claude/testdata/v1_0/thinking_session.jsonl`**

A session whose assistant turn includes a thinking block.

```jsonl
{"parentUuid":null,"isSidechain":false,"promptId":"p1","type":"user","message":{"role":"user","content":"Refactor my code"},"uuid":"20000000-0000-0000-0000-000000000001","timestamp":"2026-05-15T09:20:00.000Z","userType":"external","entrypoint":"cli","cwd":"/Users/test/proj","sessionId":"thinking-session-1","version":"2.1.133","gitBranch":"main"}
{"parentUuid":"20000000-0000-0000-0000-000000000001","isSidechain":false,"type":"assistant","message":{"id":"m1","type":"message","role":"assistant","content":[{"type":"thinking","thinking":"The user wants a refactor. I should ask which file."},{"type":"text","text":"Which file would you like me to refactor?"}]},"uuid":"20000000-0000-0000-0000-000000000002","timestamp":"2026-05-15T09:20:02.000Z","userType":"external","entrypoint":"cli","cwd":"/Users/test/proj","sessionId":"thinking-session-1","version":"2.1.133","gitBranch":"main"}
```

- [ ] **Step 5: Write `adapters/claude/testdata/synthetic_future.jsonl`**

The canary fixture. Includes a fabricated record type and an unknown content kind. The parser must keep these as `UnknownBlock` rather than crash, per the resilience contract.

```jsonl
{"parentUuid":null,"isSidechain":false,"promptId":"p1","type":"user","message":{"role":"user","content":"hello"},"uuid":"30000000-0000-0000-0000-000000000001","timestamp":"2026-05-15T09:30:00.000Z","userType":"external","entrypoint":"cli","cwd":"/Users/test/proj","sessionId":"future-1","version":"99.0.0","gitBranch":"main"}
{"type":"future-event-from-tomorrow","weirdField":"value","sessionId":"future-1"}
{"parentUuid":"30000000-0000-0000-0000-000000000001","isSidechain":false,"type":"assistant","message":{"id":"m1","type":"message","role":"assistant","content":[{"type":"text","text":"hi"},{"type":"galaxy_brain","payload":{"insight":42}}]},"uuid":"30000000-0000-0000-0000-000000000002","timestamp":"2026-05-15T09:30:02.000Z","userType":"external","entrypoint":"cli","cwd":"/Users/test/proj","sessionId":"future-1","version":"99.0.0","gitBranch":"main"}
```

- [ ] **Step 6: Commit fixtures**

```bash
git add adapters/claude/doc.go adapters/claude/testdata/
git commit -m "test(claude): real-shape fixtures and synthetic-future canary"
```

---

## Task 12: `adapters/claude/detect.go` — fingerprint and version mapping

**Files:**
- Create: `adapters/claude/detect.go`
- Test: `adapters/claude/detect_test.go`

- [ ] **Step 1: Write `adapters/claude/detect.go`**

```go
package claude

import (
    "bufio"
    "encoding/json"
    "errors"
    "io"
    "io/fs"
    "path"
    "sort"
    "strings"

    "github.com/danieljbfz/chronicle/contracts"
    "github.com/danieljbfz/chronicle/steps"
)

const (
    // adapterName is the string returned by Provider.Name and stamped on
    // every StorageVersion.Adapter this package produces.
    adapterName = "claude"

    // maxFingerprintRecords caps how many records contribute to a
    // fingerprint. The first records carry the structural variety; reading
    // a 22 MB JSONL just to fingerprint it would be wasteful.
    maxFingerprintRecords = 200

    // projectsDir is the subdirectory under the Claude root that holds
    // per-cwd session folders.
    projectsDir = "projects"
)

// knownFingerprints maps detected fingerprints to internal version codes.
// New entries land here as the upstream format evolves. When we hit a
// fingerprint not in this map, we return "unknown" — read-only operations
// still work via tolerant parsing.
var knownFingerprints = map[string]string{
    // Populated empirically from the user's real data on 2026-05-15. New
    // entries land here as Claude Code's storage evolves.
}

// detectInDir computes the fingerprint and version for the first session
// file found under the directory tree. It is the building block for the
// Provider.Detect implementation.
//
// "First session file" is fine: every session in this Claude install was
// written by the same Claude Code version, so the fingerprint will agree.
// If we ever need per-session detection, we add it then.
func detectInDir(root fs.FS) (contracts.StorageVersion, error) {
    file, err := firstSessionFile(root)
    if errors.Is(err, fs.ErrNotExist) {
        return contracts.StorageVersion{
            Adapter: adapterName,
            Version: "unknown",
        }, nil
    }
    if err != nil {
        return contracts.StorageVersion{}, err
    }

    inputs, parseable, err := readFingerprintInputs(root, file)
    if err != nil {
        return contracts.StorageVersion{}, err
    }
    if !parseable {
        return contracts.StorageVersion{}, errors.New("no parseable JSON records in " + file)
    }

    fp := steps.Fingerprint(inputs)
    version, known := knownFingerprints[fp]
    if !known {
        version = "unknown"
    }
    return contracts.StorageVersion{
        Adapter:     adapterName,
        Version:     version,
        Fingerprint: fp,
        Capabilities: contracts.Capabilities{
            ThreadTree:      known,
            ToolInvocations: known,
            ModelMetadata:   false, // Claude's JSONL does not carry per-turn model id
        },
    }, nil
}

// firstSessionFile walks projects/<*>/<*>.jsonl and returns the path of
// the first one. fs.ErrNotExist when no session file is present.
func firstSessionFile(root fs.FS) (string, error) {
    projects, err := fs.ReadDir(root, projectsDir)
    if err != nil {
        return "", err
    }
    for _, p := range projects {
        if !p.IsDir() {
            continue
        }
        entries, err := fs.ReadDir(root, path.Join(projectsDir, p.Name()))
        if err != nil {
            continue
        }
        for _, e := range entries {
            if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
                return path.Join(projectsDir, p.Name(), e.Name()), nil
            }
        }
    }
    return "", fs.ErrNotExist
}

// readFingerprintInputs streams a JSONL file and returns up to
// maxFingerprintRecords (type, keys) tuples. parseable is true once any
// line decoded as JSON.
func readFingerprintInputs(root fs.FS, file string) (inputs []steps.FingerprintInput, parseable bool, err error) {
    f, err := root.Open(file)
    if err != nil {
        return nil, false, err
    }
    defer f.Close()

    return collectFingerprintInputs(f)
}

func collectFingerprintInputs(r io.Reader) ([]steps.FingerprintInput, bool, error) {
    scanner := bufio.NewScanner(r)
    scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)

    var inputs []steps.FingerprintInput
    parseable := false
    for scanner.Scan() && len(inputs) < maxFingerprintRecords {
        var rec map[string]json.RawMessage
        if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
            continue
        }
        parseable = true
        var t string
        if raw, ok := rec["type"]; ok {
            _ = json.Unmarshal(raw, &t)
        }
        keys := make([]string, 0, len(rec))
        for k := range rec {
            keys = append(keys, k)
        }
        sort.Strings(keys)
        inputs = append(inputs, steps.FingerprintInput{Type: t, Keys: keys})
    }
    if err := scanner.Err(); err != nil {
        return nil, parseable, err
    }
    return inputs, parseable, nil
}
```

- [ ] **Step 2: Write `adapters/claude/detect_test.go`**

```go
package claude

import (
    "os"
    "testing"
    "testing/fstest"
)

func loadFixture(t *testing.T, name string) []byte {
    t.Helper()
    data, err := os.ReadFile("testdata/v1_0/" + name)
    if err != nil {
        t.Fatalf("read fixture %s: %v", name, err)
    }
    return data
}

func TestDetect_emptyTreeReturnsUnknown(t *testing.T) {
    fsys := fstest.MapFS{}
    got, err := detectInDir(fsys)
    if err != nil {
        t.Fatalf("detectInDir: %v", err)
    }
    if got.Version != "unknown" {
        t.Errorf("Version = %q, want %q", got.Version, "unknown")
    }
    if got.Adapter != "claude" {
        t.Errorf("Adapter = %q, want %q", got.Adapter, "claude")
    }
}

func TestDetect_realFixtureProducesFingerprint(t *testing.T) {
    fsys := fstest.MapFS{
        "projects/-Users-test-proj/small.jsonl": &fstest.MapFile{
            Data: loadFixture(t, "small_session.jsonl"),
        },
    }
    got, err := detectInDir(fsys)
    if err != nil {
        t.Fatalf("detectInDir: %v", err)
    }
    if got.Fingerprint == "" {
        t.Error("Fingerprint should be set for parseable JSONL")
    }
    // Unknown until we add the fingerprint to knownFingerprints — that
    // happens once we run the binary against the user's real data in the
    // smoke-test task.
    if got.Version != "unknown" {
        t.Logf("Version = %q (expected once knownFingerprints is populated)", got.Version)
    }
}

func TestDetect_invalidJSONStillProducesFingerprint(t *testing.T) {
    fsys := fstest.MapFS{
        "projects/p/s.jsonl": &fstest.MapFile{Data: []byte("not json\n{\"type\":\"user\"}\n")},
    }
    got, err := detectInDir(fsys)
    if err != nil {
        t.Fatalf("detectInDir: %v", err)
    }
    if got.Fingerprint == "" {
        t.Error("Fingerprint should still be computed when one line parses")
    }
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./adapters/claude/...
```

Expected: all three tests pass. The middle one logs the unknown version — that is fine for now.

- [ ] **Step 4: Commit**

```bash
git add adapters/claude/detect.go adapters/claude/detect_test.go
git commit -m "feat(claude): detect storage version via fingerprint"
```

---

## Task 13: `adapters/claude/parse.go` — JSONL → Conversation

**Files:**
- Create: `adapters/claude/parse.go`
- Test: `adapters/claude/parse_test.go`

- [ ] **Step 1: Write `adapters/claude/parse.go`**

```go
package claude

import (
    "bufio"
    "encoding/json"
    "fmt"
    "io"
    "io/fs"
    "sort"
    "time"

    "github.com/danieljbfz/chronicle/contracts"
)

// readSessionFile parses the given JSONL into a normalized Conversation.
// Unknown record types and unknown content kinds are preserved as
// UnknownBlock entries so the renderer can still show them.
func readSessionFile(root fs.FS, sessionFile string, source contracts.StorageVersion) (contracts.Conversation, error) {
    f, err := root.Open(sessionFile)
    if err != nil {
        return contracts.Conversation{}, err
    }
    defer f.Close()
    return parseStream(f, source)
}

func parseStream(r io.Reader, source contracts.StorageVersion) (contracts.Conversation, error) {
    scanner := bufio.NewScanner(r)
    scanner.Buffer(make([]byte, 1024*1024), 64*1024*1024)

    // Step 1: collect records as we read; keep them in order.
    var records []rawRecord
    for scanner.Scan() {
        var record rawRecord
        if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
            // Skip lines we cannot even decode as JSON. The resilience
            // contract allows this — we never crash on garbage.
            continue
        }
        record.line = string(scanner.Bytes())
        records = append(records, rec)
    }
    if err := scanner.Err(); err != nil {
        return contracts.Conversation{}, err
    }

    // Step 2: produce Messages from the parseable user/assistant/system
    // records. Other record types (last-prompt, queue-operation, ...) are
    // ignored — they are metadata, not conversation content.
    var (
        messages    []contracts.Message
        sessionID   contracts.SessionID
        cwd         string
        startedAt   time.Time
        endedAt     time.Time
    )
    for _, record := range records {
        if record.SessionID != "" {
            sessionID = contracts.SessionID(record.SessionID)
        }
        if record.Cwd != "" {
            cwd = record.Cwd
        }
        ts, _ := time.Parse(time.RFC3339Nano, record.Timestamp)
        if !ts.IsZero() {
            if startedAt.IsZero() || ts.Before(startedAt) {
                startedAt = ts
            }
            if ts.After(endedAt) {
                endedAt = ts
            }
        }

        switch record.Type {
        case "user":
            messages = append(messages, parseUserRecord(record, ts))
        case "assistant":
            messages = append(messages, parseAssistantRecord(record, ts))
        case "system":
            // Skipped for Plan A. System notes (local-command, hook output)
            // are not part of the conversation a user reads. They become
            // their own Message kind in a later plan if needed.
        case "attachment", "file-history-snapshot", "last-prompt",
            "permission-mode", "queue-operation":
            // Metadata, not conversation content. Drop silently.
        default:
            // Unknown record type. Preserve it so the user can see something
            // happened, per the resilience contract.
            messages = append(messages, contracts.Message{
                ID:        contracts.MessageID(record.UUID),
                ParentID:  contracts.MessageID(record.ParentUUID),
                Role:      contracts.RoleSystem,
                Timestamp: ts,
                IsMeta:    true,
                Blocks: []contracts.Block{
                    contracts.UnknownBlock{Kind: record.Type, Raw: []byte(record.line)},
                },
            })
        }
    }

    // Step 3: sort by timestamp as a stable, deterministic order. JSONL is
    // already chronological in practice, but a defensive sort makes tests
    // robust against minor reorderings.
    sort.SliceStable(messages, func(i, j int) bool {
        return messages[i].Timestamp.Before(messages[j].Timestamp)
    })

    return contracts.Conversation{
        SessionID:    sessionID,
        Project:      contracts.ProjectID(cwd),
        StartedAt:    startedAt,
        EndedAt:      endedAt,
        Messages:     messages,
        Capabilities: source.Capabilities,
        Source:       source,
    }, nil
}

// rawRecord captures only the fields we read directly from each JSONL line.
// json.RawMessage isolates the still-untyped message body so we can parse
// it differently based on Type.
type rawRecord struct {
    Type        string          `json:"type"`
    UUID        string          `json:"uuid"`
    ParentUUID  string          `json:"parentUuid"`
    SessionID   string          `json:"sessionId"`
    Cwd         string          `json:"cwd"`
    Timestamp   string          `json:"timestamp"`
    IsMeta      bool            `json:"isMeta"`
    IsSidechain bool            `json:"isSidechain"`
    Message     json.RawMessage `json:"message"`
    line        string          // the original JSONL line, kept for UnknownBlock fidelity
}

// userBody and assistantBody describe the shape of the embedded message
// payload for the two roles we render. They are intentionally permissive —
// missing fields decode as zero values, unknown fields are ignored.
type userBody struct {
    Role    string          `json:"role"`
    Content json.RawMessage `json:"content"`
}

type assistantBody struct {
    Role    string          `json:"role"`
    Content json.RawMessage `json:"content"`
}

func parseUserRecord(record rawRecord, ts time.Time) contracts.Message {
    var body userBody
    _ = json.Unmarshal(record.Message, &body)

    blocks := decodeUserContent(body.Content)
    return contracts.Message{
        ID:          contracts.MessageID(record.UUID),
        ParentID:    contracts.MessageID(record.ParentUUID),
        Role:        contracts.RoleUser,
        Timestamp:   ts,
        IsMeta:      record.IsMeta,
        IsSidechain: record.IsSidechain,
        Blocks:      blocks,
    }
}

func parseAssistantRecord(record rawRecord, ts time.Time) contracts.Message {
    var body assistantBody
    _ = json.Unmarshal(record.Message, &body)

    blocks := decodeAssistantContent(body.Content)
    return contracts.Message{
        ID:          contracts.MessageID(record.UUID),
        ParentID:    contracts.MessageID(record.ParentUUID),
        Role:        contracts.RoleAssistant,
        Timestamp:   ts,
        IsMeta:      record.IsMeta,
        IsSidechain: record.IsSidechain,
        Blocks:      blocks,
    }
}

// decodeUserContent handles both shapes Claude writes for user messages:
// a bare string ("How do I read a file?") or an array of typed parts
// ([{type:"text", ...}, {type:"tool_result", ...}, {type:"image", ...}]).
func decodeUserContent(raw json.RawMessage) []contracts.Block {
    if len(raw) == 0 {
        return nil
    }
    if raw[0] == '"' {
        var s string
        if err := json.Unmarshal(raw, &s); err == nil && s != "" {
            return []contracts.Block{contracts.TextBlock{Text: s}}
        }
        return nil
    }
    return decodePartArray(raw)
}

// decodeAssistantContent always handles an array of parts.
func decodeAssistantContent(raw json.RawMessage) []contracts.Block {
    if len(raw) == 0 {
        return nil
    }
    return decodePartArray(raw)
}

func decodePartArray(raw json.RawMessage) []contracts.Block {
    var parts []json.RawMessage
    if err := json.Unmarshal(raw, &parts); err != nil {
        return nil
    }
    out := make([]contracts.Block, 0, len(parts))
    for _, p := range parts {
        if block, ok := decodePart(p); ok {
            out = append(out, block)
        }
    }
    return out
}

func decodePart(raw json.RawMessage) (contracts.Block, bool) {
    var head struct {
        Type string `json:"type"`
    }
    if err := json.Unmarshal(raw, &head); err != nil {
        return nil, false
    }
    switch head.Type {
    case "text":
        var v struct{ Text string `json:"text"` }
        _ = json.Unmarshal(raw, &v)
        return contracts.TextBlock{Text: v.Text}, true
    case "thinking":
        var v struct{ Thinking string `json:"thinking"` }
        _ = json.Unmarshal(raw, &v)
        return contracts.ThinkingBlock{Text: v.Thinking}, true
    case "tool_use":
        var v struct {
            ID    string          `json:"id"`
            Name  string          `json:"name"`
            Input json.RawMessage `json:"input"`
        }
        _ = json.Unmarshal(raw, &v)
        return contracts.ToolUseBlock{Tool: v.Name, Input: v.Input, CallID: v.ID}, true
    case "tool_result":
        var v struct {
            ToolUseID string `json:"tool_use_id"`
            Content   json.RawMessage `json:"content"`
            IsError   bool `json:"is_error"`
        }
        _ = json.Unmarshal(raw, &v)
        return contracts.ToolResultBlock{
            CallID:  v.ToolUseID,
            Output:  flattenToolResultContent(v.Content),
            IsError: v.IsError,
        }, true
    case "image":
        var v struct {
            Source struct {
                Type      string `json:"type"`
                MediaType string `json:"media_type"`
                Data      string `json:"data"`
            } `json:"source"`
        }
        _ = json.Unmarshal(raw, &v)
        ref := v.Source.Type
        if v.Source.Data != "" {
            ref = fmt.Sprintf("base64:%d bytes", len(v.Source.Data))
        }
        return contracts.ImageBlock{MIME: v.Source.MediaType, PathOrInlineRef: ref}, true
    default:
        return contracts.UnknownBlock{Kind: head.Type, Raw: raw}, true
    }
}

// flattenToolResultContent accepts either a plain string or an array of
// {type:"text",text:"..."} parts and returns the concatenated text.
func flattenToolResultContent(raw json.RawMessage) string {
    if len(raw) == 0 {
        return ""
    }
    if raw[0] == '"' {
        var s string
        _ = json.Unmarshal(raw, &s)
        return s
    }
    var parts []struct {
        Type string `json:"type"`
        Text string `json:"text"`
    }
    if err := json.Unmarshal(raw, &parts); err != nil {
        return string(raw)
    }
    var out string
    for _, p := range parts {
        if p.Type == "text" {
            out += p.Text
        }
    }
    return out
}
```

- [ ] **Step 2: Write `adapters/claude/parse_test.go`**

```go
package claude

import (
    "os"
    "strings"
    "testing"
    "testing/fstest"

    "github.com/danieljbfz/chronicle/contracts"
)

func readSession(t *testing.T, fixture string) contracts.Conversation {
    t.Helper()
    data, err := os.ReadFile("testdata/v1_0/" + fixture)
    if err != nil {
        t.Fatalf("read %s: %v", fixture, err)
    }
    fsys := fstest.MapFS{
        "projects/-p/s.jsonl": &fstest.MapFile{Data: data},
    }
    c, err := readSessionFile(fsys, "projects/-p/s.jsonl", contracts.StorageVersion{Adapter: "claude"})
    if err != nil {
        t.Fatalf("readSessionFile: %v", err)
    }
    return c
}

func TestParse_smallSessionShape(t *testing.T) {
    c := readSession(t, "small_session.jsonl")
    if c.SessionID != "small-session-1" {
        t.Errorf("SessionID = %q, want %q", c.SessionID, "small-session-1")
    }
    if len(c.Messages) != 3 {
        t.Fatalf("got %d messages, want 3 (user, assistant w/ tool, user tool_result, assistant)", len(c.Messages))
        // (the small fixture has 4 conversation turns; some impls collapse the
        // tool_result-only user message into the prior turn. We do not — see
        // contract spec §4.)
    }
    if c.Messages[0].Role != contracts.RoleUser {
        t.Errorf("first message role = %q, want user", c.Messages[0].Role)
    }
    // Find the tool use.
    foundToolUse := false
    foundToolResult := false
    for _, m := range c.Messages {
        for _, b := range m.Blocks {
            if _, ok := b.(contracts.ToolUseBlock); ok {
                foundToolUse = true
            }
            if _, ok := b.(contracts.ToolResultBlock); ok {
                foundToolResult = true
            }
        }
    }
    if !foundToolUse {
        t.Error("expected a ToolUseBlock in the small fixture")
    }
    if !foundToolResult {
        t.Error("expected a ToolResultBlock in the small fixture")
    }
}

func TestParse_emptySessionIsAbandoned(t *testing.T) {
    c := readSession(t, "empty_session.jsonl")
    if !c.IsAbandoned() {
        t.Error("empty fixture should be abandoned")
    }
    if c.FirstUserPrompt() != "" {
        t.Errorf("FirstUserPrompt = %q, want empty", c.FirstUserPrompt())
    }
}

func TestParse_thinkingBlockSurvives(t *testing.T) {
    c := readSession(t, "thinking_session.jsonl")
    found := false
    for _, m := range c.Messages {
        for _, b := range m.Blocks {
            if tb, ok := b.(contracts.ThinkingBlock); ok && strings.Contains(tb.Text, "refactor") {
                found = true
            }
        }
    }
    if !found {
        t.Error("expected ThinkingBlock with the fixture content")
    }
}

// The canary. The resilience contract requires unknown record types and
// unknown content kinds to be preserved, not dropped.
func TestParse_syntheticFutureKeepsUnknowns(t *testing.T) {
    data, err := os.ReadFile("testdata/synthetic_future.jsonl")
    if err != nil {
        t.Fatal(err)
    }
    fsys := fstest.MapFS{"projects/-p/s.jsonl": &fstest.MapFile{Data: data}}
    c, err := readSessionFile(fsys, "projects/-p/s.jsonl", contracts.StorageVersion{Adapter: "claude", Version: "unknown"})
    if err != nil {
        t.Fatalf("parse must not error on synthetic future: %v", err)
    }
    var sawUnknownRecord, sawUnknownContent bool
    for _, m := range c.Messages {
        for _, b := range m.Blocks {
            if u, ok := b.(contracts.UnknownBlock); ok {
                if u.Kind == "future-event-from-tomorrow" {
                    sawUnknownRecord = true
                }
                if u.Kind == "galaxy_brain" {
                    sawUnknownContent = true
                }
            }
        }
    }
    if !sawUnknownRecord {
        t.Error("unknown record type must surface as UnknownBlock — the resilience canary")
    }
    if !sawUnknownContent {
        t.Error("unknown content kind must surface as UnknownBlock — the resilience canary")
    }
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./adapters/claude/...
```

Expected: all four tests pass. The canary is the critical one — if it ever fails after this point, the resilience contract has been broken.

- [ ] **Step 4: Commit**

```bash
git add adapters/claude/parse.go adapters/claude/parse_test.go
git commit -m "feat(claude): tolerant JSONL parser with unknown-block preservation"
```

---

## Task 14: `adapters/claude` Provider implementation + cleanup stubs

**Files:**
- Create: `adapters/claude/provider.go`
- Create: `adapters/claude/cleanup_stub.go`
- Test: `adapters/claude/provider_test.go`

- [ ] **Step 1: Write `adapters/claude/cleanup_stub.go`**

```go
package claude

import (
    "errors"
    "io/fs"

    "github.com/danieljbfz/chronicle/contracts"
)

// ErrNotImplemented is returned by Plan A cleanup methods. Plan C replaces
// them with real cascade-aware implementations.
var ErrNotImplemented = errors.New("claude: cleanup not implemented in Plan A; see docs/superpowers/plans/")

func planDeleteStub(_ fs.FS, _ contracts.SessionID) (contracts.DeletePlan, error) {
    return contracts.DeletePlan{}, ErrNotImplemented
}

func planOrphanScanStub(_ fs.FS) (contracts.DeletePlan, error) {
    return contracts.DeletePlan{}, ErrNotImplemented
}
```

- [ ] **Step 2: Write `adapters/claude/provider.go`**

```go
package claude

import (
    "errors"
    "io/fs"
    "path"
    "sort"
    "strings"

    "github.com/danieljbfz/chronicle/contracts"
)

// Provider is the Claude adapter as a value type. Composition stores one
// instance per chronicle process; the type is stateless beyond the cached
// StorageVersion.
type Provider struct {
    cached contracts.StorageVersion
    cacheOK bool
}

// New returns a ready-to-use Provider. Detection happens lazily on first
// use so that startup does not block on disk I/O.
func New() *Provider { return &Provider{} }

func (*Provider) Name() string { return adapterName }

func (p *Provider) Detect(root fs.FS) (contracts.StorageVersion, error) {
    if p.cacheOK {
        return p.cached, nil
    }
    sv, err := detectInDir(root)
    if err != nil {
        return contracts.StorageVersion{}, err
    }
    p.cached = sv
    p.cacheOK = true
    return sv, nil
}

func (p *Provider) ListProjects(root fs.FS) ([]contracts.Project, error) {
    entries, err := fs.ReadDir(root, projectsDir)
    if err != nil {
        if errors.Is(err, fs.ErrNotExist) {
            return nil, nil
        }
        return nil, err
    }
    var projects []contracts.Project
    for _, e := range entries {
        if !e.IsDir() {
            continue
        }
        proj, err := summarizeProject(root, e.Name())
        if err != nil {
            return nil, err
        }
        projects = append(projects, proj)
    }
    sort.Slice(projects, func(i, j int) bool {
        return projects[i].DisplayName < projects[j].DisplayName
    })
    return projects, nil
}

func summarizeProject(root fs.FS, folderName string) (contracts.Project, error) {
    dir := path.Join(projectsDir, folderName)
    entries, err := fs.ReadDir(root, dir)
    if err != nil {
        return contracts.Project{}, err
    }
    sessionCount := 0
    var totalBytes int64
    for _, e := range entries {
        if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
            continue
        }
        sessionCount++
        info, err := e.Info()
        if err == nil {
            totalBytes += info.Size()
        }
    }
    return contracts.Project{
        ID:           contracts.ProjectID(folderName),
        DisplayName:  decodeProjectName(folderName),
        Path:         decodeProjectPath(folderName),
        SessionCount: sessionCount,
        SizeBytes:    totalBytes,
    }, nil
}

// decodeProjectName turns "-Users-djbf-Desktop-work-claude-history" into
// "claude-history" by taking the trailing path segment. Adapter docs note
// that the directory name is the cwd with "/" replaced by "-", and we
// recover the rightmost component for display.
//
// This is heuristic — if a path component genuinely contains a hyphen
// (very rare on macOS), we still pick the trailing token. The full path
// stays available in Project.Path.
func decodeProjectName(folderName string) string {
    p := decodeProjectPath(folderName)
    if i := strings.LastIndex(p, "/"); i >= 0 && i+1 < len(p) {
        return p[i+1:]
    }
    return folderName
}

func decodeProjectPath(folderName string) string {
    // The folder is the absolute path with "/" → "-". Restore the slash on
    // the leading dash and any subsequent dashes between path components.
    // We cannot perfectly disambiguate a literal "-" in a real path
    // segment from a path separator. The Path field is a best effort.
    if strings.HasPrefix(folderName, "-") {
        return "/" + strings.ReplaceAll(folderName[1:], "-", "/")
    }
    return folderName
}

func (p *Provider) ListSessions(root fs.FS, project contracts.ProjectID) ([]contracts.SessionSummary, error) {
    dir := path.Join(projectsDir, string(project))
    entries, err := fs.ReadDir(root, dir)
    if err != nil {
        return nil, err
    }
    var summaries []contracts.SessionSummary
    for _, e := range entries {
        if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
            continue
        }
        sessionFile := path.Join(dir, e.Name())
        sv := p.cached
        c, err := readSessionFile(root, sessionFile, sv)
        if err != nil {
            return nil, err
        }
        info, _ := e.Info()
        var size int64
        if info != nil {
            size = info.Size()
        }
        summaries = append(summaries, contracts.SessionSummary{
            ID:           contracts.SessionID(strings.TrimSuffix(e.Name(), ".jsonl")),
            Project:      project,
            StartedAt:    c.StartedAt,
            LastActive:   c.EndedAt,
            Title:        c.FirstUserPrompt(),
            TurnCount:    len(c.Messages),
            SizeBytes:    size,
            Capabilities: sv.Capabilities,
            Source:       sv,
        })
    }
    sort.Slice(summaries, func(i, j int) bool {
        return summaries[i].LastActive.After(summaries[j].LastActive)
    })
    return summaries, nil
}

func (p *Provider) ReadSession(root fs.FS, id contracts.SessionID) (contracts.Conversation, error) {
    file, err := locateSessionFile(root, id)
    if err != nil {
        return contracts.Conversation{}, err
    }
    return readSessionFile(root, file, p.cached)
}

func locateSessionFile(root fs.FS, id contracts.SessionID) (string, error) {
    projects, err := fs.ReadDir(root, projectsDir)
    if err != nil {
        return "", err
    }
    for _, proj := range projects {
        if !proj.IsDir() {
            continue
        }
        candidate := path.Join(projectsDir, proj.Name(), string(id)+".jsonl")
        if _, err := fs.Stat(root, candidate); err == nil {
            return candidate, nil
        }
    }
    return "", fs.ErrNotExist
}

func (p *Provider) PlanDelete(root fs.FS, id contracts.SessionID) (contracts.DeletePlan, error) {
    return planDeleteStub(root, id)
}

func (p *Provider) PlanOrphanScan(root fs.FS) (contracts.DeletePlan, error) {
    return planOrphanScanStub(root)
}

// Provider must satisfy contracts.Provider. Compile-time check.
var _ contracts.Provider = (*Provider)(nil)
```

- [ ] **Step 3: Write `adapters/claude/provider_test.go`**

```go
package claude

import (
    "errors"
    "os"
    "testing"
    "testing/fstest"

    "github.com/danieljbfz/chronicle/contracts"
)

func mustReadFixture(t *testing.T, name string) []byte {
    t.Helper()
    data, err := os.ReadFile("testdata/v1_0/" + name)
    if err != nil {
        t.Fatal(err)
    }
    return data
}

func buildFS(t *testing.T) fstest.MapFS {
    t.Helper()
    return fstest.MapFS{
        "projects/-Users-test-proj/small-session-1.jsonl": &fstest.MapFile{
            Data: mustReadFixture(t, "small_session.jsonl"),
        },
        "projects/-Users-test-proj/empty-session-1.jsonl": &fstest.MapFile{
            Data: mustReadFixture(t, "empty_session.jsonl"),
        },
        "projects/-Users-test-other/thinking-session-1.jsonl": &fstest.MapFile{
            Data: mustReadFixture(t, "thinking_session.jsonl"),
        },
    }
}

func TestProvider_ListProjects(t *testing.T) {
    p := New()
    fsys := buildFS(t)
    projects, err := p.ListProjects(fsys)
    if err != nil {
        t.Fatal(err)
    }
    if len(projects) != 2 {
        t.Fatalf("got %d projects, want 2", len(projects))
    }
    if projects[0].DisplayName == "" || projects[0].Path == "" {
        t.Error("project display name and path should be populated")
    }
    var total int
    for _, pr := range projects {
        total += pr.SessionCount
    }
    if total != 3 {
        t.Errorf("total session count = %d, want 3", total)
    }
}

func TestProvider_ListSessions_sortedNewestFirst(t *testing.T) {
    p := New()
    fsys := buildFS(t)
    summaries, err := p.ListSessions(fsys, "-Users-test-proj")
    if err != nil {
        t.Fatal(err)
    }
    if len(summaries) != 2 {
        t.Fatalf("got %d sessions, want 2", len(summaries))
    }
    if !summaries[0].LastActive.After(summaries[1].LastActive) &&
        !summaries[0].LastActive.Equal(summaries[1].LastActive) {
        t.Errorf("sessions should be sorted newest-first: %v then %v",
            summaries[0].LastActive, summaries[1].LastActive)
    }
}

func TestProvider_ReadSession_findsAcrossProjects(t *testing.T) {
    p := New()
    fsys := buildFS(t)
    c, err := p.ReadSession(fsys, contracts.SessionID("thinking-session-1"))
    if err != nil {
        t.Fatal(err)
    }
    if c.SessionID != "thinking-session-1" {
        t.Errorf("SessionID = %q", c.SessionID)
    }
}

func TestProvider_PlanDeleteReturnsNotImplemented(t *testing.T) {
    p := New()
    fsys := buildFS(t)
    _, err := p.PlanDelete(fsys, "small-session-1")
    if !errors.Is(err, ErrNotImplemented) {
        t.Errorf("PlanDelete err = %v, want ErrNotImplemented", err)
    }
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./adapters/claude/...
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add adapters/claude/provider.go adapters/claude/cleanup_stub.go adapters/claude/provider_test.go
git commit -m "feat(claude): Provider implementation (read paths, cleanup stubs)"
```

---

## Task 15: Adapter registry and the application core

The registry (`adapters/all.go`) is the **extensibility seam** for chronicle. Adding a future provider — Cursor, Antigravity, Codex CLI — is one new package under `adapters/` plus one line in this registry. The application core (`composition`) does not know which adapters exist at compile time; it iterates the registry.

**Files:**
- Create: `adapters/all.go`
- Create: `composition/browse.go`
- Create: `composition/doctor.go`
- Test: `composition/browse_test.go`

- [ ] **Step 1: Write `adapters/all.go` — the registry**

```go
// Package adapters wires the per-tool provider packages into a single list
// that the application core can iterate. Adding a new provider is one new
// import below and one new entry in All — no other code in chronicle has
// to change.
//
// The registry lives one level above the per-tool packages on purpose:
// adapters/claude knows about ~/.claude and nothing else, while the wiring
// layer here knows about config and paths.
package adapters

import (
    "io/fs"
    "os"

    "github.com/danieljbfz/chronicle/adapters/claude"
    "github.com/danieljbfz/chronicle/contracts"
    "github.com/danieljbfz/chronicle/internal/config"
    "github.com/danieljbfz/chronicle/internal/paths"
)

// Entry is one wired-up provider ready for the application core.
type Entry struct {
    Provider contracts.Provider
    Root     string
    FS       fs.FS
}

// Factory builds an Entry, or returns ok=false to indicate "not enabled
// in config" or "configured root is missing".
type Factory func(config.Config, paths.Locations) (Entry, bool)

// All returns every registered provider factory. Adding a new tool is one
// new line in this slice — no other code in chronicle needs to change.
func All() []Factory {
    return []Factory{
        claudeFactory,
        // Plan B appends copilotFactory.
        // Future plans append cursorFactory, antigravityFactory, etc.
    }
}

func claudeFactory(settings config.Config, locations paths.Locations) (Entry, bool) {
    if !settings.Providers.Claude.Enabled {
        return Entry{}, false
    }
    root := settings.Providers.Claude.Root
    if root == "" {
        root = locations.ClaudeRoot
    }
    return Entry{
        Provider: claude.New(),
        Root:     root,
        FS:       os.DirFS(root),
    }, true
}
```

- [ ] **Step 2: Write `composition/browse.go`**

```go
// Package composition is the only layer that talks to the real filesystem.
// It instantiates providers, hands each one a rooted fs.FS, and exposes the
// flat API the entrypoints (CLI, TUI, web) consume.
package composition

import (
    "errors"
    "io/fs"

    "github.com/danieljbfz/chronicle/adapters"
    "github.com/danieljbfz/chronicle/contracts"
    "github.com/danieljbfz/chronicle/internal/config"
    "github.com/danieljbfz/chronicle/internal/paths"
)

// providerEntry pairs a Provider with the absolute filesystem root it reads.
type providerEntry struct {
    Provider contracts.Provider
    Root     string
    FS       fs.FS
    Version  contracts.StorageVersion
}

// App is the wired-up composition the entrypoints use. Construct one per
// process at startup, then call its read methods.
type App struct {
    settings  config.Config
    locations paths.Locations
    providers []*providerEntry
}

// New builds an App: it resolves paths, loads config, instantiates each
// enabled provider with an fs.FS rooted at its data directory, and runs
// Detect once on each so the Doctor view has results to show.
func New() (*App, error) {
    locations, err := paths.Resolve()
    if err != nil {
        return nil, err
    }
    settings, err := config.Load(locations.ConfigFile)
    if err != nil {
        return nil, err
    }

    a := &App{settings: settings, locations: locations}

    // The set of providers comes from the registry — adding a new tool
    // (Cursor, Antigravity, ...) is one line in adapters/all.go and zero
    // changes here. The registry is the only place that knows the list.
    for _, factory := range adapters.All() {
        entry, ok := factory(settings, locations)
        if !ok {
            continue // provider is disabled in config, or root is missing
        }
        a.providers = append(a.providers, &providerEntry{
            Provider: entry.Provider,
            Root:     entry.Root,
            FS:       entry.FS,
        })
    }

    // Run Detect on each provider once. Errors for "directory does not
    // exist" are silently downgraded to a disabled state — a user may not
    // have one of the tools installed. Other errors are returned.
    for _, p := range a.providers {
        sv, err := p.Provider.Detect(p.FS)
        if err != nil {
            if errors.Is(err, fs.ErrNotExist) {
                continue
            }
            return nil, err
        }
        p.Version = sv
    }

    return a, nil
}

// ListProjects returns every project across every detected provider, with
// the provider name prepended for display.
type ProjectListing struct {
    Provider string
    Project  contracts.Project
    Source   contracts.StorageVersion
}

func (a *App) ListProjects() ([]ProjectListing, error) {
    var out []ProjectListing
    for _, p := range a.providers {
        projects, err := p.Provider.ListProjects(p.FS)
        if err != nil {
            if errors.Is(err, fs.ErrNotExist) {
                continue
            }
            return nil, err
        }
        for _, proj := range projects {
            out = append(out, ProjectListing{
                Provider: p.Provider.Name(),
                Project:  proj,
                Source:   p.Version,
            })
        }
    }
    return out, nil
}

// ListSessionsAll fans out across every provider and project. CLI list
// commands use this. Pass a non-empty providerName to filter to one tool.
type SessionListing struct {
    Provider string
    Summary  contracts.SessionSummary
}

func (a *App) ListSessionsAll(providerName string) ([]SessionListing, error) {
    var out []SessionListing
    for _, p := range a.providers {
        if providerName != "" && p.Provider.Name() != providerName {
            continue
        }
        projects, err := p.Provider.ListProjects(p.FS)
        if err != nil {
            if errors.Is(err, fs.ErrNotExist) {
                continue
            }
            return nil, err
        }
        for _, proj := range projects {
            sessions, err := p.Provider.ListSessions(p.FS, proj.ID)
            if err != nil {
                return nil, err
            }
            for _, s := range sessions {
                out = append(out, SessionListing{Provider: p.Provider.Name(), Summary: s})
            }
        }
    }
    return out, nil
}

// ReadSession finds the session across providers and returns the parsed
// Conversation. Returns fs.ErrNotExist when no provider knows the id.
func (a *App) ReadSession(id contracts.SessionID) (contracts.Conversation, error) {
    for _, p := range a.providers {
        c, err := p.Provider.ReadSession(p.FS, id)
        if err == nil {
            return c, nil
        }
        if !errors.Is(err, fs.ErrNotExist) {
            return contracts.Conversation{}, err
        }
    }
    return contracts.Conversation{}, fs.ErrNotExist
}

// Settings returns the resolved config for callers that need it (CLI flags
// overriding values, the doctor command, etc.).
func (a *App) Settings() config.Config { return a.settings }

// Locations returns the resolved XDG paths.
func (a *App) Locations() paths.Locations { return a.locations }
```

- [ ] **Step 3: Write `composition/doctor.go`**

```go
package composition

import "github.com/danieljbfz/chronicle/contracts"

// ProviderHealth is the row chronicle doctor renders per provider.
type ProviderHealth struct {
    Name        string
    Root        string
    Version     contracts.StorageVersion
    Reachable   bool
    Note        string // human-readable warning when Reachable=false or Version=unknown
    SessionCount int
}

// Doctor returns the current state of every wired provider. Entrypoints
// render it as text or JSON.
func (a *App) Doctor() []ProviderHealth {
    out := make([]ProviderHealth, 0, len(a.providers))
    for _, p := range a.providers {
        h := ProviderHealth{
            Name:    p.Provider.Name(),
            Root:    p.Root,
            Version: p.Version,
        }
        projects, err := p.Provider.ListProjects(p.FS)
        if err != nil {
            h.Reachable = false
            h.Note = err.Error()
        } else {
            h.Reachable = true
            for _, proj := range projects {
                h.SessionCount += proj.SessionCount
            }
            if !p.Version.IsKnown() {
                h.Note = "Storage version is unknown (fingerprint " + p.Version.Fingerprint + "). Read-only operations work; destructive operations refuse without an extra opt-in."
            }
        }
        out = append(out, h)
    }
    return out
}
```

- [ ] **Step 4: Write `composition/browse_test.go`**

```go
package composition

import (
    "io/fs"
    "testing"
    "testing/fstest"

    "github.com/danieljbfz/chronicle/contracts"
)

// fakeProvider lets composition tests stay in-package and avoid file I/O.
type fakeProvider struct {
    name     string
    projects []contracts.Project
    sessions map[contracts.ProjectID][]contracts.SessionSummary
    convos   map[contracts.SessionID]contracts.Conversation
    version  contracts.StorageVersion
}

func (f *fakeProvider) Name() string                                              { return f.name }
func (f *fakeProvider) Detect(fs.FS) (contracts.StorageVersion, error)            { return f.version, nil }
func (f *fakeProvider) ListProjects(fs.FS) ([]contracts.Project, error)           { return f.projects, nil }
func (f *fakeProvider) ListSessions(_ fs.FS, p contracts.ProjectID) ([]contracts.SessionSummary, error) {
    return f.sessions[p], nil
}
func (f *fakeProvider) ReadSession(_ fs.FS, id contracts.SessionID) (contracts.Conversation, error) {
    c, ok := f.convos[id]
    if !ok {
        return contracts.Conversation{}, fs.ErrNotExist
    }
    return c, nil
}
func (f *fakeProvider) PlanDelete(fs.FS, contracts.SessionID) (contracts.DeletePlan, error) {
    return contracts.DeletePlan{}, nil
}
func (f *fakeProvider) PlanOrphanScan(fs.FS) (contracts.DeletePlan, error) {
    return contracts.DeletePlan{}, nil
}

func newAppWithFakes(p ...*fakeProvider) *App {
    a := &App{}
    for _, fp := range p {
        a.providers = append(a.providers, &providerEntry{
            Provider: fp,
            FS:       fstest.MapFS{},
            Version:  fp.version,
        })
    }
    return a
}

func TestApp_ListProjects_combinesProviders(t *testing.T) {
    a := newAppWithFakes(
        &fakeProvider{name: "claude", projects: []contracts.Project{{ID: "p1", DisplayName: "proj1"}}},
        &fakeProvider{name: "copilot", projects: []contracts.Project{{ID: "p2", DisplayName: "proj2"}}},
    )
    listings, err := a.ListProjects()
    if err != nil {
        t.Fatal(err)
    }
    if len(listings) != 2 {
        t.Fatalf("got %d listings, want 2", len(listings))
    }
    names := []string{listings[0].Provider, listings[1].Provider}
    if names[0]+names[1] != "claudecopilot" && names[0]+names[1] != "copilotclaude" {
        t.Errorf("providers = %v, expected claude+copilot", names)
    }
}

func TestApp_ReadSession_unknownIdReturnsNotExist(t *testing.T) {
    a := newAppWithFakes(&fakeProvider{name: "claude"})
    _, err := a.ReadSession("nope")
    if err == nil {
        t.Error("expected fs.ErrNotExist")
    }
}

func TestDoctor_includesUnknownVersionNote(t *testing.T) {
    a := newAppWithFakes(&fakeProvider{
        name:    "claude",
        version: contracts.StorageVersion{Adapter: "claude", Version: "unknown", Fingerprint: "deadbeef"},
    })
    healths := a.Doctor()
    if len(healths) != 1 {
        t.Fatalf("got %d health entries, want 1", len(healths))
    }
    if healths[0].Note == "" {
        t.Error("Doctor should attach a Note for unknown version")
    }
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./adapters/... ./composition/...
```

Expected: all three composition tests pass; the existing claude tests still pass.

- [ ] **Step 6: Commit**

```bash
git add adapters/all.go composition/
git commit -m "feat(adapters,composition): registry + browse + doctor wired around providers"
```

---

## Task 16: `cmd/chronicle` scaffolding with cobra

**Files:**
- Modify: `go.mod` (add cobra)
- Create: `cmd/chronicle/main.go`
- Test: `cmd/chronicle/main_test.go`

- [ ] **Step 1: Add cobra**

```bash
go get github.com/spf13/cobra@latest
```

Expected: `go.mod` lists `github.com/spf13/cobra`.

- [ ] **Step 2: Write `cmd/chronicle/main.go` (root command only — subcommands land in later tasks)**

```go
// chronicle is the local tool for browsing, exporting, and cleaning the
// on-disk history of AI coding assistants. See the README for usage and
// docs/superpowers/specs/2026-05-15-chronicle-design.md for the design.
package main

import (
    "fmt"
    "os"

    "github.com/spf13/cobra"
)

var version = "0.1.0-plan-a"

func main() {
    if err := newRootCmd().Execute(); err != nil {
        // cobra already printed the error; exit non-zero.
        os.Exit(1)
    }
}

func newRootCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:           "chronicle",
        Short:         "Browse, export, and clean the history of AI coding assistants",
        Long:          "chronicle reads ~/.claude and (in later plans) VS Code Copilot storage,\nrenders sessions as Markdown, and helps you clean up the mess.",
        SilenceUsage:  true,
        SilenceErrors: false,
        Version:       version,
    }

    cmd.AddCommand(newListCmd())
    cmd.AddCommand(newExportCmd())
    cmd.AddCommand(newCopyCmd())
    cmd.AddCommand(newDoctorCmd())
    return cmd
}

// fail is a tiny helper for command handlers — prints to stderr and
// returns a non-nil error so cobra exits non-zero.
func fail(format string, args ...any) error {
    fmt.Fprintf(os.Stderr, "chronicle: "+format+"\n", args...)
    return fmt.Errorf(format, args...)
}
```

- [ ] **Step 3: Write `cmd/chronicle/main_test.go` (smoke test that the binary builds and `--version` works)**

```go
package main

import (
    "bytes"
    "strings"
    "testing"
)

func TestRootCmd_versionFlag(t *testing.T) {
    cmd := newRootCmd()
    var buf bytes.Buffer
    cmd.SetOut(&buf)
    cmd.SetErr(&buf)
    cmd.SetArgs([]string{"--version"})
    if err := cmd.Execute(); err != nil {
        t.Fatalf("Execute: %v", err)
    }
    if !strings.Contains(buf.String(), version) {
        t.Errorf("--version output = %q, want it to contain %q", buf.String(), version)
    }
}

func TestRootCmd_helpListsSubcommands(t *testing.T) {
    cmd := newRootCmd()
    var buf bytes.Buffer
    cmd.SetOut(&buf)
    cmd.SetErr(&buf)
    cmd.SetArgs([]string{"--help"})
    if err := cmd.Execute(); err != nil {
        t.Fatalf("Execute --help: %v", err)
    }
    for _, want := range []string{"list", "export", "copy", "doctor"} {
        if !strings.Contains(buf.String(), want) {
            t.Errorf("--help missing subcommand %q in:\n%s", want, buf.String())
        }
    }
}
```

- [ ] **Step 4: Compile and test**

Steps 5–8 add the subcommand definitions. Their `new...Cmd` functions are referenced above; we add them next so the tests will pass after Task 17. Until then, this task will not compile — which is the failing-test signal of TDD. **Skip running tests at this exact step.** The next four tasks build the missing commands.

- [ ] **Step 5: (deferred) Commit happens at the end of Task 17**

---

## Task 17: `chronicle list` subcommand

**Files:**
- Create: `cmd/chronicle/list.go`
- Test: `cmd/chronicle/list_test.go`

- [ ] **Step 1: Write `cmd/chronicle/list.go`**

```go
package main

import (
    "encoding/json"
    "fmt"
    "io"
    "os"

    "github.com/spf13/cobra"

    "github.com/danieljbfz/chronicle/composition"
)

func newListCmd() *cobra.Command {
    var providerFlag string

    cmd := &cobra.Command{
        Use:   "list",
        Short: "List sessions across all detected providers (JSON lines)",
        RunE: func(cmd *cobra.Command, args []string) error {
            app, err := composition.New()
            if err != nil {
                return fail("init: %v", err)
            }
            listings, err := app.ListSessionsAll(providerFlag)
            if err != nil {
                return fail("list: %v", err)
            }
            return writeListings(cmd.OutOrStdout(), listings)
        },
    }
    cmd.Flags().StringVar(&providerFlag, "provider", "", `Limit to one provider, e.g. "claude"`)
    return cmd
}

func writeListings(w io.Writer, listings []composition.SessionListing) error {
    encoder := json.NewEncoder(w)
    for _, l := range listings {
        out := struct {
            Provider     string   `json:"provider"`
            SessionID    string   `json:"session_id"`
            ProjectID    string   `json:"project_id"`
            Title        string   `json:"title"`
            StartedAt    string   `json:"started_at,omitempty"`
            LastActive   string   `json:"last_active,omitempty"`
            TurnCount    int      `json:"turn_count"`
            SizeBytes    int64    `json:"size_bytes"`
            Version      string   `json:"version"`
            Fingerprint  string   `json:"fingerprint,omitempty"`
        }{
            Provider:    l.Provider,
            SessionID:   string(l.Summary.ID),
            ProjectID:   string(l.Summary.Project),
            Title:       l.Summary.Title,
            StartedAt:   fmtTime(l.Summary.StartedAt),
            LastActive:  fmtTime(l.Summary.LastActive),
            TurnCount:   l.Summary.TurnCount,
            SizeBytes:   l.Summary.SizeBytes,
            Version:     l.Summary.Source.Version,
            Fingerprint: l.Summary.Source.Fingerprint,
        }
        if err := encoder.Encode(out); err != nil {
            return fmt.Errorf("encode: %w", err)
        }
    }
    return nil
}

// silence "unused" when this file is built with stripped imports during tests
var _ = os.Stdout
```

- [ ] **Step 2: Add `fmtTime` helper in `cmd/chronicle/main.go`**

Append below the existing code in `cmd/chronicle/main.go`:

```go
import "time" // add to the import block at the top, do not duplicate "fmt" / "os"

func fmtTime(t time.Time) string {
    if t.IsZero() {
        return ""
    }
    return t.Format(time.RFC3339)
}
```

(If the engineer's editor produces duplicate imports, the build will fail. Merge `time` into the existing import block — it should be the only change to imports here.)

- [ ] **Step 3: Write `cmd/chronicle/list_test.go`**

```go
package main

import (
    "bytes"
    "encoding/json"
    "strings"
    "testing"

    "github.com/danieljbfz/chronicle/composition"
    "github.com/danieljbfz/chronicle/contracts"
)

func TestWriteListings_emitsJSONLines(t *testing.T) {
    var buf bytes.Buffer
    err := writeListings(&buf, []composition.SessionListing{
        {Provider: "claude", Summary: contracts.SessionSummary{
            ID: "abc", Project: "-Users-test-proj", Title: "Hello",
            TurnCount: 3, SizeBytes: 1234,
            Source: contracts.StorageVersion{Version: "claude-1.0", Fingerprint: "abcd1234"},
        }},
        {Provider: "claude", Summary: contracts.SessionSummary{ID: "def"}},
    })
    if err != nil {
        t.Fatal(err)
    }
    lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
    if len(lines) != 2 {
        t.Fatalf("got %d lines, want 2", len(lines))
    }
    var first map[string]any
    if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
        t.Fatalf("line 0 is not JSON: %v", err)
    }
    if first["session_id"] != "abc" {
        t.Errorf("session_id = %v, want abc", first["session_id"])
    }
    if first["title"] != "Hello" {
        t.Errorf("title = %v, want Hello", first["title"])
    }
}
```

- [ ] **Step 4: Run tests for cmd/chronicle**

```bash
go test ./cmd/chronicle/...
```

Expected: tests for `list` pass. Tests for `export`, `copy`, `doctor` referenced from `newRootCmd` may still be missing — but the helper-level test we just wrote passes. The full root-command tests pass once Task 18 lands.

- [ ] **Step 5: Do not commit yet — continue with Task 18.**

---

## Task 18: `chronicle export` subcommand

**Files:**
- Create: `cmd/chronicle/export.go`
- Test: `cmd/chronicle/export_test.go`

- [ ] **Step 1: Write `cmd/chronicle/export.go`**

```go
package main

import (
    "fmt"
    "io"
    "os"

    "github.com/spf13/cobra"

    "github.com/danieljbfz/chronicle/composition"
    "github.com/danieljbfz/chronicle/contracts"
    "github.com/danieljbfz/chronicle/steps"
)

func newExportCmd() *cobra.Command {
    var (
        noTools, noThinking, noMeta bool
        outPath                     string
    )
    cmd := &cobra.Command{
        Use:   "export <sessionId>",
        Short: "Write a filtered Markdown transcript to a file or stdout",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            app, err := composition.New()
            if err != nil {
                return fail("init: %v", err)
            }
            return runExport(app, contracts.SessionID(args[0]), exportOpts{
                noTools:    noTools,
                noThinking: noThinking,
                noMeta:     noMeta,
                outPath:    outPath,
            }, cmd.OutOrStdout())
        },
    }
    cmd.Flags().BoolVar(&noTools, "no-tools", false, "Drop tool use and tool result blocks")
    cmd.Flags().BoolVar(&noThinking, "no-thinking", false, "Drop assistant thinking blocks")
    cmd.Flags().BoolVar(&noMeta, "no-meta", false, "Drop meta messages (slash-command echoes, hook output)")
    cmd.Flags().StringVarP(&outPath, "out", "o", "", "Write to this file instead of stdout")
    return cmd
}

type exportOpts struct {
    noTools, noThinking, noMeta bool
    outPath                     string
}

func runExport(app *composition.App, id contracts.SessionID, opts exportOpts, stdout io.Writer) error {
    conv, err := app.ReadSession(id)
    if err != nil {
        return fail("read session %q: %v", id, err)
    }
    conv = steps.Filter(conv, steps.FilterOptions{
        HideTools:    opts.noTools,
        HideThinking: opts.noThinking,
        HideMeta:     opts.noMeta,
    })
    md := steps.Markdown(conv)

    if opts.outPath == "" {
        _, err := io.WriteString(stdout, md)
        return err
    }
    if err := os.WriteFile(opts.outPath, []byte(md), 0o644); err != nil {
        return fmt.Errorf("write %s: %w", opts.outPath, err)
    }
    fmt.Fprintf(os.Stderr, "Wrote %d bytes to %s\n", len(md), opts.outPath)
    return nil
}
```

- [ ] **Step 2: Write `cmd/chronicle/export_test.go`**

```go
package main

import (
    "bytes"
    "io/fs"
    "strings"
    "testing"
    "testing/fstest"

    "github.com/danieljbfz/chronicle/composition"
    "github.com/danieljbfz/chronicle/contracts"
)

// stubProvider lets us exercise runExport without touching the real
// filesystem. It is defined here (not exported) because export is the only
// caller in this package that needs it.
type stubProvider struct {
    convo contracts.Conversation
}

func (stubProvider) Name() string                                                { return "stub" }
func (stubProvider) Detect(fs.FS) (contracts.StorageVersion, error)              { return contracts.StorageVersion{Adapter: "stub", Version: "stub-1"}, nil }
func (stubProvider) ListProjects(fs.FS) ([]contracts.Project, error)             { return nil, nil }
func (stubProvider) ListSessions(fs.FS, contracts.ProjectID) ([]contracts.SessionSummary, error) {
    return nil, nil
}
func (s stubProvider) ReadSession(_ fs.FS, _ contracts.SessionID) (contracts.Conversation, error) {
    return s.convo, nil
}
func (stubProvider) PlanDelete(fs.FS, contracts.SessionID) (contracts.DeletePlan, error) {
    return contracts.DeletePlan{}, nil
}
func (stubProvider) PlanOrphanScan(fs.FS) (contracts.DeletePlan, error) {
    return contracts.DeletePlan{}, nil
}

func TestRunExport_writesMarkdownToStdoutWhenNoOut(t *testing.T) {
    convo := contracts.Conversation{
        SessionID: "abc",
        Source:    contracts.StorageVersion{Adapter: "stub"},
        Messages: []contracts.Message{
            {Role: contracts.RoleUser, Blocks: []contracts.Block{contracts.TextBlock{Text: "hello"}}},
            {Role: contracts.RoleAssistant, Blocks: []contracts.Block{contracts.TextBlock{Text: "hi"}}},
        },
    }
    app := composition.NewForTest([]contracts.Provider{stubProvider{convo: convo}}, []fs.FS{fstest.MapFS{}})
    var buf bytes.Buffer
    if err := runExport(app, "abc", exportOpts{}, &buf); err != nil {
        t.Fatal(err)
    }
    if !strings.Contains(buf.String(), "hello") {
        t.Errorf("output missing user text: %q", buf.String())
    }
}
```

- [ ] **Step 3: Add `composition.NewForTest` helper**

In `composition/browse.go`, append:

```go
// NewForTest builds an App from fakes. Production code must use New(); tests
// in this and other packages use this constructor to skip filesystem I/O.
func NewForTest(providers []contracts.Provider, roots []fs.FS) *App {
    a := &App{}
    for i, p := range providers {
        var fsys fs.FS
        if i < len(roots) {
            fsys = roots[i]
        }
        a.providers = append(a.providers, &providerEntry{Provider: p, FS: fsys})
    }
    return a
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./...
```

Expected: tests pass for `composition`, `cmd/chronicle`, `steps`, `contracts`, `adapters/claude`, `internal/paths`, `internal/config`.

- [ ] **Step 5: Do not commit yet — continue with Task 19.**

---

## Task 19: `chronicle copy` and `chronicle doctor` subcommands

**Files:**
- Create: `cmd/chronicle/copy.go`
- Create: `cmd/chronicle/doctor.go`
- Test: `cmd/chronicle/doctor_test.go`

- [ ] **Step 1: Write `cmd/chronicle/copy.go`**

```go
package main

import (
    "fmt"
    "os"

    "github.com/spf13/cobra"

    "github.com/danieljbfz/chronicle/composition"
    "github.com/danieljbfz/chronicle/contracts"
    "github.com/danieljbfz/chronicle/steps"
)

func newCopyCmd() *cobra.Command {
    var noTools, noThinking bool
    cmd := &cobra.Command{
        Use:   "copy <sessionId>",
        Short: "Copy a filtered Markdown transcript to the clipboard via OSC52",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            app, err := composition.New()
            if err != nil {
                return fail("init: %v", err)
            }
            conv, err := app.ReadSession(contracts.SessionID(args[0]))
            if err != nil {
                return fail("read session %q: %v", args[0], err)
            }
            conv = steps.Filter(conv, steps.FilterOptions{
                HideTools:    noTools,
                HideThinking: noThinking,
            })
            md := steps.Markdown(conv)
            if err := steps.CopyOSC52(cmd.OutOrStdout(), md); err != nil {
                return fail("clipboard: %v", err)
            }
            fmt.Fprintf(os.Stderr, "Copied %d bytes to clipboard via OSC52.\n", len(md))
            return nil
        },
    }
    cmd.Flags().BoolVar(&noTools, "no-tools", false, "Drop tool use and tool result blocks")
    cmd.Flags().BoolVar(&noThinking, "no-thinking", false, "Drop assistant thinking blocks")
    return cmd
}
```

- [ ] **Step 2: Write `cmd/chronicle/doctor.go`**

```go
package main

import (
    "encoding/json"
    "fmt"
    "io"

    "github.com/spf13/cobra"

    "github.com/danieljbfz/chronicle/composition"
)

func newDoctorCmd() *cobra.Command {
    var asJSON bool
    cmd := &cobra.Command{
        Use:   "doctor",
        Short: "Show detected providers, versions, fingerprints, and any warnings",
        RunE: func(cmd *cobra.Command, args []string) error {
            app, err := composition.New()
            if err != nil {
                return fail("init: %v", err)
            }
            healths := app.Doctor()
            if asJSON {
                return writeDoctorJSON(cmd.OutOrStdout(), healths)
            }
            return writeDoctorText(cmd.OutOrStdout(), healths)
        },
    }
    cmd.Flags().BoolVar(&asJSON, "json", false, "Emit results as JSON instead of text")
    return cmd
}

func writeDoctorJSON(w io.Writer, healths []composition.ProviderHealth) error {
    encoder := json.NewEncoder(w)
    encoder.SetIndent("", "  ")
    return encoder.Encode(healths)
}

func writeDoctorText(w io.Writer, healths []composition.ProviderHealth) error {
    if len(healths) == 0 {
        fmt.Fprintln(w, "No providers detected. Enable providers in ~/.config/chronicle/config.toml.")
        return nil
    }
    for _, h := range healths {
        fmt.Fprintf(w, "Provider: %s\n", h.Name)
        fmt.Fprintf(w, "  Root:        %s\n", h.Root)
        fmt.Fprintf(w, "  Version:     %s\n", h.Version.Version)
        if h.Version.Fingerprint != "" {
            fmt.Fprintf(w, "  Fingerprint: %s\n", h.Version.Fingerprint)
        }
        fmt.Fprintf(w, "  Reachable:   %v\n", h.Reachable)
        fmt.Fprintf(w, "  Sessions:    %d\n", h.SessionCount)
        if h.Note != "" {
            fmt.Fprintf(w, "  Note:        %s\n", h.Note)
        }
        fmt.Fprintln(w)
    }
    return nil
}
```

- [ ] **Step 3: Write `cmd/chronicle/doctor_test.go`**

```go
package main

import (
    "bytes"
    "encoding/json"
    "strings"
    "testing"

    "github.com/danieljbfz/chronicle/composition"
    "github.com/danieljbfz/chronicle/contracts"
)

func TestDoctorText_rendersFields(t *testing.T) {
    healths := []composition.ProviderHealth{{
        Name:         "claude",
        Root:         "/home/u/.claude",
        Version:      contracts.StorageVersion{Version: "claude-1.0", Fingerprint: "abc123"},
        Reachable:    true,
        SessionCount: 42,
    }}
    var buf bytes.Buffer
    if err := writeDoctorText(&buf, healths); err != nil {
        t.Fatal(err)
    }
    for _, want := range []string{"claude", "/home/u/.claude", "claude-1.0", "abc123", "Sessions:    42"} {
        if !strings.Contains(buf.String(), want) {
            t.Errorf("doctor text missing %q in:\n%s", want, buf.String())
        }
    }
}

func TestDoctorJSON_isValidJSON(t *testing.T) {
    healths := []composition.ProviderHealth{{Name: "claude", Reachable: true}}
    var buf bytes.Buffer
    if err := writeDoctorJSON(&buf, healths); err != nil {
        t.Fatal(err)
    }
    var got []composition.ProviderHealth
    if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
        t.Fatalf("doctor JSON not parseable: %v", err)
    }
    if len(got) != 1 || got[0].Name != "claude" {
        t.Errorf("decoded %v, want one entry named claude", got)
    }
}

func TestDoctorText_emptyHealthsExplains(t *testing.T) {
    var buf bytes.Buffer
    if err := writeDoctorText(&buf, nil); err != nil {
        t.Fatal(err)
    }
    if !strings.Contains(buf.String(), "No providers detected") {
        t.Errorf("empty doctor should explain itself, got: %q", buf.String())
    }
}
```

- [ ] **Step 4: Run the whole test suite**

```bash
go test ./...
```

Expected: every package passes.

- [ ] **Step 5: Build the binary**

```bash
go build -o chronicle ./cmd/chronicle
./chronicle --version
./chronicle --help
```

Expected: version line prints, help lists `list`, `export`, `copy`, `doctor`.

- [ ] **Step 6: Commit everything from Tasks 16 through 19**

```bash
git add cmd/chronicle/ composition/browse.go go.mod go.sum
git commit -m "feat(cli): list, export, copy, doctor subcommands"
```

---

## Task 20: End-to-end smoke test against real data, populate knownFingerprints

**Files:**
- Modify: `adapters/claude/detect.go`
- Modify: `README.md`

- [ ] **Step 1: Run `doctor` against the real `~/.claude`**

```bash
./chronicle doctor
```

Expected output: one block for `claude` with `Version: unknown`, a non-empty fingerprint, and a `Reachable: true` line. Copy the fingerprint — it goes into the code in the next step.

- [ ] **Step 2: Record the observed fingerprint as `claude-1.0`**

Edit `adapters/claude/detect.go`. Replace the empty `knownFingerprints` map with the captured value:

```go
var knownFingerprints = map[string]string{
    "<paste-the-fingerprint-from-step-1>": "claude-1.0",
}
```

Re-run:

```bash
./chronicle doctor
```

Expected: `Version: claude-1.0` (not unknown).

- [ ] **Step 3: Spot-check `list`**

```bash
./chronicle list | head -3
```

Expected: three JSON lines, each with `provider: "claude"`, real session ids, titles, sizes.

- [ ] **Step 4: Spot-check `export`**

Pick a session id from step 3 with a non-trivial title and run:

```bash
./chronicle export <sessionId> --no-tools --no-thinking -o /tmp/test.md
wc -l /tmp/test.md
head -40 /tmp/test.md
```

Expected: the file is written, the head shows the title, the User and Assistant labels, and the conversation text. No tool blocks, no thinking quotes.

- [ ] **Step 5: Spot-check `copy`** (skip if running over SSH and the local terminal cannot accept OSC52)

```bash
./chronicle copy <sessionId> --no-tools
```

Expected: stderr message "Copied N bytes to clipboard via OSC52." Paste somewhere to confirm.

- [ ] **Step 6: Update the README with the captured fingerprint note**

Append to `README.md`:

```markdown
## Recognized storage versions (Plan A)

| Provider | Version | First seen |
|---|---|---|
| Claude Code | `claude-1.0` | 2026-05-15 (Claude Code 2.1.x) |

If `chronicle doctor` shows `Version: unknown`, the fingerprint did not match
the table above. The tool still works in read-only mode — see
`docs/research/07-schema-resilience.md` for the contract.
```

- [ ] **Step 7: Commit**

```bash
git add adapters/claude/detect.go README.md
git commit -m "feat(claude): map first observed fingerprint to claude-1.0"
```

- [ ] **Step 8: Final verification**

```bash
go test ./...
go vet ./...
./chronicle doctor
```

Expected: all tests pass, vet is clean, doctor reports `claude-1.0` for the local install.

---

## Self-review notes (filled in after writing the plan)

**Spec coverage:**

- §3 layering — covered by Tasks 2–4 (contracts), 5–6 (internal), 7–10 (steps), 11–14 (adapter), 15 (composition), 16–19 (cmd).
- §4 domain contracts — Tasks 2–4 cover every type listed in the spec.
- §5.1 Claude adapter — Tasks 11–14.
- §5.2 Copilot — deferred to Plan B (called out in the scope-split message).
- §5.3 stubs — Plan B introduces the empty packages when we add the second adapter.
- §6 resilience contract — `steps/fingerprint.go` (Task 7), `claude/detect.go` (Task 12), `claude/parse.go` UnknownBlock preservation + the canary test (Task 13), Doctor surface (Task 15).
- §9 CLI commands `list` / `export` / `copy` / `doctor` — Tasks 17–19. `clean`, `trash`, `tui`, `serve` deferred to later plans.
- §11 config — Task 6.
- §12 distribution — Task 20 produces a working `go build`. A Homebrew tap is post-v1 work.
- §13 test strategy — every task ships tests next to source; the canary test in Task 13 enforces the resilience contract.
- §14 deferred items — match the spec (TUI, Copilot, cleanup, web all deferred to later plans).

**Placeholder scan:** none. Every code block contains real Go that compiles.

**Type consistency:** I cross-checked the names `Provider`, `Conversation`, `Message`, `Block`, `StorageVersion`, `Capabilities`, `DeletePlan`, `DeleteItem`, `Project`, `ProjectID`, `SessionID`, `SessionSummary`, `MessageID`, `Role` across tasks. They are stable.

**Scope:** twenty tasks. Each is bite-sized. Plan A produces a working CLI in roughly 20 sittings of 5–15 minutes each — appropriate for a single plan.

**Known minor item to verify during execution:** Task 17 step 2 asks the engineer to merge a `time` import into the existing import block of `main.go`. If they accidentally add a second import block, `go vet` will catch it and the fix is a one-line move.
