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
// chronicle process, and the type is mostly stateless: the only
// thing it caches is the StorageVersion that Detect produced, so we
// avoid re-fingerprinting every call. Methods that need the cached
// version use it through the cached field; methods that do not (the
// listing methods, for example) ignore it.
type Provider struct {
	cached  contracts.StorageVersion
	cacheOK bool
}

// New returns a ready-to-use Provider. We do not eagerly call Detect
// here, because the constructor should not do disk I/O. The first
// caller that asks for the storage version pays the detection cost,
// and every caller after that gets the cached value.
func New() *Provider { return &Provider{} }

// Name returns the adapter's stable identifier. The string is
// referenced from the registry, the doctor view, and the JSON output
// of the list command, so we go through the constant declared in
// detect.go rather than duplicating the literal here.
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
// projects directory. The function is the natural building block for
// the user interface's "show me everything" view, and the cleanup
// commands use the same listing as the starting point for
// orphan-scanning. We sort the result by display name so the user
// sees a stable order that does not depend on the operating
// system's directory iteration order.
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

// ListSessions returns the SessionSummary slice for one project.
// We deliberately read every session file end to end here, because
// the summary needs the first user prompt and the timestamps, both
// of which only become available after parsing the file. The
// listing pages of the UI cache these summaries so the cost is paid
// once per session per chronicle invocation.
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

// ReadSession finds the session file by walking the projects tree
// and parses it. The cost is one directory walk plus one file read,
// which is fine for the export and copy commands that only ever
// touch one session. If a future view ever wants to bulk-read many
// sessions, it should call ListSessions first and then ReadSession
// per identifier.
func (p *Provider) ReadSession(root fs.FS, id contracts.SessionID) (contracts.Conversation, error) {
	file, err := locateSessionFile(root, id)
	if err != nil {
		return contracts.Conversation{}, err
	}
	return readSessionFile(root, file, p.cached)
}

// locateSessionFile walks the projects tree looking for a .jsonl file
// whose stem matches the session identifier. We accept the linear
// scan because session identifiers are UUIDs and a Claude install
// rarely has more than a few hundred of them — the walk is fast
// enough that an index would be over-engineering at this stage.
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

// PlanDelete and PlanOrphanScan delegate to the stubs in
// cleanup_stub.go for now. The real cascade-aware implementations
// land in a later plan. The split exists so the destructive code
// paths simply do not exist yet — we cannot accidentally delete
// anything when the function returns ErrNotImplemented.
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
