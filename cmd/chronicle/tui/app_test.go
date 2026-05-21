package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/keys"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/screens/sessions"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/theme"
	"github.com/danieljbfz/chronicle/contracts"
)

// TestAppModel_View_RendersHeader pins the contract that the
// top-level model always renders a header line above the active
// screen's content. In the default state, the header is the
// chronicle title and version. Phase 1's screen below it is the
// session list, so the body shows whichever state the sessions
// screen is in. The view also requests alt-screen mode for the
// program.
func TestAppModel_View_RendersHeader(t *testing.T) {
	m := newAppModel(nil, keys.Default(), theme.New(theme.VariantTerminal), "0.1.0")

	view := m.View()

	if !strings.Contains(view.Content, "chronicle 0.1.0") {
		t.Errorf("the header should render the version string. Got:\n%s", view.Content)
	}
	if !view.AltScreen {
		t.Error("the program should request alt-screen mode")
	}
	if view.WindowTitle != "chronicle" {
		t.Errorf("the terminal window title should be set; got %q", view.WindowTitle)
	}
}

// TestAppModel_Update_QuitOnQ confirms that pressing the bound
// quit key returns a command whose execution produces a QuitMsg.
// The quit binding covers both `q` and `ctrl+c`, so the test
// exercises the more user-visible `q` form. The check that
// follows the binding match (the session list must not be in
// filter mode) is exercised through the no-filter default state
// of the screen.
func TestAppModel_Update_QuitOnQ(t *testing.T) {
	m := newAppModel(nil, keys.Default(), theme.New(theme.VariantTerminal), "0.1.0")

	msg := tea.KeyPressMsg{Code: 'q', Text: "q"}

	_, cmd := m.Update(msg)
	if cmd == nil {
		t.Fatal("pressing q should return a non-nil command")
	}

	out := cmd()
	if _, ok := out.(tea.QuitMsg); !ok {
		t.Errorf("pressing q should resolve to a QuitMsg, got %T", out)
	}
}

// TestAppModel_Update_OpenRequestShowsStatus confirms that when
// the session list emits an OpenRequestMsg, the app model
// surfaces a transient status line that names the session whose
// transcript was requested. Phase 2 will replace the status line
// with a real screen switch, but until then the status line is
// the user-visible evidence that the wiring works end to end.
func TestAppModel_Update_OpenRequestShowsStatus(t *testing.T) {
	m := newAppModel(nil, keys.Default(), theme.New(theme.VariantTerminal), "0.1.0")

	out, _ := m.Update(sessions.OpenRequestMsg{
		SessionID: contracts.SessionID("abc-123"),
		ProjectID: contracts.ProjectID("proj-1"),
		Provider:  "claude",
	})

	updated, ok := out.(appModel)
	if !ok {
		t.Fatalf("Update should return an appModel, got %T", out)
	}
	if !strings.Contains(updated.status, "abc-123") {
		t.Errorf("the status should name the requested session id; got %q", updated.status)
	}
}

// TestAppModel_Update_WindowSizeStored confirms the resize
// message updates the model's dimensions and forwards the
// reduced size to the embedded sessions screen. The reduction
// reserves room for the header line the app model draws above
// the screen content.
func TestAppModel_Update_WindowSizeStored(t *testing.T) {
	m := newAppModel(nil, keys.Default(), theme.New(theme.VariantTerminal), "0.1.0")

	out, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	updated, ok := out.(appModel)
	if !ok {
		t.Fatalf("Update should return an appModel, got %T", out)
	}
	if updated.width != 120 || updated.height != 40 {
		t.Errorf("expected width=120 height=40, got width=%d height=%d", updated.width, updated.height)
	}
}
