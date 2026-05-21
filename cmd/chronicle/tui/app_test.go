package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/keys"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/theme"
)

// TestAppModel_View_RendersWelcome pins the contract that the
// welcome screen displays the chronicle version, the placeholder
// body that explains what the TUI will eventually do, and the
// help bar. It also confirms the screen requests alt-screen mode,
// because anything less than full-window would feel cramped for a
// browser-shaped tool.
func TestAppModel_View_RendersWelcome(t *testing.T) {
	m := newAppModel(nil, keys.Default(), theme.New(theme.VariantTerminal), "0.1.0")

	view := m.View()

	if !strings.Contains(view.Content, "chronicle 0.1.0") {
		t.Errorf("the welcome screen should render the version string. Got:\n%s", view.Content)
	}
	if !strings.Contains(view.Content, "under construction") {
		t.Errorf("the welcome screen should explain that the TUI is in progress. Got:\n%s", view.Content)
	}
	if !strings.Contains(view.Content, "chronicle --help") {
		t.Errorf("the welcome screen should point users at the working CLI. Got:\n%s", view.Content)
	}
	if !strings.Contains(view.Content, "quit") {
		t.Errorf("the help bar should advertise the quit binding. Got:\n%s", view.Content)
	}
	if !view.AltScreen {
		t.Error("the welcome screen should request alt-screen mode")
	}
}

// TestAppModel_Update_QuitOnQ confirms that pressing the bound
// quit key returns a command whose execution produces a QuitMsg.
// The quit binding covers both `q` and `ctrl+c`, so the test
// exercises the more user-visible `q` form. Phase 1's screens
// will rely on the same binding being honoured globally.
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

// TestAppModel_Update_WindowSizeStored confirms the resize
// message updates the model's dimensions. Later screens rely on
// the dimensions being available before their first render.
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
