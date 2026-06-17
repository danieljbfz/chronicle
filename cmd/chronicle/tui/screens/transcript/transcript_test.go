package transcript

import (
	"errors"
	"strings"
	"testing"
	"time"

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
	// Init batches the fetch with the spinner's tick command, so
	// the outer call resolves to a tea.BatchMsg. The fetch is one
	// of the inner commands; iterate the batch and find the
	// loadedMsg the test cares about.
	loaded, ok := findLoadedMsg(cmd)
	if !ok {
		t.Fatal("the Init batch should include a command that resolves to a loadedMsg")
	}
	if loaded.conv.Title != sample.Title {
		t.Errorf("loadedMsg should carry the loaded conversation; got Title=%q", loaded.conv.Title)
	}
	if loaded.rendered == "" {
		t.Error("loadedMsg should carry the rendered Markdown")
	}
}

// findLoadedMsg walks the batch of commands Init returns and runs
// each one until it finds the loadedMsg the test asserts against.
// The spinner's tick command resolves to a different type, so the
// helper skips past it without failing.
func findLoadedMsg(cmd tea.Cmd) (loadedMsg, bool) {
	msg := cmd()
	switch v := msg.(type) {
	case loadedMsg:
		return v, true
	case tea.BatchMsg:
		for _, c := range v {
			if c == nil {
				continue
			}
			if loaded, ok := findLoadedMsg(c); ok {
				return loaded, true
			}
		}
	}
	return loadedMsg{}, false
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

// TestUpdate_TenJKeypressesAdvanceTheViewportByTen pins the
// property the user's "scroll feels queued" report is about.
// Pressing j ten times in a row should advance the viewport's
// YOffset by ten — every keypress is processed in the Update
// it arrives in, with no spinner-tick interleaving or other
// queued work that would delay the visible response.
//
// The test renders enough content to be scrollable, drives
// the model through ten consecutive j keypresses, and reads
// the viewport's YOffset after each one. If any keypress
// fails to advance the offset by exactly one, the test
// surfaces the discrepancy with the keypress index so a
// future regression has a precise repro.
func TestUpdate_TenJKeypressesAdvanceTheViewportByTen(t *testing.T) {
	// Build a long body so the viewport has somewhere to
	// scroll to. Two hundred lines is well past any plausible
	// viewport height the test runs at.
	var body strings.Builder
	for i := 0; i < 200; i++ {
		body.WriteString("line ")
		body.WriteString(strings.Repeat("x", 10))
		body.WriteByte('\n')
	}

	m := newTestModel(contracts.Conversation{}, nil)
	// Size the viewport before loading so the scroll positions
	// match a realistic runtime sequence.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated
	// Hand the rendered body straight into the loaded path so
	// the screen flips to ready without running the real
	// fetch.
	updated, _ = m.Update(loadedMsg{rendered: body.String()})
	m = updated

	startOffset := m.viewport.YOffset()
	for i := 1; i <= 10; i++ {
		updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
		m = updated
		wantOffset := startOffset + i
		if m.viewport.YOffset() != wantOffset {
			t.Errorf("after keypress %d: YOffset = %d, want %d", i, m.viewport.YOffset(), wantOffset)
		}
	}
}

// TestView_StaysWellUnderTheFrameBudget pins a property the
// transcript reader depends on: a View call on a real-world-sized
// transcript completes well under the 60-fps frame budget (about
// 16 milliseconds), so scroll keypresses feel instant rather than
// queued. A render that creeps toward the budget drops frames and
// makes scrolling feel laggy, so the test fails well before that.
func TestView_StaysWellUnderTheFrameBudget(t *testing.T) {
	const bodySize = 1 << 20 // one megabyte, the realistic upper bound
	var body strings.Builder
	body.Grow(bodySize)
	for body.Len() < bodySize {
		body.WriteString("the quick brown fox jumps over the lazy dog with a moderate sprinkling of words\n")
	}
	m := newTestModel(contracts.Conversation{}, nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	updated, _ = updated.Update(loadedMsg{rendered: body.String()})

	// Take the fastest of several renders rather than timing one.
	// A single wall-clock measurement inside a parallel `-race`
	// run is noisy: the scheduler can preempt the one call being
	// timed and inflate it, which is a property of the machine's
	// load at that instant, not of the renderer. The fastest of a
	// handful of renders reflects the render's own cost. View is
	// read-only, so repeating it is safe.
	const renders = 10
	var best time.Duration
	for i := 0; i < renders; i++ {
		start := time.Now()
		_ = updated.View()
		if d := time.Since(start); i == 0 || d < best {
			best = d
		}
	}

	// Eight milliseconds is half the 60-fps frame budget, so the
	// test catches a render drifting toward the laggy regime before
	// it reaches the budget cliff. The current renderer lands
	// comfortably inside it even under the race detector.
	const budget = 8 * time.Millisecond
	if best > budget {
		t.Errorf("fastest View on a 1 MB transcript took %v, want <= %v (60 FPS frame budget is %v)", best, budget, 16*time.Millisecond)
	}
}

// BenchmarkView_RealWorldTranscript measures how long View
// takes for a transcript-sized body. Bubble Tea throttles
// frames to a default 60 FPS, which gives the renderer about
// 16ms per frame; a View that takes meaningfully more than
// that drops frames and feels like queued input to the user.
// The benchmark exists to give a fast empirical answer to
// "is rendering the bottleneck?" the next time the question
// comes up.
func BenchmarkView_RealWorldTranscript(b *testing.B) {
	var body strings.Builder
	for i := 0; i < 1000; i++ {
		body.WriteString("the quick brown fox jumps over the lazy dog\n")
	}
	m := newTestModel(contracts.Conversation{}, nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	updated, _ = updated.Update(loadedMsg{rendered: body.String()})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = updated.View()
	}
}

// BenchmarkView_HugeRenderedBody measures the View cost for a
// 1 MB rendered body, the rough size a long real Claude
// session produces after glamour renders the Markdown. A
// 60 FPS frame budget allows about 16 milliseconds per
// frame; if View on a 1 MB body takes meaningfully more than
// that, the bubble-tea runtime drops frames and the user
// sees scroll input pile up. The benchmark gives the next
// session a precise measurement to act on, not a guess.
func BenchmarkView_HugeRenderedBody(b *testing.B) {
	const bodySize = 1 << 20 // 1 MB
	var body strings.Builder
	body.Grow(bodySize)
	for body.Len() < bodySize {
		body.WriteString("the quick brown fox jumps over the lazy dog with a moderate sprinkling of words\n")
	}
	m := newTestModel(contracts.Conversation{}, nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	updated, _ = updated.Update(loadedMsg{rendered: body.String()})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = updated.View()
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
