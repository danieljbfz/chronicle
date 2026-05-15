package claude

import (
	"errors"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/danieljbfz/chronicle/contracts"
)

// Provider is the Claude adapter. Composition keeps one instance per
// chronicle process. The type is mostly stateless. The only thing it
// hangs on to is the StorageVersion that Detect produced, so we do
// not re-fingerprint on every call. Methods that need the cached
// version read it from the cached field. Methods that do not, like
// the plain listing methods, ignore it.
type Provider struct {
	cached  contracts.StorageVersion
	cacheOK bool
}

// New returns a ready-to-use Provider. The constructor stays
// I/O-free on purpose, so we do not call Detect here. The first
// caller that asks for the storage version is the one that triggers
// the disk read, and everyone after that gets the cached result.
func New() *Provider { return &Provider{} }

// Name returns the adapter's stable identifier. The same string
// shows up in the registry, in the doctor view, and in the JSON
// output of the list command, so we read it from the constant
// declared in detect.go instead of repeating the literal here.
func (*Provider) Name() string { return adapterName }

// Detect returns the StorageVersion for the given root, computing
// the fingerprint on the first call and serving from the cache on
// every later call. The cache lives on the Provider value rather
// than as package-level state because the Provider is the natural
// owner of "what did we see when we looked at this user's data?"
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

// ListProjects returns one Project per subdirectory under the
// projects directory. The user interface uses this for the
// "show me everything" view, and the cleanup commands use the same
// listing as the starting point for their orphan scan. We sort by
// display name so the order stays stable across runs, regardless of
// how the operating system happens to iterate the directory.
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

// summarizeProject produces a Project summary for one subdirectory
// of projects. We count the .jsonl files and sum their sizes, but
// we do not parse them, because the listing view should be cheap
// even when the user has thousands of sessions.
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

// decodeProjectName turns Claude's encoded directory name back into
// the trailing path component the user actually recognizes. Claude
// stores a directory called "-Users-djbf-Desktop-work-claude-history"
// for the project at /Users/djbf/Desktop/work/claude-history, so the
// trailing token "claude-history" is what most users want to see in
// the project listing.
//
// The decoding is heuristic, because Claude's encoding loses
// information: a real path that contained a literal hyphen
// component looks identical to one that did not. We accept the
// ambiguity and keep the full reconstructed path in Project.Path so
// power users can still distinguish edge cases.
func decodeProjectName(folderName string) string {
	p := decodeProjectPath(folderName)
	if i := strings.LastIndex(p, "/"); i >= 0 && i+1 < len(p) {
		return p[i+1:]
	}
	return folderName
}

// decodeProjectPath reverses Claude's encoding back into the
// best-effort absolute path. The encoding turns every forward slash
// into a hyphen and prepends a leading hyphen for the root slash, so
// our reverse turns the leading hyphen back into the root slash and
// every other hyphen back into a slash. This is wrong for paths
// whose components legitimately contained hyphens, and the
// docstring of decodeProjectName explains why we accept the loss.
func decodeProjectPath(folderName string) string {
	if strings.HasPrefix(folderName, "-") {
		return "/" + strings.ReplaceAll(folderName[1:], "-", "/")
	}
	return folderName
}

// ListSessions returns one summary per session in a project. We
// read every session file end to end inside this function. The
// summary needs the first user prompt and the timestamps, and
// neither of those is available until we have parsed the file.
//
// Reading every file sounds expensive, but it only happens once
// per session per chronicle run. The user interface caches the
// summaries it gets back, so opening the same listing twice does
// not pay the cost twice.
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

// ReadSession finds one session by walking the projects tree and
// returns the parsed Conversation. The walk takes one directory
// listing plus one file read. That is cheap enough for the export
// and copy commands, which only ever touch one session at a time.
//
// A future view that needs to read many sessions in bulk should
// call ListSessions first to get all the identifiers, then call
// ReadSession on each one. Doing the walk per identifier would
// repeat work the listing already did.
func (p *Provider) ReadSession(root fs.FS, id contracts.SessionID) (contracts.Conversation, error) {
	file, err := locateSessionFile(root, id)
	if err != nil {
		return contracts.Conversation{}, err
	}
	return readSessionFile(root, file, p.cached)
}

// locateSessionFile walks the projects tree and returns the path
// of the .jsonl file whose name matches the session identifier. We
// scan the tree linearly rather than building an index. Session
// identifiers are UUIDs, and a Claude install almost never has
// more than a few hundred of them, so the walk is fast enough.
// Building an index would be more code to maintain for no real
// gain at this scale.
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

// PlanDelete and PlanOrphanScan call the stubs in cleanup_stub.go
// for now. The real cascade-aware versions land in a later plan,
// once the trash subsystem exists to back them up.
//
// Keeping the destructive paths as stubs is a safety choice. The
// stub returns ErrNotImplemented and that is all it does. There is
// no code in chronicle today that can accidentally delete anything,
// because the code that would do the deleting has not been written
// yet.
func (p *Provider) PlanDelete(root fs.FS, id contracts.SessionID) (contracts.DeletePlan, error) {
	return planDeleteStub(root, id)
}

func (p *Provider) PlanOrphanScan(root fs.FS) (contracts.DeletePlan, error) {
	return planOrphanScanStub(root)
}

// Compile-time check: *Provider satisfies contracts.Provider. The
// blank identifier discards the value, and the type annotation
// forces the compiler to verify the relationship. If we ever add a
// method to the interface or change a signature, the build fails
// right here with an error that names the missing method.
var _ contracts.Provider = (*Provider)(nil)
