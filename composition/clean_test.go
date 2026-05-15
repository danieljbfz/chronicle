package composition

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/danieljbfz/chronicle/contracts"
	"github.com/danieljbfz/chronicle/internal/config"
	"github.com/danieljbfz/chronicle/internal/paths"
)

// cleanerFake is a Provider that also implements Cleaner.
// We use it inside this test file to exercise the cleanup
// orchestrator without going through the full registry. The
// composition layer only ever interacts with providers
// through the contract interfaces, so a fake that satisfies
// the same interfaces is a faithful stand-in.
//
// Each method either returns canned data or routes back to a
// hook the test can set, which lets one fake serve every
// branch of clean.go without a separate type per case.
type cleanerFake struct {
	name     string
	projects []contracts.Project
	sessions map[contracts.ProjectID][]contracts.SessionSummary
	convos   map[contracts.SessionID]contracts.Conversation
	plans    map[contracts.SessionID]contracts.DeletePlan
	orphans  contracts.DeletePlan
}

func (f *cleanerFake) Name() string { return f.name }
func (f *cleanerFake) Detect(fs.FS) (contracts.StorageVersion, error) {
	return contracts.StorageVersion{}, nil
}
func (f *cleanerFake) ListProjects(fs.FS) ([]contracts.Project, error) {
	return f.projects, nil
}
func (f *cleanerFake) ListSessions(_ fs.FS, p contracts.ProjectID) ([]contracts.SessionSummary, error) {
	return f.sessions[p], nil
}
func (f *cleanerFake) ReadSession(_ fs.FS, id contracts.SessionID) (contracts.Conversation, error) {
	c, ok := f.convos[id]
	if !ok {
		return contracts.Conversation{}, fs.ErrNotExist
	}
	return c, nil
}
func (f *cleanerFake) PlanDelete(_ fs.FS, id contracts.SessionID) (contracts.DeletePlan, error) {
	plan, ok := f.plans[id]
	if !ok {
		return contracts.DeletePlan{}, fs.ErrNotExist
	}
	return plan, nil
}
func (f *cleanerFake) PlanOrphanScan(fs.FS) (contracts.DeletePlan, error) {
	return f.orphans, nil
}

// newCleanTestApp builds an App that wraps one cleanerFake
// with a real (temporary) provider root and trash directory.
// We want a real data root because PlanCleanup eventually
// calls Trash, which moves files to disk, and Trash needs a
// real filesystem to do its job. The fake stands in only for
// the Provider methods. The trash side stays on real disk.
func newCleanTestApp(t *testing.T, fake *cleanerFake) (*App, string) {
	t.Helper()
	dataRoot := t.TempDir()
	trashRoot := t.TempDir()
	a := &App{
		settings: config.Defaults(),
		locations: paths.Locations{
			TrashDir: trashRoot,
		},
		providers: []*providerEntry{{
			Provider: fake,
			Root:     dataRoot,
			FS:       os.DirFS(dataRoot),
		}},
	}
	return a, dataRoot
}

// TestPlanCleanup_abandonedReturnsOnePlanPerAbandonedSession
// is the happy path for the abandoned-cleanup category. The
// fake reports three sessions in one project. Two are
// abandoned (zero real user prompts), one has a real prompt.
// PlanCleanup should return two plans, one per abandoned
// session, and zero for the live one.
func TestPlanCleanup_abandonedReturnsOnePlanPerAbandonedSession(t *testing.T) {
	fake := &cleanerFake{
		name:     "claude",
		projects: []contracts.Project{{ID: "p1"}},
		sessions: map[contracts.ProjectID][]contracts.SessionSummary{
			"p1": {{ID: "s1"}, {ID: "s2"}, {ID: "s3"}},
		},
		convos: map[contracts.SessionID]contracts.Conversation{
			// s1 and s2 have no real user prompts — abandoned.
			"s1": {},
			"s2": {Messages: []contracts.Message{{Role: contracts.RoleUser, IsMeta: true}}},
			// s3 has one real prompt — alive.
			"s3": {Messages: []contracts.Message{{
				Role:   contracts.RoleUser,
				Blocks: []contracts.Block{contracts.TextBlock{Text: "hi"}},
			}}},
		},
		plans: map[contracts.SessionID]contracts.DeletePlan{
			"s1": {SessionID: "s1", Items: []contracts.DeleteItem{{Path: "s1.jsonl"}}},
			"s2": {SessionID: "s2", Items: []contracts.DeleteItem{{Path: "s2.jsonl"}}},
		},
	}
	a, _ := newCleanTestApp(t, fake)

	planned, err := a.PlanCleanup([]CleanCategory{CategoryAbandoned}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(planned) != 2 {
		t.Errorf("planned = %d, want 2 (s1 and s2)", len(planned))
	}
}

// TestPlanCleanup_orphansReturnsOnePlanPerProviderWithOrphans
// confirms the orphans category aggregates one plan per
// provider that has orphan items. An empty plan (zero items)
// is dropped because there is nothing for the user to review,
// which keeps the dry-run output focused on real work.
func TestPlanCleanup_orphansReturnsOnePlanPerProviderWithOrphans(t *testing.T) {
	fake := &cleanerFake{
		name: "claude",
		orphans: contracts.DeletePlan{
			Category: "claude-orphans",
			Items:    []contracts.DeleteItem{{Path: "file-history/abc"}},
		},
	}
	a, _ := newCleanTestApp(t, fake)

	planned, err := a.PlanCleanup([]CleanCategory{CategoryOrphans}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(planned) != 1 {
		t.Errorf("planned = %d, want 1 (one provider, one orphan plan)", len(planned))
	}
}

// TestPlanCleanup_orphansSkipsEmptyPlans covers the edge
// case where the orphan scan finds nothing. An empty plan
// must not appear in the result, because surfacing "this
// provider has zero orphans" as a separate item would just
// be noise in the dry-run output.
func TestPlanCleanup_orphansSkipsEmptyPlans(t *testing.T) {
	fake := &cleanerFake{name: "claude"} // no orphan items
	a, _ := newCleanTestApp(t, fake)

	planned, err := a.PlanCleanup([]CleanCategory{CategoryOrphans}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(planned) != 0 {
		t.Errorf("planned = %d, want 0 for empty orphan scan", len(planned))
	}
}

// TestPlanCleanup_providerFilterScopesToOneTool confirms the
// providerName argument limits the result to the named
// provider. The CLI exposes this through `chronicle clean
// abandoned --provider claude`. A user with several tools
// installed should be able to clean one at a time.
func TestPlanCleanup_providerFilterScopesToOneTool(t *testing.T) {
	claudeFake := &cleanerFake{
		name:     "claude",
		projects: []contracts.Project{{ID: "p1"}},
		sessions: map[contracts.ProjectID][]contracts.SessionSummary{
			"p1": {{ID: "s1"}},
		},
		convos: map[contracts.SessionID]contracts.Conversation{"s1": {}},
		plans: map[contracts.SessionID]contracts.DeletePlan{
			"s1": {SessionID: "s1", Items: []contracts.DeleteItem{{Path: "s1.jsonl"}}},
		},
	}
	copilotFake := &cleanerFake{
		name:     "copilot",
		projects: []contracts.Project{{ID: "p2"}},
		sessions: map[contracts.ProjectID][]contracts.SessionSummary{
			"p2": {{ID: "s2"}},
		},
		convos: map[contracts.SessionID]contracts.Conversation{"s2": {}},
		plans: map[contracts.SessionID]contracts.DeletePlan{
			"s2": {SessionID: "s2", Items: []contracts.DeleteItem{{Path: "s2.jsonl"}}},
		},
	}
	a := &App{
		settings:  config.Defaults(),
		locations: paths.Locations{TrashDir: t.TempDir()},
		providers: []*providerEntry{
			{Provider: claudeFake, Root: t.TempDir()},
			{Provider: copilotFake, Root: t.TempDir()},
		},
	}

	planned, err := a.PlanCleanup([]CleanCategory{CategoryAbandoned}, "claude")
	if err != nil {
		t.Fatal(err)
	}
	if len(planned) != 1 {
		t.Fatalf("planned = %d, want 1 (only claude)", len(planned))
	}
	if planned[0].ProviderName() != "claude" {
		t.Errorf("provider = %q, want claude", planned[0].ProviderName())
	}
}

// TestPlannedDeletion_ProviderNameAndRoot pins the small
// accessor methods. They look trivial, but they are part of
// the public CLI surface — the user sees these values in the
// dry-run output, so any drift would change what the user
// reads.
func TestPlannedDeletion_ProviderNameAndRoot(t *testing.T) {
	fake := &cleanerFake{name: "claude"}
	root := "/some/where"
	pd := PlannedDeletion{
		provider: &providerEntry{Provider: fake, Root: root},
		Plan:     contracts.DeletePlan{SessionID: "s1"},
	}
	if got := pd.ProviderName(); got != "claude" {
		t.Errorf("ProviderName = %q, want claude", got)
	}
	if got := pd.ProviderRoot(); got != root {
		t.Errorf("ProviderRoot = %q, want %q", got, root)
	}

	// The nil-provider case happens when PlannedDeletion is
	// constructed from a zero value. Both accessors should
	// return empty strings instead of panicking.
	var empty PlannedDeletion
	if empty.ProviderName() != "" {
		t.Error("zero-value PlannedDeletion should have empty ProviderName")
	}
	if empty.ProviderRoot() != "" {
		t.Error("zero-value PlannedDeletion should have empty ProviderRoot")
	}
}

// TestExecuteCleanup_movesEveryPlanItemIntoTrash is the
// integration test for the full execute flow. We hand
// ExecuteCleanup a slice of PlannedDeletions backed by real
// files on disk, then assert that every file moved into the
// trash and that the trash listing reports the entries we
// expected.
func TestExecuteCleanup_movesEveryPlanItemIntoTrash(t *testing.T) {
	fake := &cleanerFake{name: "claude"}
	a, dataRoot := newCleanTestApp(t, fake)

	// Drop two real files into the data root so the move has
	// something to relocate.
	for _, name := range []string{"a.jsonl", "b.jsonl"} {
		if err := os.WriteFile(filepath.Join(dataRoot, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	planned := []PlannedDeletion{
		{provider: a.providers[0], Plan: contracts.DeletePlan{
			SessionID: "s1",
			Items:     []contracts.DeleteItem{{Path: "a.jsonl"}},
		}},
		{provider: a.providers[0], Plan: contracts.DeletePlan{
			SessionID: "s2",
			Items:     []contracts.DeleteItem{{Path: "b.jsonl"}},
		}},
	}

	entries, err := a.ExecuteCleanup(planned)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}

	// Originals are gone, trash entries exist.
	for _, name := range []string{"a.jsonl", "b.jsonl"} {
		if _, err := os.Stat(filepath.Join(dataRoot, name)); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s should be gone after ExecuteCleanup, err=%v", name, err)
		}
	}

	listed, err := a.TrashList()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 {
		t.Errorf("trash list = %d, want 2", len(listed))
	}
}

// TestExecuteCleanup_skipsEmptyPlans confirms that a plan
// with zero items is silently dropped, not turned into an
// empty trash entry. An empty trash entry would just be
// noise in `chronicle trash list`.
func TestExecuteCleanup_skipsEmptyPlans(t *testing.T) {
	fake := &cleanerFake{name: "claude"}
	a, _ := newCleanTestApp(t, fake)

	planned := []PlannedDeletion{
		{provider: a.providers[0], Plan: contracts.DeletePlan{SessionID: "empty"}},
	}
	entries, err := a.ExecuteCleanup(planned)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("entries = %d, want 0 for an empty plan", len(entries))
	}
}

// TestPlanCleanup_unknownCategoryReturnsError pins the error
// path for the category dispatcher. The CLI does not expose
// arbitrary category strings, but the contract is worth
// keeping precise so a future feature does not silently
// produce no plans when it asks for an unknown one.
func TestPlanCleanup_unknownCategoryReturnsError(t *testing.T) {
	a, _ := newCleanTestApp(t, &cleanerFake{name: "claude"})
	_, err := a.PlanCleanup([]CleanCategory{"not-a-real-category"}, "")
	if err == nil {
		t.Error("expected an error for an unknown clean category")
	}
}

// TestPlanCleanup_defaultsToAbandonedWhenNoCategoryPassed
// confirms the empty-categories shortcut. Today the only
// implemented category is "abandoned", so passing nothing
// should run that one. The contract matters because the
// "default category" set is what governs the future
// `chronicle clean` (no args) command if we ever add one.
func TestPlanCleanup_defaultsToAbandonedWhenNoCategoryPassed(t *testing.T) {
	fake := &cleanerFake{
		name:     "claude",
		projects: []contracts.Project{{ID: "p1"}},
		sessions: map[contracts.ProjectID][]contracts.SessionSummary{
			"p1": {{ID: "s1"}},
		},
		convos: map[contracts.SessionID]contracts.Conversation{"s1": {}},
		plans: map[contracts.SessionID]contracts.DeletePlan{
			"s1": {SessionID: "s1", Items: []contracts.DeleteItem{{Path: "s1.jsonl"}}},
		},
	}
	a, _ := newCleanTestApp(t, fake)

	planned, err := a.PlanCleanup(nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(planned) != 1 {
		t.Errorf("planned = %d, want 1 (abandoned should run by default)", len(planned))
	}
}

// TestPlanCleanup_skipsNonCleanerProvider confirms that a
// provider which does not implement contracts.Cleaner is
// silently skipped instead of causing an error. The base
// Provider interface does not include cleanup, so this is
// the path a future read-only adapter takes.
func TestPlanCleanup_skipsNonCleanerProvider(t *testing.T) {
	a := &App{
		settings:  config.Defaults(),
		locations: paths.Locations{TrashDir: t.TempDir()},
		providers: []*providerEntry{
			{Provider: &readOnlyFake{}, Root: t.TempDir()},
		},
	}
	planned, err := a.PlanCleanup([]CleanCategory{CategoryAbandoned}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(planned) != 0 {
		t.Errorf("planned = %d, want 0 for a read-only provider", len(planned))
	}
}

// readOnlyFake satisfies contracts.Provider but deliberately
// does not implement contracts.Cleaner. We use it in the
// test above to exercise the "skip when no Cleaner" branch.
type readOnlyFake struct{}

func (readOnlyFake) Name() string { return "read-only" }
func (readOnlyFake) Detect(fs.FS) (contracts.StorageVersion, error) {
	return contracts.StorageVersion{}, nil
}
func (readOnlyFake) ListProjects(fs.FS) ([]contracts.Project, error) { return nil, nil }
func (readOnlyFake) ListSessions(fs.FS, contracts.ProjectID) ([]contracts.SessionSummary, error) {
	return nil, nil
}
func (readOnlyFake) ReadSession(fs.FS, contracts.SessionID) (contracts.Conversation, error) {
	return contracts.Conversation{}, nil
}

// TestPlanCleanupStale_picksOnlyOldSessions is the happy
// path for by-age cleanup. Two sessions: one with
// LastActive 90 days ago, one with LastActive yesterday.
// With --older-than=30d, only the old one should produce a
// plan. Pinning the threshold and the timestamps directly
// keeps the math obvious.
func TestPlanCleanupStale_picksOnlyOldSessions(t *testing.T) {
	now := time.Now()
	old := now.Add(-90 * 24 * time.Hour)
	recent := now.Add(-1 * 24 * time.Hour)
	fake := &cleanerFake{
		name:     "claude",
		projects: []contracts.Project{{ID: "p1"}},
		sessions: map[contracts.ProjectID][]contracts.SessionSummary{
			"p1": {
				{ID: "old", Project: "p1", LastActive: old},
				{ID: "recent", Project: "p1", LastActive: recent},
			},
		},
		plans: map[contracts.SessionID]contracts.DeletePlan{
			"old":    {SessionID: "old", Items: []contracts.DeleteItem{{Path: "old.jsonl"}}},
			"recent": {SessionID: "recent", Items: []contracts.DeleteItem{{Path: "recent.jsonl"}}},
		},
	}
	a, _ := newCleanTestApp(t, fake)

	planned, err := a.PlanCleanupStale(30*24*time.Hour, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(planned) != 1 {
		t.Fatalf("planned = %d, want 1 (only the old session is stale)", len(planned))
	}
	if planned[0].Plan.SessionID != "old" {
		t.Errorf("plan session id = %q, want old", planned[0].Plan.SessionID)
	}
}

// TestPlanCleanupStale_skipsZeroTimestamps confirms the
// safety rule. A session whose LastActive is the zero value
// (the adapter could not extract an end time, common on
// hand-edited fixtures and very old sessions) must NOT be
// flagged as stale. Treating "unknown" as "infinitely old"
// would surprise the user the first time it deleted real
// data.
func TestPlanCleanupStale_skipsZeroTimestamps(t *testing.T) {
	fake := &cleanerFake{
		name:     "claude",
		projects: []contracts.Project{{ID: "p1"}},
		sessions: map[contracts.ProjectID][]contracts.SessionSummary{
			"p1": {{ID: "no-ts", Project: "p1"}},
		},
		plans: map[contracts.SessionID]contracts.DeletePlan{
			"no-ts": {SessionID: "no-ts", Items: []contracts.DeleteItem{{Path: "no-ts.jsonl"}}},
		},
	}
	a, _ := newCleanTestApp(t, fake)

	planned, err := a.PlanCleanupStale(30*24*time.Hour, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(planned) != 0 {
		t.Errorf("planned = %d, want 0 (zero timestamps must not count as stale)", len(planned))
	}
}

// TestPlanCleanupStale_rejectsTooSmallThreshold pins the
// minimum-age guard. A 0-duration or sub-day threshold
// would mark every session as stale, which is too
// destructive to expose through a numeric off-by-one.
func TestPlanCleanupStale_rejectsTooSmallThreshold(t *testing.T) {
	a, _ := newCleanTestApp(t, &cleanerFake{name: "claude"})
	if _, err := a.PlanCleanupStale(0, ""); err == nil {
		t.Error("expected an error for an --older-than of 0")
	}
	if _, err := a.PlanCleanupStale(time.Hour, ""); err == nil {
		t.Error("expected an error for a sub-day --older-than value")
	}
}

// TestPlanCleanupStale_filtersByProvider mirrors the
// provider-name behaviour of PlanCleanup. Two cleaner-fakes,
// both with stale sessions, but only the named one should
// contribute plans.
func TestPlanCleanupStale_filtersByProvider(t *testing.T) {
	old := time.Now().Add(-90 * 24 * time.Hour)
	claudeFake := &cleanerFake{
		name:     "claude",
		projects: []contracts.Project{{ID: "p1"}},
		sessions: map[contracts.ProjectID][]contracts.SessionSummary{
			"p1": {{ID: "claude-old", Project: "p1", LastActive: old}},
		},
		plans: map[contracts.SessionID]contracts.DeletePlan{
			"claude-old": {SessionID: "claude-old", Items: []contracts.DeleteItem{{Path: "x"}}},
		},
	}
	copilotFake := &cleanerFake{
		name:     "copilot",
		projects: []contracts.Project{{ID: "p2"}},
		sessions: map[contracts.ProjectID][]contracts.SessionSummary{
			"p2": {{ID: "copilot-old", Project: "p2", LastActive: old}},
		},
		plans: map[contracts.SessionID]contracts.DeletePlan{
			"copilot-old": {SessionID: "copilot-old", Items: []contracts.DeleteItem{{Path: "y"}}},
		},
	}
	a := &App{
		settings:  config.Defaults(),
		locations: paths.Locations{TrashDir: t.TempDir()},
		providers: []*providerEntry{
			{Provider: claudeFake, Root: t.TempDir()},
			{Provider: copilotFake, Root: t.TempDir()},
		},
	}

	planned, err := a.PlanCleanupStale(30*24*time.Hour, "copilot")
	if err != nil {
		t.Fatal(err)
	}
	if len(planned) != 1 || planned[0].Plan.SessionID != "copilot-old" {
		t.Errorf("planned = %+v, want one entry for copilot-old", planned)
	}
}

// TestPlanCleanupStale_skipsNonCleanerProvider confirms
// the read-only provider behaviour. A provider that does
// not implement contracts.Cleaner is silently skipped, the
// same way PlanCleanup handles it.
func TestPlanCleanupStale_skipsNonCleanerProvider(t *testing.T) {
	a := &App{
		settings:  config.Defaults(),
		locations: paths.Locations{TrashDir: t.TempDir()},
		providers: []*providerEntry{
			{Provider: &readOnlyFake{}, Root: t.TempDir()},
		},
	}
	planned, err := a.PlanCleanupStale(30*24*time.Hour, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(planned) != 0 {
		t.Errorf("planned = %d, want 0 for a read-only provider", len(planned))
	}
}
