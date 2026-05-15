package copilotchat

import (
	"errors"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/danieljbfz/chronicle/contracts"
)

// Provider is the chronicle adapter for the GitHub Copilot
// Chat extension, which stores its data inside VS Code's
// workspaceStorage tree. Composition keeps one instance
// per detected Copilot Chat root (one for VS Code, one for
// VS Code Insiders, and so on). Provider holds a small
// cache of the storage version it detected the first time
// someone asked, so repeated calls do not pay the
// detection cost again.
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
// doctor view and the JSON output of the list command use
// this to label rows that came from the Copilot Chat
// extension.
func (*Provider) Name() string { return adapterName }

// Detect returns the StorageVersion for the given root. The
// first call computes the fingerprint by reading one
// session file. Every later call serves from the in-memory
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

// ListProjects returns one Project per VS Code workspace plus a
// synthetic "(no workspace)" project that holds the chats from
// VS Code windows that were opened without a folder. The synthetic
// project only appears when at least one empty-window chat exists,
// so users who never use Copilot in unsaved windows do not see an
// empty-looking row in the listing.
func (p *Provider) ListProjects(root fs.FS) ([]contracts.Project, error) {
	projects, err := listWorkspaceProjects(root)
	if err != nil {
		return nil, newError("list projects", workspaceStorageDir, err)
	}

	if emptyProject, ok := emptyWindowProject(root); ok {
		projects = append(projects, emptyProject)
	}

	sort.Slice(projects, func(i, j int) bool {
		return projects[i].DisplayName < projects[j].DisplayName
	})
	return projects, nil
}

// listWorkspaceProjects walks workspaceStorage and produces one
// Project per workspace that has at least one Copilot chat session.
// We skip workspaces with no sessions because they exist for some
// other VS Code feature (an extension's storage, for example) and
// the user has no reason to see them in chronicle.
func listWorkspaceProjects(root fs.FS) ([]contracts.Project, error) {
	entries, err := fs.ReadDir(root, workspaceStorageDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var projects []contracts.Project
	for _, ws := range entries {
		if !ws.IsDir() {
			continue
		}
		summary, ok := summarizeWorkspace(root, ws.Name())
		if !ok {
			continue
		}
		projects = append(projects, summary)
	}
	return projects, nil
}

// summarizeWorkspace counts the chat sessions inside one workspace
// folder and decodes the workspace's display name from its
// workspace.json file. The function returns ok=false when the
// workspace has no chat sessions, so the caller can skip empty
// workspaces.
func summarizeWorkspace(root fs.FS, hash string) (contracts.Project, bool) {
	chatDir := path.Join(workspaceStorageDir, hash, chatSessionsDir)
	entries, err := fs.ReadDir(root, chatDir)
	if err != nil {
		return contracts.Project{}, false
	}

	sessionCount := 0
	var totalBytes int64
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		sessionCount++
		if info, err := entry.Info(); err == nil {
			totalBytes += info.Size()
		}
	}
	if sessionCount == 0 {
		return contracts.Project{}, false
	}

	var displayName, absolutePath string
	if meta, ok := readWorkspaceMeta(root, hash); ok {
		absolutePath = decodeFolderURI(meta.Folder)
		displayName = projectDisplayName(absolutePath)
	}
	if displayName == "" {
		// Fall back to a shortened version of the hash so the user
		// has something readable when the workspace has no
		// metadata file or carries a non-file URI.
		displayName = "workspace-" + hash[:8]
	}

	return contracts.Project{
		ID:           contracts.ProjectID(hash),
		DisplayName:  displayName,
		Path:         absolutePath,
		SessionCount: sessionCount,
		SizeBytes:    totalBytes,
	}, true
}

// emptyWindowProject builds the synthetic "(no workspace)" Project
// that holds chats started in folder-less VS Code windows. The
// function returns ok=false when there are no such chats, so the
// project does not show up as an empty row.
func emptyWindowProject(root fs.FS) (contracts.Project, bool) {
	entries, err := fs.ReadDir(root, emptyWindowChatSessionsDir)
	if err != nil {
		return contracts.Project{}, false
	}
	sessionCount := 0
	var totalBytes int64
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		sessionCount++
		if info, err := entry.Info(); err == nil {
			totalBytes += info.Size()
		}
	}
	if sessionCount == 0 {
		return contracts.Project{}, false
	}
	return contracts.Project{
		ID:           emptyWindowProjectID,
		DisplayName:  emptyWindowDisplayName,
		SessionCount: sessionCount,
		SizeBytes:    totalBytes,
	}, true
}

// ListSessions returns one SessionSummary per chat in the given
// project. The lookup branches on whether the project is the
// synthetic empty-window bucket or a real workspace, because each
// case lives in a different directory.
func (p *Provider) ListSessions(root fs.FS, project contracts.ProjectID) ([]contracts.SessionSummary, error) {
	dir := chatDirForProject(project)
	entries, err := fs.ReadDir(root, dir)
	if err != nil {
		return nil, newError("list sessions", dir, err)
	}

	var summaries []contracts.SessionSummary
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		sessionFile := path.Join(dir, entry.Name())
		conv, err := readSessionFile(root, sessionFile, project, p.cached)
		if err != nil {
			return nil, newError("read session", sessionFile, err)
		}
		var size int64
		if info, err := entry.Info(); err == nil {
			size = info.Size()
		}
		summaries = append(summaries, contracts.SessionSummary{
			ID:           contracts.SessionID(strings.TrimSuffix(entry.Name(), ".jsonl")),
			Project:      project,
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

// chatDirForProject picks the right directory to look in based on
// whether the project ID is the synthetic empty-window marker or a
// real workspace hash.
func chatDirForProject(project contracts.ProjectID) string {
	if project == emptyWindowProjectID {
		return emptyWindowChatSessionsDir
	}
	return path.Join(workspaceStorageDir, string(project), chatSessionsDir)
}

// sessionTitle picks the best title for a session listing. Copilot
// sometimes records a custom title that the user typed (or that
// VS Code auto-generated from the first prompt). When that exists,
// we use it. Otherwise we fall back to the first user prompt.
func sessionTitle(conv contracts.Conversation) string {
	if conv.Title != "" {
		return conv.Title
	}
	return conv.FirstUserPrompt()
}

// ReadSession finds the session file by walking both the workspace
// chats and the empty-window chats. The walk takes one directory
// listing per workspace, plus one for the empty-window bucket.
// That cost is fine for the export and copy commands, which only
// read one session at a time.
func (p *Provider) ReadSession(root fs.FS, id contracts.SessionID) (contracts.Conversation, error) {
	if file, project, ok := locateInWorkspaces(root, id); ok {
		conv, err := readSessionFile(root, file, project, p.cached)
		if err != nil {
			return contracts.Conversation{}, newError("read session", file, err)
		}
		return conv, nil
	}
	if file, ok := locateInEmptyWindows(root, id); ok {
		conv, err := readSessionFile(root, file, emptyWindowProjectID, p.cached)
		if err != nil {
			return contracts.Conversation{}, newError("read session", file, err)
		}
		return conv, nil
	}
	// Unknown session id. We return the bare sentinel so the
	// caller's errors.Is checks see the type they expect.
	return contracts.Conversation{}, fs.ErrNotExist
}

// locateInWorkspaces walks every workspace looking for a session
// file whose name matches the given identifier. We return the file
// path and the owning project ID so the caller can build a
// Conversation with the right Project field.
func locateInWorkspaces(root fs.FS, id contracts.SessionID) (string, contracts.ProjectID, bool) {
	entries, err := fs.ReadDir(root, workspaceStorageDir)
	if err != nil {
		return "", "", false
	}
	for _, ws := range entries {
		if !ws.IsDir() {
			continue
		}
		candidate := path.Join(workspaceStorageDir, ws.Name(), chatSessionsDir, string(id)+".jsonl")
		if _, err := fs.Stat(root, candidate); err == nil {
			return candidate, contracts.ProjectID(ws.Name()), true
		}
	}
	return "", "", false
}

// locateInEmptyWindows looks for the session under
// globalStorage/emptyWindowChatSessions. Same shape as
// locateInWorkspaces, only one directory deep.
func locateInEmptyWindows(root fs.FS, id contracts.SessionID) (string, bool) {
	candidate := path.Join(emptyWindowChatSessionsDir, string(id)+".jsonl")
	if _, err := fs.Stat(root, candidate); err == nil {
		return candidate, true
	}
	return "", false
}

// Compile-time check: *Provider satisfies the base
// contracts.Provider interface. The optional capabilities
// declare their own assertions in the file where their
// methods live (cleanup.go for Cleaner, and so on), so a
// future contract change fails the build right next to the
// methods that need updating.
var _ contracts.Provider = (*Provider)(nil)
