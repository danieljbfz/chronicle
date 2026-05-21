package ui

import (
	"errors"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"

	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/keys"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/theme"
)

// testFrame returns a Frame configured for the terminal
// theme. Every Frame test calls this so the assertions read
// against one stable starting point.
func testFrame() Frame {
	return NewFrame(theme.New(theme.VariantTerminal), keys.Default())
}

// TestFrame_Render_FitsTheBudget pins the load-bearing
// property the frame exists to guarantee: the rendered
// output is exactly height rows tall and no row exceeds the
// width. A previous status-row design had a height-accounting
// bug that pushed the footer one row off the bottom of the
// screen; this test would have caught it before the user
// did.
func TestFrame_Render_FitsTheBudget(t *testing.T) {
	f := testFrame()
	for _, dims := range []struct{ w, h int }{
		{80, 24},
		{120, 32},
		{40, 12},
		{200, 50},
	} {
		got := f.Render(dims.w, dims.h, nil, Ready("body content"))
		gotHeight := lipgloss.Height(got)
		gotWidth := maxLineWidth(got)
		if gotHeight != dims.h {
			t.Errorf("Render(%d, %d) height = %d, want %d", dims.w, dims.h, gotHeight, dims.h)
		}
		if gotWidth > dims.w {
			t.Errorf("Render(%d, %d) widest row = %d, want <= %d", dims.w, dims.h, gotWidth, dims.w)
		}
	}
}

// TestFrame_Footer_AlwaysHasHelpAndQuit pins the universal
// anchors the footer carries. Every screen renders through
// the frame, and the user needs to know how to open the
// full help and how to leave the program regardless of
// which screen they are on. A footer that hides those keys
// behind truncation is a discoverability bug.
func TestFrame_Footer_AlwaysHasHelpAndQuit(t *testing.T) {
	f := testFrame()
	got := f.Render(120, 24, nil, Ready(""))
	for _, want := range []string{"?", "help", "q", "quit"} {
		if !strings.Contains(got, want) {
			t.Errorf("footer should contain %q regardless of screen bindings; got:\n%s", want, got)
		}
	}
}

// TestFrame_Footer_KeepsScreenBindings confirms the screen's
// curated bindings reach the footer alongside the global
// anchors. The order is screen first, anchors last, so the
// most context-relevant keys sit at the leading edge of the
// row where the eye lands first.
func TestFrame_Footer_KeepsScreenBindings(t *testing.T) {
	f := testFrame()
	screenBindings := []key.Binding{
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	}
	got := f.Render(120, 24, screenBindings, Ready(""))
	if !strings.Contains(got, "refresh") {
		t.Errorf("footer should include screen-curated bindings; got:\n%s", got)
	}
	// The screen's "refresh" should appear before the
	// global "help" anchor on the row, because the screen
	// bindings render first.
	if strings.Index(got, "refresh") > strings.Index(got, "help") {
		t.Error("screen bindings should appear before the universal anchors")
	}
}

// TestFrame_Loading_CentersTheSpinner confirms the loading
// state sits in the middle of the body region rather than
// pinned to the top with empty space below. The old
// PlaceVertical(Top) layout looked like a broken page; the
// PlaceVertical(Center) layout reads as a deliberate
// loading state.
func TestFrame_Loading_CentersTheSpinner(t *testing.T) {
	f := testFrame()
	s := NewSpinner(theme.New(theme.VariantTerminal), "Loading…")
	got := f.Render(80, 20, nil, Loading(s))

	loadingRow := -1
	for i, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "Loading…") {
			loadingRow = i
			break
		}
	}
	if loadingRow < 2 {
		t.Errorf("loading message should sit in the middle of the body, not at the top; row %d of:\n%s", loadingRow, got)
	}
}

// TestFrame_Error_QuotesTheError pins the error state's
// shape: the body shows the underlying error verbatim and
// the detail prose the screen passed in.
func TestFrame_Error_QuotesTheError(t *testing.T) {
	f := testFrame()
	got := f.Render(80, 20, nil, Error(errors.New("disk gone away"), "Run `chronicle doctor`."))
	if !strings.Contains(got, "disk gone away") {
		t.Errorf("error body should quote the underlying error; got:\n%s", got)
	}
	if !strings.Contains(got, "chronicle doctor") {
		t.Errorf("error body should include the next-step detail; got:\n%s", got)
	}
}

// TestFrame_DividerSeparatesBodyFromFooter confirms the
// muted divider row sits between the body region and the
// help footer. The divider is the visual boundary that
// keeps the eye from reading the help bindings as part of
// the content above them.
func TestFrame_DividerSeparatesBodyFromFooter(t *testing.T) {
	f := testFrame()
	got := f.Render(40, 10, nil, Ready("hello"))
	lines := strings.Split(got, "\n")
	if len(lines) < 3 {
		t.Fatalf("rendered output should have at least three rows; got:\n%s", got)
	}
	dividerRow := lines[len(lines)-2]
	if !strings.Contains(dividerRow, "─") {
		t.Errorf("the row above the footer should be a divider; got %q", dividerRow)
	}
}

// maxLineWidth returns the widest visible row in s, measured
// in terminal columns. The helper strips ANSI escape codes
// through lipgloss.Width so styled content does not skew the
// measurement.
func maxLineWidth(s string) int {
	max := 0
	for _, line := range strings.Split(s, "\n") {
		if w := lipgloss.Width(line); w > max {
			max = w
		}
	}
	return max
}
