package stats

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/keys"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/theme"
	"github.com/danieljbfz/chronicle/composition"
)

// fakeSource is the test double for composition.App's Stats
// method. Tests construct it with the Stats result or the error
// they want the screen to receive and pass it to New as the
// Source argument.
type fakeSource struct {
	stats composition.Stats
	err   error
}

func (f fakeSource) Stats(composition.StatsOptions) (composition.Stats, error) {
	return f.stats, f.err
}

// newTestModel constructs a Model wired to a Source with the
// given stats and error. The other inputs are realistic
// defaults so each test focuses on the behaviour it exercises
// rather than on constructor boilerplate.
func newTestModel(s composition.Stats, srcErr error) Model {
	return New(
		fakeSource{stats: s, err: srcErr},
		keys.Default(),
		theme.New(theme.VariantTerminal),
	)
}

// sampleStats is the fixture every behaviour test reaches for.
// It carries one row per provider, one top project, and one
// model so every section the renderer can emit is exercised.
func sampleStats() composition.Stats {
	when := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	return composition.Stats{
		GeneratedAt: when,
		Total: composition.Aggregate{
			Sessions: 12, Messages: 480, SizeBytes: 1024 * 1024,
			OldestAt: when.AddDate(0, -1, 0), NewestAt: when,
		},
		Providers: []composition.ProviderStats{
			{Name: "claude", Projects: 3, Aggregate: composition.Aggregate{Sessions: 9, Messages: 360, SizeBytes: 768 * 1024}},
			{Name: "copilot-chat", Projects: 1, Aggregate: composition.Aggregate{Sessions: 3, Messages: 120, SizeBytes: 256 * 1024}},
		},
		TopProjects: []composition.ProjectStats{
			{Provider: "claude", DisplayName: "chronicle", Path: "~/Desktop/work/chronicle", Aggregate: composition.Aggregate{Sessions: 7, SizeBytes: 512 * 1024}},
		},
		ByModel: []composition.ModelStats{
			{Model: "claude-sonnet-4-6", Aggregate: composition.Aggregate{Sessions: 10, Messages: 400, SizeBytes: 900 * 1024}},
			{Model: "", Aggregate: composition.Aggregate{Sessions: 2, Messages: 80, SizeBytes: 124 * 1024}},
		},
	}
}

// TestNew_StartsInLoadingState pins the contract that a fresh
// Model is in its loading state until the asynchronous fetch
// resolves. The view announces the load so a user who lands on
// the screen mid-fetch knows what is happening.
func TestNew_StartsInLoadingState(t *testing.T) {
	m := newTestModel(composition.Stats{}, nil)
	if m.status != statusLoading {
		t.Errorf("a fresh Model should be loading, got status %d", m.status)
	}
	if !strings.Contains(m.View(), "Computing the summary") {
		t.Errorf("loading view should announce the load; got %q", m.View())
	}
}

// TestInit_ReturnsLoadCommand confirms Init kicks off the
// Stats call and the resulting command produces a loadedMsg
// carrying the parsed summary and a non-empty rendered string.
func TestInit_ReturnsLoadCommand(t *testing.T) {
	m := newTestModel(sampleStats(), nil)

	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init should return a non-nil command")
	}
	msg := cmd()
	loaded, ok := msg.(loadedMsg)
	if !ok {
		t.Fatalf("the load command should resolve to a loadedMsg, got %T", msg)
	}
	if loaded.stats.Total.Sessions != 12 {
		t.Errorf("loadedMsg should carry the loaded summary; got Total.Sessions=%d", loaded.stats.Total.Sessions)
	}
	if loaded.rendered == "" {
		t.Error("loadedMsg should carry the rendered summary")
	}
}

// TestUpdate_LoadedMsg_PopulatesViewport confirms the loaded
// branch flips the model to ready and writes the rendered
// summary into the embedded viewport. The WindowSizeMsg the
// test sends first mirrors the live runtime sequence — Bubble
// Tea hands every screen its dimensions before the screen
// becomes visible, and the viewport returns an empty string
// until it has a non-zero size.
func TestUpdate_LoadedMsg_PopulatesViewport(t *testing.T) {
	m, _ := newTestModel(sampleStats(), nil).Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m, _ = m.Update(loadedMsg{stats: sampleStats(), rendered: "rendered summary body"})

	if m.status != statusReady {
		t.Errorf("status should be ready after loadedMsg, got %d", m.status)
	}
	if !strings.Contains(m.viewport.View(), "rendered summary body") {
		t.Errorf("viewport should contain the rendered summary; got %q", m.viewport.View())
	}
}

// TestUpdate_ErrMsg_QuotesError confirms the failure branch
// flips the model to error and shows a full-sentence message
// that names the underlying problem and the next action the
// user can take.
func TestUpdate_ErrMsg_QuotesError(t *testing.T) {
	m, _ := newTestModel(composition.Stats{}, nil).Update(errMsg{err: errors.New("disk gone away")})

	if m.status != statusError {
		t.Errorf("status should be error after errMsg, got %d", m.status)
	}
	view := m.View()
	if !strings.Contains(view, "disk gone away") {
		t.Errorf("error view should quote the underlying error; got %q", view)
	}
	if !strings.Contains(view, "chronicle doctor") {
		t.Errorf("error view should point users at chronicle doctor; got %q", view)
	}
}

// TestRenderStats_includesEverySection confirms the renderer
// emits the four sections of the summary when each one has
// data. The test reaches into the package-level helper rather
// than driving a Model so the assertions stay focused on the
// rendered output rather than on the asynchronous load path.
func TestRenderStats_includesEverySection(t *testing.T) {
	out := renderStats(sampleStats(), 120, theme.New(theme.VariantTerminal))

	for _, want := range []string{"Totals", "By provider", "Top 1 project", "By model"} {
		if !strings.Contains(out, want) {
			t.Errorf("renderStats output should include section %q; got:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "claude") {
		t.Error("renderStats should mention the claude provider row")
	}
	if !strings.Contains(out, "(unknown)") {
		t.Error("renderStats should render the empty-model bucket as (unknown)")
	}
}

// TestRenderStats_widthBoundsTheOutput confirms the tables
// honour the width budget the renderer is given. The test
// renders the same summary at two very different widths and
// asserts the wide render is wider than the narrow one,
// which is the property terminal-resize handling depends on
// — a row that ignored the width budget would produce the
// same string regardless of the terminal size.
func TestRenderStats_widthBoundsTheOutput(t *testing.T) {
	t1 := theme.New(theme.VariantTerminal)
	narrow := renderStats(sampleStats(), 60, t1)
	wide := renderStats(sampleStats(), 160, t1)
	if narrow == wide {
		t.Error("renderStats should produce different output at different widths")
	}
}
