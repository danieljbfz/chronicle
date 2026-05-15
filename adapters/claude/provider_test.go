package claude

import (
	"errors"
	"os"
	"testing"
	"testing/fstest"

	"github.com/danieljbfz/chronicle/contracts"
)

// mustReadFixture is a small helper that loads one fixture file and
// panics through the test framework if the read fails. We use it for
// the test-time setup paths where a missing fixture is a setup bug
// rather than a behaviour we want to assert against.
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

// TestProvider_ListSessions_sortedNewestFirst pins the sort order:
// sessions whose last activity is more recent come first in the
// listing. The user interface depends on this ordering to put the
// session the user is most likely to want at the top of the list.
func TestProvider_ListSessions_sortedNewestFirst(t *testing.T) {
	p := New()
	fsys := buildFS(t)
	summaries, err := p.ListSessions(fsys, "-Users-test-proj")
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 2 {
		t.Fatalf("got %d sessions, want 2", len(summaries))
	}
	if !summaries[0].LastActive.After(summaries[1].LastActive) &&
		!summaries[0].LastActive.Equal(summaries[1].LastActive) {
		t.Errorf("sessions should be sorted newest-first: %v then %v",
			summaries[0].LastActive, summaries[1].LastActive)
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

// TestProvider_PlanDeleteReturnsNotImplemented pins the temporary
// behaviour of the cleanup stubs. The composition layer in this
// plan should never call PlanDelete in production, but if it ever
// does, the error has to be the sentinel so callers can branch on
// it cleanly with errors.Is. The check protects us against anyone
// silently changing the stub to return nil, which would let
// destructive code reach the rest of chronicle before the trash
// subsystem exists to catch it.
func TestProvider_PlanDeleteReturnsNotImplemented(t *testing.T) {
	p := New()
	fsys := buildFS(t)
	_, err := p.PlanDelete(fsys, "small-session-1")
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("PlanDelete err = %v, want ErrNotImplemented", err)
	}
}
