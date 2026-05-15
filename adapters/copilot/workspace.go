package copilot

import (
	"encoding/json"
	"io/fs"
	"net/url"
	"path"
	"strings"
)

// workspaceStorageDir is the subdirectory under the Copilot root
// that holds one folder per workspace. Each workspace folder is
// named after an opaque hash that VS Code generates from the
// workspace's folder URI. We resolve the hash back to the folder
// path through the workspace.json file inside the same folder.
const workspaceStorageDir = "workspaceStorage"

// workspaceMetadataFile is the small JSON file inside each
// workspace folder that records the URI of the folder VS Code was
// opened on. The file shape is a single object with one key:
//
//	{ "folder": "file:///Users/djbf/Desktop/work/claude-history" }
const workspaceMetadataFile = "workspace.json"

// chatSessionsDir is the subdirectory inside each workspace folder
// that contains one .jsonl file per Copilot chat session. Sessions
// live there from the moment the user starts a chat in the panel
// until the user explicitly deletes them through the VS Code UI.
const chatSessionsDir = "chatSessions"

// emptyWindowChatSessionsDir is the directory that holds chat
// sessions started in VS Code windows that were not opened on a
// folder. These chats have no workspace, so they live under
// globalStorage rather than under any workspaceStorage subfolder.
const emptyWindowChatSessionsDir = "globalStorage/emptyWindowChatSessions"

// workspaceMeta is what we read out of one workspace.json file. We
// only care about the folder field today. VS Code may add other
// fields in future versions and we ignore them.
type workspaceMeta struct {
	Folder string `json:"folder"`
}

// readWorkspaceMeta opens the workspace.json file inside one
// workspace folder and returns the parsed metadata. A missing or
// malformed file means the workspace folder exists for some other
// VS Code feature, not for chat, so we treat both as "no metadata"
// instead of as an error. The caller decides what to do with that.
func readWorkspaceMeta(root fs.FS, hash string) (workspaceMeta, bool) {
	f, err := root.Open(path.Join(workspaceStorageDir, hash, workspaceMetadataFile))
	if err != nil {
		return workspaceMeta{}, false
	}
	defer f.Close()
	var meta workspaceMeta
	if err := json.NewDecoder(f).Decode(&meta); err != nil {
		return workspaceMeta{}, false
	}
	return meta, true
}

// decodeFolderURI turns a "file:///Users/djbf/Desktop/work/claude-history"
// URI into the absolute path "/Users/djbf/Desktop/work/claude-history".
// VS Code always writes file URIs through Node's URL helper, so the
// shape is reliable and we use the standard library to undo it.
//
// We return the empty string for a URI we cannot decode. Today that
// only happens for non-file URIs, which VS Code emits for things
// like remote workspaces. A later release of chronicle can support
// those, but for now we just skip them.
func decodeFolderURI(uri string) string {
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "file" {
		return ""
	}
	return parsed.Path
}

// projectDisplayName turns an absolute workspace path into the
// short name we show in the UI. The trailing path component is
// what most users recognize as their project. For
// "/Users/djbf/Desktop/work/claude-history" we return
// "claude-history".
func projectDisplayName(absPath string) string {
	if absPath == "" {
		return ""
	}
	if i := strings.LastIndex(absPath, "/"); i >= 0 && i+1 < len(absPath) {
		return absPath[i+1:]
	}
	return absPath
}

// emptyWindowProjectID and emptyWindowDisplayName are the synthetic
// identifiers we use for the bucket of chat sessions that VS Code
// records when no folder was open. The user sees these grouped
// under one "no workspace" pseudo-project in the listing.
const (
	emptyWindowProjectID    = "__empty_window__"
	emptyWindowDisplayName  = "(no workspace)"
)
