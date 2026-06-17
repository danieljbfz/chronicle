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
// when deleting a project's memory.
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

// TestListGlobalMemory_findsClaudeMdAtRoot pins the canonical
// global file. When ~/.claude/CLAUDE.md is on disk, the
// listing should return one entry that names it and carries
// the right size and modification time. This is the file
// Claude reads at the start of every session, so missing it
// in the listing would leave the user blind to the most
// important memory file in their tree.
func TestListGlobalMemory_findsClaudeMdAtRoot(t *testing.T) {
	fsys := fstest.MapFS{
		"CLAUDE.md": {Data: []byte("# global rules\n")},
	}
	entries, err := New().ListGlobalMemory(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1 (just CLAUDE.md)", len(entries))
	}
	if entries[0].FileName != "CLAUDE.md" {
		t.Errorf("filename = %q, want CLAUDE.md", entries[0].FileName)
	}
	if entries[0].SizeBytes == 0 {
		t.Error("size should be greater than zero")
	}
}

// TestListGlobalMemory_emptyTreeReturnsNoEntries covers the
// fresh-install case. A user who has never created a global
// CLAUDE.md should get an empty slice and a nil error, not
// a confusing "no such file" surfaced from deep inside.
func TestListGlobalMemory_emptyTreeReturnsNoEntries(t *testing.T) {
	entries, err := New().ListGlobalMemory(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("entries = %d, want 0 for empty tree", len(entries))
	}
}

// TestListGlobalMemory_skipsDirectoryShapedEntries handles
// the unlikely-but-possible case where a CLAUDE.md
// directory has been created at the root. The listing
// should treat it as "no global memory" rather than crash
// on the next read attempt.
func TestListGlobalMemory_skipsDirectoryShapedEntries(t *testing.T) {
	fsys := fstest.MapFS{
		"CLAUDE.md/inner.txt": {Data: []byte("oops")},
	}
	entries, err := New().ListGlobalMemory(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("entries = %d, want 0 (CLAUDE.md is a directory in this fixture)", len(entries))
	}
}

// TestGlobalMemoryFilePath_returnsTheNameUnchanged pins the
// path-resolution rule. Global memory lives at the top of
// the chronicle root, so the relative path is the filename
// itself with no directory prefix. This is the contract
// composition relies on when joining the result with the
// provider's absolute root.
func TestGlobalMemoryFilePath_returnsTheNameUnchanged(t *testing.T) {
	got := New().GlobalMemoryFilePath("CLAUDE.md")
	if got != "CLAUDE.md" {
		t.Errorf("GlobalMemoryFilePath = %q, want CLAUDE.md", got)
	}
}

// TestDefaultGlobalMemoryFile_returnsClaudeMd pins the
// adapter's declared default. The CLI dispatches into this
// when the user runs `chronicle memory show --global`
// without naming a file. Pinning the value here means the
// CLI's --global default cannot drift away from what Claude
// itself reads on disk without the test catching it.
func TestDefaultGlobalMemoryFile_returnsClaudeMd(t *testing.T) {
	if got := New().DefaultGlobalMemoryFile(); got != "CLAUDE.md" {
		t.Errorf("DefaultGlobalMemoryFile = %q, want CLAUDE.md", got)
	}
}

// TestPlanDeleteGlobalMemory_picksUpTheKnownFile confirms a
// fixture with CLAUDE.md produces a plan with one item.
// Categories matter for the trash listing later, so we
// also pin the category string.
func TestPlanDeleteGlobalMemory_picksUpTheKnownFile(t *testing.T) {
	fsys := fstest.MapFS{
		"CLAUDE.md": {Data: []byte("# global rules\n")},
	}
	plan, err := New().PlanDeleteGlobalMemory(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Category != "claude-global-memory" {
		t.Errorf("category = %q, want claude-global-memory", plan.Category)
	}
	if len(plan.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(plan.Items))
	}
	if plan.Items[0].Path != "CLAUDE.md" {
		t.Errorf("path = %q, want CLAUDE.md", plan.Items[0].Path)
	}
}

// TestPlanDeleteGlobalMemory_emptyTreeReturnsEmptyPlan
// confirms the no-file case. The plan should be a
// well-formed value with zero items and the right category,
// not an error: "delete global memory" when there is none
// is a no-op, not a failure.
func TestPlanDeleteGlobalMemory_emptyTreeReturnsEmptyPlan(t *testing.T) {
	plan, err := New().PlanDeleteGlobalMemory(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Items) != 0 {
		t.Errorf("items = %d, want 0", len(plan.Items))
	}
	if plan.Category != "claude-global-memory" {
		t.Errorf("category = %q, want claude-global-memory even on empty plan", plan.Category)
	}
}
