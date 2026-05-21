package ui

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"

	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/keys"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/theme"
)

// HelpBar renders the short help line every screen shows at the
// bottom of its frame. It is the one canonical implementation
// each screen composes — drift between screens, like the kind
// that produced one help line for the session list and another
// for the stats screen before this package existed, is no
// longer possible because there is nothing to drift from.
//
// extras is the slice of bindings the screen wants to advertise
// before the global ones. Pass the bindings that are specific
// to the current section (the transcript reader's u/d half-page
// jumps, the stats screen's r refresh) and the renderer
// prepends them to the global set so the most useful keys for
// the active section sit at the front of the line.
//
// width caps the rendered line. The bubbles help component
// truncates trailing entries with an ellipsis when the line
// would overflow, so the bar always fits on a single terminal
// row regardless of how many bindings the screen offers. A
// non-positive width disables the cap and renders every
// binding, which is the right behaviour for tests but not for
// the live runtime.
func HelpBar(t theme.Theme, k keys.KeyMap, extras []key.Binding, width int) string {
	model := help.New()
	model.Styles.ShortKey = t.HelpKey
	model.Styles.ShortDesc = t.HelpDesc
	model.Styles.ShortSeparator = t.Muted
	model.Styles.Ellipsis = t.Muted

	globals := k.ShortHelp()
	bindings := make([]key.Binding, 0, len(extras)+len(globals))
	bindings = append(bindings, extras...)
	bindings = append(bindings, globals...)

	// The bubbles help component's built-in truncation is
	// unreliable in the v2.1.0 release we depend on: at modest
	// widths it produces a rendered line wider than the budget
	// without trimming. The user reported the visible symptom
	// against an earlier hand-rolled help line ("1-5 s" cut off
	// by the terminal edge), and the bubbles version reproduces
	// the same overflow. Until the upstream component handles
	// this reliably, the bar enforces the budget itself by
	// rendering bindings one at a time and stopping with an
	// ellipsis as soon as the next binding would overflow.
	if width <= 0 {
		return model.ShortHelpView(bindings)
	}
	return truncatedHelpView(model, t, bindings, width)
}

// truncatedHelpView walks the bindings, renders each one through
// the bubbles help component (so the styling stays identical to
// the untruncated path), and stops as soon as the next binding
// plus its separator would push the rendered width past the
// budget. The remaining bindings are dropped and an ellipsis
// marks the truncation, the same shape the bubbles component
// produces when its own truncation works.
func truncatedHelpView(model help.Model, t theme.Theme, bindings []key.Binding, width int) string {
	separator := t.Muted.Render(" • ")
	separatorWidth := lipgloss.Width(separator)
	ellipsis := " " + t.Muted.Render("…")
	ellipsisWidth := lipgloss.Width(ellipsis)

	var b strings.Builder
	total := 0
	for i, binding := range bindings {
		if !binding.Enabled() {
			continue
		}
		piece := model.ShortHelpView([]key.Binding{binding})
		pieceWidth := lipgloss.Width(piece)

		// Step 1: account for the separator that sits between
		// this item and the previous one. The first rendered
		// item does not need one.
		addedWidth := pieceWidth
		if total > 0 {
			addedWidth += separatorWidth
		}

		// Step 2: refuse the item when it would not leave room
		// for an ellipsis after it. The cap on the ellipsis row
		// is what guarantees the bar fits the budget even when
		// the final accepted item is short and a long one
		// follows.
		remaining := len(bindings) - i - 1
		needsEllipsis := remaining > 0
		extra := 0
		if needsEllipsis {
			extra = ellipsisWidth
		}
		if total+addedWidth+extra > width {
			if total > 0 {
				b.WriteString(ellipsis)
			}
			return b.String()
		}

		// Step 3: commit the item.
		if total > 0 {
			b.WriteString(separator)
		}
		b.WriteString(piece)
		total += addedWidth
	}
	return b.String()
}
