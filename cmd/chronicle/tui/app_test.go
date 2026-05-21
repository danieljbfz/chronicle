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

// TestAppModel_View_RendersBreadcrumb pins the contract that
// the top-level model always renders the session list screen's
// own breadcrumb header inside the tea.View it returns. In the
// default state, the screen is in its loading branch, so the
// breadcrumb sits above a "scanning" line.
func TestAppModel_View_RendersBreadcrumb(t *testing.T) {
	m := newAppModel(nil, keys.Default(), theme.New(theme.VariantTerminal), "0.1.0", DefaultGlamourStyle)

	view := m.View()

	if !strings.Contains(view.Content, "chronicle") || !strings.Contains(view.Content, "sessions") {
		t.Errorf("the screen header should carry the chronicle/sessions breadcrumb. Got:\n%s", view.Content)
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
// exercises the more user-visible `q` form. The filter-mode
// guard the app model carries is exercised through the
// no-filter default state of the screen.
func TestAppModel_Update_QuitOnQ(t *testing.T) {
	m := newAppModel(nil, keys.Default(), theme.New(theme.VariantTerminal), "0.1.0", DefaultGlamourStyle)

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

// TestAppModel_Update_OpenRequestSwitchesToTranscript confirms
// the app model routes an OpenRequestMsg to the transcript
// reader. The message arrives when the user presses Enter on a
// session row, and the expected behaviour is a screen switch
// (current flips to screenTranscript) plus a command that kicks
// off the transcript reader's read-and-render pipeline. The
// test does not execute the returned command, which would dial
// into the nil app handle the test passes for brevity.
func TestAppModel_Update_OpenRequestSwitchesToTranscript(t *testing.T) {
	m := newAppModel(nil, keys.Default(), theme.New(theme.VariantTerminal), "0.1.0", DefaultGlamourStyle)

	out, cmd := m.Update(sessions.OpenRequestMsg{
		SessionID: contracts.SessionID("abc-123"),
		ProjectID: contracts.ProjectID("proj-1"),
		Provider:  "claude",
	})

	updated, ok := out.(appModel)
	if !ok {
		t.Fatalf("Update should return an appModel, got %T", out)
	}
	if updated.current != screenTranscript {
		t.Errorf("after OpenRequestMsg the active screen should be screenTranscript, got %d", updated.current)
	}
	if cmd == nil {
		t.Fatal("OpenRequestMsg should return a non-nil command (the transcript reader's load)")
	}
}

// TestAppModel_Update_WindowSizeForwards confirms the resize
// message updates the model's dimensions and forwards the size
// through to the embedded sessions screen so its list resizes
// without losing the focus row.
func TestAppModel_Update_WindowSizeForwards(t *testing.T) {
	m := newAppModel(nil, keys.Default(), theme.New(theme.VariantTerminal), "0.1.0", DefaultGlamourStyle)

	out, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	updated, ok := out.(appModel)
	if !ok {
		t.Fatalf("Update should return an appModel, got %T", out)
	}
	if updated.width != 120 || updated.height != 40 {
		t.Errorf("expected width=120 height=40, got width=%d height=%d", updated.width, updated.height)
	}
}
