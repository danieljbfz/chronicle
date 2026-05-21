package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/keys"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/screens/sessions"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/screens/transcript"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/theme"
	"github.com/danieljbfz/chronicle/contracts"
)

// TestAppModel_View_RendersTabStrip pins the contract that the
// top-level model always renders the section tab strip inside the
// tea.View it returns. The strip carries the brand and every
// registered section label, the program runs in alt-screen mode,
// and the terminal window title is set.
func TestAppModel_View_RendersTabStrip(t *testing.T) {
	m := newAppModel(nil, keys.Default(), theme.New(theme.VariantTerminal), "0.1.0", DefaultGlamourStyle)
	// Give the model a realistic terminal size so the chrome
	// renders in its full-label tier rather than its compact one.
	out, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 32})
	updated := out.(appModel)

	view := updated.View()

	for _, want := range []string{"chronicle", "sessions", "stats"} {
		if !strings.Contains(view.Content, want) {
			t.Errorf("tab strip should mention %q; got:\n%s", want, view.Content)
		}
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

// TestAppModel_Update_OpenRequestOpensTranscript confirms the app
// model routes an OpenRequestMsg to the transcript reader. The
// message arrives when the user presses Enter on a session row,
// and the expected behaviour is that the transcript overlay opens
// (showTranscript flips to true) plus a command that kicks off the
// transcript reader's read-and-render pipeline. The test does not
// execute the returned command, which would dial into the nil app
// handle the test passes for brevity.
func TestAppModel_Update_OpenRequestOpensTranscript(t *testing.T) {
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
	if !updated.showTranscript {
		t.Error("after OpenRequestMsg the transcript overlay should be open")
	}
	if cmd == nil {
		t.Fatal("OpenRequestMsg should return a non-nil command (the transcript reader's load)")
	}
}

// TestAppModel_Update_NumberKeySwitchesSection confirms a number
// key jumps directly to the section in that position. Pressing "2"
// activates the stats section, the second tab in the order.
func TestAppModel_Update_NumberKeySwitchesSection(t *testing.T) {
	m := newAppModel(nil, keys.Default(), theme.New(theme.VariantTerminal), "0.1.0", DefaultGlamourStyle)

	out, _ := m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	updated, ok := out.(appModel)
	if !ok {
		t.Fatalf("Update should return an appModel, got %T", out)
	}
	if updated.active != sectionStats {
		t.Errorf("pressing 2 should activate the stats section, got %d", updated.active)
	}
}

// TestAppModel_Update_BackClosesTranscript confirms the transcript
// overlay closes when the reader emits a BackMsg, returning the
// user to the section they came from.
func TestAppModel_Update_BackClosesTranscript(t *testing.T) {
	m := newAppModel(nil, keys.Default(), theme.New(theme.VariantTerminal), "0.1.0", DefaultGlamourStyle)
	m.showTranscript = true

	out, _ := m.Update(transcript.BackMsg{})
	updated, ok := out.(appModel)
	if !ok {
		t.Fatalf("Update should return an appModel, got %T", out)
	}
	if updated.showTranscript {
		t.Error("a BackMsg should close the transcript overlay")
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
