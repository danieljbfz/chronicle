package claude

import (
	"os"
	"testing"
	"testing/fstest"

	"github.com/danieljbfz/chronicle/contracts"
)

// mustReadFixture is a small helper that loads one fixture file and
// panics through the test framework if the read fails. We use it
// for the test-time setup paths where a missing fixture is a setup
// bug, not a behaviour we want to assert against.
func mustReadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/v1_0/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// buildFS lays out a fake ~/.claude with two projects and three
// sessions, mixing one of each fixture across them. This is enough
// surface area to cover every Provider method without making the
// test setup painful to read.
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

// TestProvider_ListProjects confirms the listing is grouped by
// directory and that the per-project session count adds up. The
// fixture has two projects with two and one sessions respectively,
// so the total has to be three.
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

// TestProvider_ListSessionRefs_enumeratesAndSummarizes confirms the
// cheap enumeration returns one ref per session with a usable id and
// locator, and that summarizing each ref yields a summary for the same
// session. The composition layer relies on this split to cache the
// summaries and parse only the sessions whose files changed.
func TestProvider_ListSessionRefs_enumeratesAndSummarizes(t *testing.T) {
	p := New()
	fsys := buildFS(t)
	refs, err := p.ListSessionRefs(fsys, "-Users-test-proj")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 {
		t.Fatalf("got %d refs, want 2", len(refs))
	}
	for _, ref := range refs {
		if ref.ID == "" || ref.Locator == "" {
			t.Fatalf("ref missing id or locator: %+v", ref)
		}
		summary, err := p.SummarizeSession(fsys, ref)
		if err != nil {
			t.Fatalf("summarize %s: %v", ref.ID, err)
		}
		if summary.ID != ref.ID {
			t.Fatalf("summary id %q != ref id %q", summary.ID, ref.ID)
		}
	}
}

// TestProvider_ReadSession_findsAcrossProjects confirms the
// linear-scan lookup finds a session no matter which project it
// lives in. The export and copy commands rely on this: the user
// passes a session identifier and chronicle is expected to find it
// without making the user remember the project name.
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

// TestProvider_implementsCleaner pins the fact that the Claude
// adapter supports cascade-aware cleanup. If anyone ever removes
// one of the Cleaner methods (PlanDelete or PlanOrphanScan), the
// type assertion fails and the test catches the regression
// before any destructive code ships.
func TestProvider_implementsCleaner(t *testing.T) {
	var p any = New()
	if _, ok := p.(contracts.Cleaner); !ok {
		t.Error("*Provider should satisfy contracts.Cleaner")
	}
}
