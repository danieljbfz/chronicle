package copilotagent

import (
	"errors"
	"io/fs"
	"path"

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
// directory. Every later call serves from the in-memory
// cache.
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
	// Step 1: enumerate the session-state directory. A
	// missing directory is the fresh-install case and
	// translates to an empty result rather than an error.
	entries, err := fs.ReadDir(root, sessionStateDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, newError("list projects", sessionStateDir, err)
	}

	// Step 2: count sessions and accumulate disk usage.
	// We measure each session's events.jsonl size because
	// that is the dominant on-disk weight per session. A
	// more precise total would walk every file inside the
	// session directory and sum, which is not worth the
	// cost for a listing.
	sessionCount := 0
	var totalBytes int64
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sessionCount++
		eventsPath := path.Join(sessionStateDir, entry.Name(), eventsFile)
		if info, err := fs.Stat(root, eventsPath); err == nil {
			totalBytes += info.Size()
		}
	}

	// Step 3: emit one synthetic project covering every
	// session, or nothing at all when the user has never
	// invoked the agent runtime.
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

// ListSessionRefs returns one parse-free ref per session directory under
// session-state. The project argument is ignored because every session
// belongs to the single synthetic project. The locator is the session
// directory, and the size comes from its events.jsonl, the dominant
// on-disk weight per session.
func (p *Provider) ListSessionRefs(root fs.FS, _ contracts.ProjectID) ([]contracts.SessionRef, error) {
	entries, err := fs.ReadDir(root, sessionStateDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, newError("list sessions", sessionStateDir, err)
	}
	var refs []contracts.SessionRef
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sessionDir := path.Join(sessionStateDir, entry.Name())
		ref := contracts.SessionRef{
			ID:      contracts.SessionID(entry.Name()),
			Project: agentProjectID,
			Locator: sessionDir,
		}
		if info, err := fs.Stat(root, path.Join(sessionDir, eventsFile)); err == nil {
			ref.SizeBytes = info.Size()
			ref.ModTime = info.ModTime()
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

// SummarizeSession folds the one session's event stream into a listing
// summary, paid only on a cache miss. The locator is the session
// directory the events live in.
func (p *Provider) SummarizeSession(root fs.FS, ref contracts.SessionRef) (contracts.SessionSummary, error) {
	conv, err := readSession(root, ref.Locator, p.cached)
	if err != nil {
		return contracts.SessionSummary{}, newError("read session", ref.Locator, err)
	}
	return contracts.NewSessionSummary(ref, conv, p.cached), nil
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
