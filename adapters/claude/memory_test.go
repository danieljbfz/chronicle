package claude

import (
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"
)

// TestListMemories_findsFilesAcrossProjects walks two
// projects, one with a memory directory and one without. The
// function should return entries only for the project that
// actually has memory files, sorted MEMORY.md first because
// uppercase sorts before lowercase.
func TestListMemories_findsFilesAcrossProjects(t *testing.T) {
	fsys := fstest.MapFS{
		"projects/-Users-test-projA/" + validUUID + ".jsonl": {Data: []byte(`{}`)},
		"projects/-Users-test-projA/memory/MEMORY.md":        {Data: []byte("# index")},
		"projects/-Users-test-projA/memory/architecture.md":  {Data: []byte("# arch notes")},
		"projects/-Users-test-projB/" + otherUUID + ".jsonl": {Data: []byte(`{}`)},
	}
	entries, err := New().ListMemories(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2 (MEMORY.md and architecture.md)", len(entries))
	}
	if entries[0].FileName != "MEMORY.md" {
		t.Errorf("first entry = %q, want MEMORY.md (uppercase sorts first)", entries[0].FileName)
	}
	if entries[1].FileName != "architecture.md" {
		t.Errorf("second entry = %q, want architecture.md", entries[1].FileName)
	}
	if entries[0].Project != "-Users-test-projA" {
		t.Errorf("project = %q, want the projA encoded id", entries[0].Project)
	}
}

// TestListMemories_emptyTreeReturnsNoEntries covers the
// no-projects case. A brand-new chronicle install should not
// crash on an empty tree, just return an empty slice.
func TestListMemories_emptyTreeReturnsNoEntries(t *testing.T) {
	entries, err := New().ListMemories(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("entries = %d, want 0 for empty tree", len(entries))
	}
}

// TestListMemories_skipsNonMarkdownFiles makes sure we only
// surface .md files to the user. If something else lands in
// the memory directory, it is not Claude's auto-memory and we
// should leave it for the user to investigate manually.
func TestListMemories_skipsNonMarkdownFiles(t *testing.T) {
	fsys := fstest.MapFS{
		"projects/-Users-test/memory/MEMORY.md": {Data: []byte("# index")},
		"projects/-Users-test/memory/notes.txt": {Data: []byte("not memory")},
		"projects/-Users-test/memory/.DS_Store": {Data: []byte("macos junk")},
	}
	entries, err := New().ListMemories(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("entries = %d, want 1 (only the .md file)", len(entries))
	}
}

// TestMemoryFilePath builds the right path string for a known
// project and filename. The CLI uses this to find the file on
// disk for editing or showing. Pinning the format here means
// no caller has to assemble the path by hand.
func TestMemoryFilePath(t *testing.T) {
	got := New().MemoryFilePath("-Users-test-proj", "MEMORY.md")
	want := "projects/-Users-test-proj/memory/MEMORY.md"
	if got != want {
		t.Errorf("MemoryFilePath = %q, want %q", got, want)
	}
}

// TestPlanDeleteProjectMemory_includesAllFiles confirms a
// project with three memory files produces a plan with three
// items. The test guards against a regression where someone
// changes the function to skip files based on the wrong
// criterion, which would silently leave memory files behind
// after the user said "delete this project's memory."
func TestPlanDeleteProjectMemory_includesAllFiles(t *testing.T) {
	fsys := fstest.MapFS{
		"projects/-Users-test/memory/MEMORY.md":       {Data: []byte("# idx")},
		"projects/-Users-test/memory/debugging.md":    {Data: []byte("# dbg")},
		"projects/-Users-test/memory/architecture.md": {Data: []byte("# arch")},
	}
	plan, err := New().PlanDeleteProjectMemory(fsys, "-Users-test")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Category != "claude-memory" {
		t.Errorf("category = %q, want claude-memory", plan.Category)
	}
	if len(plan.Items) != 3 {
		t.Errorf("items = %d, want 3", len(plan.Items))
	}
}

// TestPlanDeleteProjectMemory_missingProjectIsAnError keeps
// the contract tight. A caller that asks to delete the memory
// of a project with no memory directory gets an explicit
// error, not a silently empty plan, because the typical
// reason for asking is "I expected memory here, why is there
// none?" — and a misleading silent success would hide the
// underlying problem.
func TestPlanDeleteProjectMemory_missingProjectIsAnError(t *testing.T) {
	_, err := New().PlanDeleteProjectMemory(fstest.MapFS{}, "-Users-no-such-project")
	if err == nil {
		t.Error("expected an error for a project with no memory dir")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want one wrapping fs.ErrNotExist", err)
	}
}
