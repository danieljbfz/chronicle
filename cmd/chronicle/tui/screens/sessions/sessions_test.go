package sessions

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/keys"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/theme"
	"github.com/danieljbfz/chronicle/composition"
	"github.com/danieljbfz/chronicle/contracts"
)

// fakeLister is the test double for composition.App's
// ListSessionsAll method. Tests construct it with the listings
// or the error they want the screen to receive, and pass it to
// New as the Lister argument.
type fakeLister struct {
	listings []composition.SessionListing
	err      error
}

func (f fakeLister) ListSessionsAll(string) ([]composition.SessionListing, error) {
	return f.listings, f.err
}

// newTestModel constructs a Model wired to a Lister with the
// given listings and no error. Tests that need the error path
// build their own fakeLister.
func newTestModel(listings []composition.SessionListing) Model {
	return New(
		fakeLister{listings: listings},
		keys.Default(),
		theme.New(theme.VariantTerminal),
	)
}

func TestNew_StartsInLoadingState(t *testing.T) {
	m := newTestModel(nil)
	if m.status != statusLoading {
		t.Errorf("a fresh Model should be loading, got status %d", m.status)
	}
	view := m.View()
	if !strings.Contains(view, "Scanning") {
		t.Errorf("loading view should announce the scan; got %q", view)
	}
}

func TestInit_ReturnsLoadCommand(t *testing.T) {
	listings := []composition.SessionListing{sample("a", "Project A", time.Now())}
	m := newTestModel(listings)

	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init should return a non-nil command")
	}
	msg := cmd()
	loaded, ok := msg.(loadedMsg)
	if !ok {
		t.Fatalf("the load command should resolve to a loadedMsg, got %T", msg)
	}
	if len(loaded.listings) != 1 {
		t.Errorf("expected one listing through the load command, got %d", len(loaded.listings))
	}
}

func TestUpdate_LoadedMsg_PopulatesAndSorts(t *testing.T) {
	now := time.Now()
	older := now.Add(-2 * time.Hour)
	oldest := now.Add(-24 * time.Hour)

	// The listings arrive in the wrong order on purpose. The
	// screen is responsible for sorting them most-recent-first
	// before handing them to the embedded list.
	listings := []composition.SessionListing{
		sample("oldest", "Project C", oldest),
		sample("now", "Project A", now),
		sample("older", "Project B", older),
	}

	m, _ := newTestModel(nil).Update(loadedMsg{listings: listings})

	if m.status != statusReady {
		t.Errorf("after a loadedMsg the status should be ready, got %d", m.status)
	}
	items := m.list.Items()
	if len(items) != 3 {
		t.Fatalf("expected three items in the list, got %d", len(items))
	}
	first, ok := items[0].(sessionItem)
	if !ok {
		t.Fatalf("the list items should be sessionItem values, got %T", items[0])
	}
	if first.listing.Summary.ID != contracts.SessionID("now") {
		t.Errorf("the most recently active session should sort first; got %q", first.listing.Summary.ID)
	}
}

func TestUpdate_ErrMsg_ShowsError(t *testing.T) {
	m, _ := newTestModel(nil).Update(errMsg{err: errors.New("disk gone away")})

	if m.status != statusError {
		t.Errorf("after an errMsg the status should be error, got %d", m.status)
	}
	view := m.View()
	if !strings.Contains(view, "disk gone away") {
		t.Errorf("the error view should quote the underlying error; got %q", view)
	}
	if !strings.Contains(view, "chronicle doctor") {
		t.Errorf("the error view should point users at the doctor command; got %q", view)
	}
}

func TestView_EmptyState_NamesNextStep(t *testing.T) {
	m, _ := newTestModel(nil).Update(loadedMsg{listings: nil})

	view := m.View()
	if !strings.Contains(view, "No sessions") {
		t.Errorf("the empty view should announce zero sessions; got %q", view)
	}
	if !strings.Contains(view, "chronicle doctor") {
		t.Errorf("the empty view should suggest chronicle doctor; got %q", view)
	}
}

func TestUpdate_EnterEmitsOpenRequest(t *testing.T) {
	listings := []composition.SessionListing{sample("session-1", "proj-1", time.Now())}

	m, _ := newTestModel(nil).Update(loadedMsg{listings: listings})

	// The list initialises with width/height zero. Bubbles list
	// still resolves SelectedItem off the first row when the
	// width is zero, but the size message keeps the test honest
	// about the dimensions the live screen would receive.
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	if cmd == nil {
		t.Fatal("Enter on a populated session row should return a non-nil command")
	}
	msg := cmd()
	req, ok := msg.(OpenRequestMsg)
	if !ok {
		t.Fatalf("Enter should resolve to an OpenRequestMsg, got %T", msg)
	}
	if req.SessionID != contracts.SessionID("session-1") {
		t.Errorf("the OpenRequestMsg should name the selected session; got %q", req.SessionID)
	}
}

func TestSessionItem_FilterValueIncludesEveryField(t *testing.T) {
	item := sessionItem{listing: sample("id-1", "Project Alpha", time.Now())}
	item.listing.Summary.Title = "Refactor the user table"
	item.listing.Provider = "claude"

	got := item.FilterValue()
	for _, want := range []string{"Refactor the user table", "Project Alpha", "claude"} {
		if !strings.Contains(got, want) {
			t.Errorf("FilterValue should include %q; got %q", want, got)
		}
	}
}

// sample is the small factory the tests use to build a
// SessionListing with the fields the screen actually reads.
// Fields the screen never touches are left at zero.
func sample(id, project string, lastActive time.Time) composition.SessionListing {
	return composition.SessionListing{
		Provider: "claude",
		Summary: contracts.SessionSummary{
			ID:         contracts.SessionID(id),
			Project:    contracts.ProjectID(project),
			Title:      "Session " + id,
			LastActive: lastActive,
			StartedAt:  lastActive.Add(-time.Hour),
		},
	}
}
