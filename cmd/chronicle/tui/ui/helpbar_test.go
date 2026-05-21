package ui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"

	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/keys"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/theme"
)

// TestHelpBar_includesExtrasAndGlobals confirms the bar renders
// both the screen's extras and the global key map's short-help
// bindings, with the extras leading. This is the property the
// per-screen call sites rely on so a section's most useful keys
// sit at the front of the line.
func TestHelpBar_includesExtrasAndGlobals(t *testing.T) {
	extras := []key.Binding{
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	}
	got := HelpBar(theme.New(theme.VariantTerminal), keys.Default(), extras, 0)

	if !strings.Contains(got, "r") || !strings.Contains(got, "refresh") {
		t.Errorf("help bar should include the extra binding; got %q", got)
	}
	// The global ShortHelp includes the up binding, so its
	// description should appear in the rendered string too.
	if !strings.Contains(got, "up") {
		t.Errorf("help bar should include the global up binding; got %q", got)
	}
	// The extras come first, so the position of "refresh" should
	// be before the position of "up" in the rendered string.
	if strings.Index(got, "refresh") > strings.Index(got, "up") {
		t.Errorf("extras should render before globals; got %q", got)
	}
}

// TestHelpBar_truncatesAtNarrowWidth confirms the bar honours
// the width budget. At a narrow width the bubbles help component
// drops trailing entries and shows an ellipsis rather than
// overflowing the line. The "1-5 s" cut-off the user reported
// against the old hand-rolled help line is the bug this test
// guards against: with a width cap in place, the renderer
// truncates with an ellipsis rather than producing a string
// wider than the budget.
func TestHelpBar_truncatesAtNarrowWidth(t *testing.T) {
	extras := []key.Binding{
		key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "first")),
		key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "second")),
		key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "third")),
	}
	got := HelpBar(theme.New(theme.VariantTerminal), keys.Default(), extras, 30)

	if !strings.Contains(got, "…") {
		t.Errorf("help bar should truncate with an ellipsis at width=30; got %q", got)
	}
}

// TestHelpBar_neverExceedsBudget pins the load-bearing property
// the truncation exists to enforce: the rendered string's
// visible width never exceeds the width argument. The user's
// original "1-5 s" overflow report is exactly the failure shape
// this test guards against, evaluated across a range of widths
// from the floor of what fits anything (10) to a comfortable
// terminal (120).
func TestHelpBar_neverExceedsBudget(t *testing.T) {
	extras := []key.Binding{
		key.NewBinding(key.WithKeys("u", "d"), key.WithHelp("u/d", "half page")),
		key.NewBinding(key.WithKeys("g", "G"), key.WithHelp("g/G", "top/bottom")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	}
	for _, width := range []int{10, 20, 30, 50, 80, 120} {
		got := HelpBar(theme.New(theme.VariantTerminal), keys.Default(), extras, width)
		if lipgloss.Width(got) > width {
			t.Errorf("HelpBar at width=%d rendered %d visible columns; got %q",
				width, lipgloss.Width(got), got)
		}
	}
}
