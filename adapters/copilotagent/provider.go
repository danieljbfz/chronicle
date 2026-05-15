package copilotagent

import (
	"errors"
	"io/fs"
	"path"
	"sort"

	"github.com/danieljbfz/chronicle/contracts"
)

// agentProjectID is the synthetic project identifier the
// adapter uses for every session. The agent runtime stores
// sessions in a flat list under session-state/, with no
// per-project subdivision on disk: the cwd is recorded
// inside each session's events.jsonl, but two sessions
// from different cwds live as siblings in the same
// directory tree. We therefore present one synthetic
// project that holds every agent session.
//
// A future iteration could group sessions by their
// recorded cwd and surface one project per distinct
// workspace folder. That is a small refactor: scan every
// session's session.start event, group by cwd, expose one
// Project per unique value. We defer that until the agent
// runtime is in active use and the listing experience
// becomes the bottleneck.
const (
	agentProjectID   contracts.ProjectID = "agent-sessions"
	agentDisplayName string              = "Agent sessions"
)

// Provider is the chronicle adapter for the GitHub Copilot
// agent runtime (the @github/copilot-sdk LocalSessionManager
// at ~/.copilot/). The type holds a small cache of the
// storage version it detected the first time someone
// asked, so repeated calls do not pay the detection cost
// again.
type Provider struct {
	cached  contracts.StorageVersion
	cacheOK bool
}

// New returns a ready-to-use Provider. The constructor
// stays I/O-free on purpose. The first caller that asks
// for the storage version is the one that pays the
// disk-read cost, and everyone after that gets the cached
// result.
func New() *Provider { return &Provider{} }

// Name returns the adapter's stable identifier. The
// doctor view and the JSON output of the list command
// use this to label rows that came from the agent
// runtime, distinguishing them from the chat extension's
// rows in the same chronicle install.
func (*Provider) Name() string { return adapterName }

// Detect returns the StorageVersion for the given root.
// The first call inspects the root for a session-state
// directory; every later call serves from the in-memory
// cache.
func (p *Provider) Detect(root fs.FS) (contracts.StorageVersion, error) {
	if p.cacheOK {
		return p.cached, nil
	}
	sv, err := Detect(root)
	if err != nil {
		return contracts.StorageVersion{}, err
	}
	p.cached = sv
	p.cacheOK = true
	return sv, nil
}

// ListProjects returns a single synthetic project that
// holds every agent session. The agent runtime does not
// partition its sessions by project on disk, so the
// adapter cannot honestly present a per-project listing.
// The synthetic project keeps the chronicle UI uniform
// across providers without misrepresenting how the data
// is stored.
//
// Returns an empty slice (not an error) when the
// session-state directory is missing or empty. A
// fresh-install user with no agent activity yet hits this
// branch.
func (p *Provider) ListProjects(root fs.FS) ([]contracts.Project, error) {
	entries, err := fs.ReadDir(root, sessionStateDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, newError("list projects", sessionStateDir, err)
	}

	sessionCount := 0
	var totalBytes int64
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sessionCount++
		// The session size is the size of every file
		// inside the session directory, not just events.jsonl.
		// We count the events.jsonl size because that is the
		// dominant on-disk weight; a more precise total
		// would walk every file inside the directory and
		// sum, which is not worth the cost for a listing.
		eventsPath := path.Join(sessionStateDir, entry.Name(), eventsFile)
		if info, err := fs.Stat(root, eventsPath); err == nil {
			totalBytes += info.Size()
		}
	}
	if sessionCount == 0 {
		return nil, nil
	}
	return []contracts.Project{{
		ID:           agentProjectID,
		DisplayName:  agentDisplayName,
		SessionCount: sessionCount,
		SizeBytes:    totalBytes,
	}}, nil
}

// ListSessions returns one SessionSummary per session
// directory under session-state/. The adapter ignores the
// project argument because every session belongs to the
// single synthetic project; the parameter is retained to
// satisfy the contracts.Provider interface and so a
// future per-cwd grouping refinement does not change the
// signature.
func (p *Provider) ListSessions(root fs.FS, project contracts.ProjectID) ([]contracts.SessionSummary, error) {
	entries, err := fs.ReadDir(root, sessionStateDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, newError("list sessions", sessionStateDir, err)
	}

	var summaries []contracts.SessionSummary
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sessionDir := path.Join(sessionStateDir, entry.Name())
		conv, err := readSession(root, sessionDir, p.cached)
		if err != nil {
			// One unreadable session should not bury the
			// rest. We skip it and let the doctor view
			// surface the per-session read failure if the
			// user asks.
			continue
		}
		var size int64
		eventsPath := path.Join(sessionDir, eventsFile)
		if info, err := fs.Stat(root, eventsPath); err == nil {
			size = info.Size()
		}
		summaries = append(summaries, contracts.SessionSummary{
			ID:           contracts.SessionID(entry.Name()),
			Project:      agentProjectID,
			StartedAt:    conv.StartedAt,
			LastActive:   conv.EndedAt,
			Title:        sessionTitle(conv),
			TurnCount:    len(conv.Messages),
			SizeBytes:    size,
			Capabilities: p.cached.Capabilities,
			Source:       p.cached,
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].LastActive.After(summaries[j].LastActive)
	})
	return summaries, nil
}

// ReadSession returns the parsed Conversation for one
// session by id. The agent stores each session in its own
// directory, so the lookup is one stat call.
func (p *Provider) ReadSession(root fs.FS, id contracts.SessionID) (contracts.Conversation, error) {
	sessionDir := path.Join(sessionStateDir, string(id))
	if _, err := fs.Stat(root, path.Join(sessionDir, eventsFile)); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return contracts.Conversation{}, fs.ErrNotExist
		}
		return contracts.Conversation{}, newError("read session", sessionDir, err)
	}
	conv, err := readSession(root, sessionDir, p.cached)
	if err != nil {
		return contracts.Conversation{}, err
	}
	conv.Project = agentProjectID
	return conv, nil
}

// sessionTitle picks the best title for a session listing.
// readSession populates conv.Title from
// vscode.metadata.json when present; otherwise we fall back
// to the first user prompt, the same convention every
// other adapter follows.
func sessionTitle(conv contracts.Conversation) string {
	if conv.Title != "" {
		return conv.Title
	}
	return conv.FirstUserPrompt()
}

// Compile-time check: *Provider satisfies the base
// contracts.Provider interface. If a future contract
// change adds or renames a method, the build fails right
// here with the missing method named.
//
// The agent adapter does not yet implement contracts.Cleaner.
// Cleanup capabilities arrive once we have a clear picture
// of the agent runtime's per-session sibling artifacts
// (checkpoints, files, research) and what cascade rules
// they need.
var _ contracts.Provider = (*Provider)(nil)
