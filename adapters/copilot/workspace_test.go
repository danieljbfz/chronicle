package copilot

import (
	"testing"
	"testing/fstest"
)

// TestReadWorkspaceMeta_returnsFolderURI walks one workspace and
// confirms we read the folder URI back out of workspace.json. The
// URI is what later steps decode into a friendly project name.
func TestReadWorkspaceMeta_returnsFolderURI(t *testing.T) {
	fsys := fstest.MapFS{
		"workspaceStorage/abc123/workspace.json": &fstest.MapFile{
			Data: []byte(`{"folder":"file:///Users/djbf/Desktop/work/claude-history"}`),
		},
	}
	meta, ok := readWorkspaceMeta(fsys, "abc123")
	if !ok {
		t.Fatal("expected workspace meta to be present")
	}
	if meta.Folder != "file:///Users/djbf/Desktop/work/claude-history" {
		t.Errorf("Folder = %q", meta.Folder)
	}
}

// TestReadWorkspaceMeta_missingReturnsNotOK confirms that a
// workspace folder without a workspace.json file produces ok=false
// rather than an error. Many VS Code workspace folders exist for
// extensions other than Copilot, and we want to skip them quietly.
func TestReadWorkspaceMeta_missingReturnsNotOK(t *testing.T) {
	fsys := fstest.MapFS{}
	if _, ok := readWorkspaceMeta(fsys, "abc123"); ok {
		t.Error("expected ok=false for missing workspace.json")
	}
}

// TestDecodeFolderURI_returnsAbsolutePath confirms the standard
// case: a "file:///..." URI becomes a clean absolute path with no
// scheme prefix and no URL escaping.
func TestDecodeFolderURI_returnsAbsolutePath(t *testing.T) {
	got := decodeFolderURI("file:///Users/djbf/Desktop/work/claude-history")
	want := "/Users/djbf/Desktop/work/claude-history"
	if got != want {
		t.Errorf("decodeFolderURI = %q, want %q", got, want)
	}
}

// TestDecodeFolderURI_handlesPercentEscapes confirms that VS Code's
// percent-encoded characters survive the decode round trip. A
// folder named "my project" becomes "file:///.../my%20project" and
// we must unescape it back.
func TestDecodeFolderURI_handlesPercentEscapes(t *testing.T) {
	got := decodeFolderURI("file:///tmp/my%20project")
	want := "/tmp/my project"
	if got != want {
		t.Errorf("decodeFolderURI = %q, want %q", got, want)
	}
}

// TestDecodeFolderURI_returnsEmptyForNonFile confirms we skip
// remote workspaces (vscode-remote://, etc.) instead of pretending
// to recognize them. Returning the empty string lets the caller
// fall back to the workspace hash for the display name.
func TestDecodeFolderURI_returnsEmptyForNonFile(t *testing.T) {
	if got := decodeFolderURI("vscode-remote://foo/bar"); got != "" {
		t.Errorf("non-file URI should decode to empty, got %q", got)
	}
}

// TestProjectDisplayName_takesTrailingComponent pins the rule that
// a project name is the trailing path component. Most users
// recognize their projects by their folder name, not by the full
// absolute path.
func TestProjectDisplayName_takesTrailingComponent(t *testing.T) {
	got := projectDisplayName("/Users/djbf/Desktop/work/claude-history")
	if got != "claude-history" {
		t.Errorf("projectDisplayName = %q, want %q", got, "claude-history")
	}
}
