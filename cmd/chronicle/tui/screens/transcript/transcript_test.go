package transcript

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/keys"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/theme"
	"github.com/danieljbfz/chronicle/contracts"
)

// fakeReader is the test double for composition.App's
// ReadSession method. Tests construct it with the Conversation
// or the error the model should receive, and pass it to New as
// the Reader argument.
type fakeReader struct {
	conv contracts.Conversation
	err  error
}

func (f fakeReader) ReadSession(_ contracts.SessionID) (contracts.Conversation, error) {
	return f.conv, f.err
}

// newTestModel constructs a Model wired to a Reader with the
// given conversation and error. The other inputs are realistic
// defaults — the standard key map, the terminal-palette theme,
// the "dark" glamour stylesheet, and stable identifiers — so
// each test focuses on the behaviour it exercises rather than
// on constructor boilerplate.
func newTestModel(conv contracts.Conversation, readErr error) Model {
	return New(
		fakeReader{conv: conv, err: readErr},
		keys.Default(),
		theme.New(theme.VariantTerminal),
		"dark",
		contracts.SessionID("test-session"),
		contracts.ProjectID("test-project"),
		"claude",
	)
}

// TestNew_StartsInLoadingState pins the contract that a fresh
// Model is in its loading state until the asynchronous fetch
// resolves. The view announces the load so a user who lands on
// the screen mid-fetch knows what is happening.
func TestNew_StartsInLoadingState(t *testing.T) {
	m := newTestModel(contracts.Conversation{}, nil)
	if m.status != statusLoading {
		t.Errorf("a fresh Model should be loading, got status %d", m.status)
	}
	view := m.View()
	if !strings.Contains(view, "Loading transcript") {
		t.Errorf("loading view should announce the load; got %q", view)
	}
}

// TestInit_ReturnsLoadCommand confirms Init kicks off the
// read-and-render pipeline and that the resulting command
// produces a loadedMsg carrying both the parsed conversation
// and a non-empty rendered Markdown string.
func TestInit_ReturnsLoadCommand(t *testing.T) {
	sample := contracts.Conversation{
		SessionID: "test-session",
		Title:     "Refactor the user table",
		Messages: []contracts.Message{
			{Role: contracts.RoleUser, Blocks: []contracts.Block{contracts.TextBlock{Text: "hello"}}},
		},
	}
	m := newTestModel(sample, nil)

	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init should return a non-nil command")
	}
	msg := cmd()
	loaded, ok := msg.(loadedMsg)
	if !ok {
		t.Fatalf("the load command should resolve to a loadedMsg, got %T", msg)
	}
	if loaded.conv.Title != sample.Title {
		t.Errorf("loadedMsg should carry the loaded conversation; got Title=%q", loaded.conv.Title)
	}
	if loaded.rendered == "" {
		t.Error("loadedMsg should carry the rendered Markdown")
	}
}

// TestUpdate_LoadedMsg_PopulatesViewport confirms the loaded
// branch flips the model to ready and writes the rendered
// Markdown into the embedded viewport, so the user sees the
// transcript on the next frame. The WindowSizeMsg the test
// sends first mirrors the live runtime sequence — Bubble Tea
// hands every screen its dimensions before the screen becomes
// visible, and the viewport returns an empty string until it
// has a non-zero size.
func TestUpdate_LoadedMsg_PopulatesViewport(t *testing.T) {
	conv := contracts.Conversation{Title: "T"}
	m, _ := newTestModel(conv, nil).Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = m.Update(loadedMsg{conv: conv, rendered: "rendered markdown body"})

	if m.status != statusReady {
		t.Errorf("status should be ready after loadedMsg, got %d", m.status)
	}
	if !strings.Contains(m.viewport.View(), "rendered markdown body") {
		t.Errorf("viewport should contain the rendered Markdown; got %q", m.viewport.View())
	}
}

// TestUpdate_ErrMsg_QuotesError confirms the failure branch
// flips the model to error and shows a full-sentence message
// that names the underlying problem and the next action the
// user can take.
func TestUpdate_ErrMsg_QuotesError(t *testing.T) {
	m, _ := newTestModel(contracts.Conversation{}, nil).Update(errMsg{err: errors.New("disk gone away")})

	if m.status != statusError {
		t.Errorf("status should be error after errMsg, got %d", m.status)
	}
	view := m.View()
	if !strings.Contains(view, "disk gone away") {
		t.Errorf("error view should quote the underlying error; got %q", view)
	}
	if !strings.Contains(view, "Esc") {
		t.Errorf("error view should point the user at Esc; got %q", view)
	}
}

// TestUpdate_EscEmitsBackMsg confirms the back binding produces
// the message the app model consumes to return the user to the
// session list. The screen does not navigate itself — it emits
// an intent and lets the top-level model route.
func TestUpdate_EscEmitsBackMsg(t *testing.T) {
	m, _ := newTestModel(contracts.Conversation{}, nil).Update(loadedMsg{rendered: "x"})

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc, Text: "esc"})
	if cmd == nil {
		t.Fatal("Esc should return a non-nil command")
	}
	msg := cmd()
	if _, ok := msg.(BackMsg); !ok {
		t.Fatalf("Esc should resolve to a BackMsg, got %T", msg)
	}
}

// TestUpdate_WindowSizeRerendersAtNewWidth confirms the resize
// path re-runs the Markdown render at the new wrap width. The
// test loads a long assistant reply, then sends two resize
// messages at very different widths. The viewport content
// after each resize must differ — glamour wraps to the width
// it is given, so the rendered output reflects the new width
// directly. If it did not, a terminal resize would leave the
// transcript wrapped to the previous width and parts of the
// text would either truncate or run off the edge.
func TestUpdate_WindowSizeRerendersAtNewWidth(t *testing.T) {
	body := strings.Repeat("the quick brown fox jumps over the lazy dog ", 8)
	conv := contracts.Conversation{
		Messages: []contracts.Message{
			{Role: contracts.RoleAssistant, Blocks: []contracts.Block{contracts.TextBlock{Text: body}}},
		},
	}
	m, _ := newTestModel(conv, nil).Update(loadedMsg{conv: conv, rendered: "initial"})

	m, _ = m.Update(tea.WindowSizeMsg{Width: 40, Height: 24})
	narrow := m.viewport.View()

	m, _ = m.Update(tea.WindowSizeMsg{Width: 200, Height: 24})
	wide := m.viewport.View()

	if narrow == wide {
		t.Error("WindowSizeMsg should re-render the Markdown at the new wrap width")
	}
}
