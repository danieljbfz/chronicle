package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/screens/sessions"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/screens/stats"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/theme"
)

// Screen is one top-level section of the TUI — the session list,
// the stats view, and the doctor, trash, and memory views that
// land in later phases. The app model holds the built screens in
// a section-keyed registry and routes the active one's Init,
// Update, and View, so adding a section is a registry entry
// rather than another branch in a type switch.
//
// Update returns a Screen rather than a concrete model so the app
// can store the result back into the registry without knowing
// which section it is. The concrete screen models live in their
// own packages and return their own types, so each one is wrapped
// in a small adapter in this file. The adapter is the seam that
// lets a value-type model in another package satisfy this
// interface without an import cycle — the interface lives here in
// the tui package, and the screen packages never import it.
type Screen interface {
	Init() tea.Cmd
	Update(tea.Msg) (Screen, tea.Cmd)
	View() string
}

// section identifies a top-level screen and fixes its position in
// the tab strip. The iota order is the left-to-right order the
// tabs render in and the order Tab and Shift-Tab cycle through.
type section int

const (
	sectionSessions section = iota
	sectionStats
)

// sectionMeta carries the two presentation facts the tab strip
// needs for one section: the number key that jumps to it and the
// label it shows. The number is a string rather than an int
// because it is compared against the keypress text and rendered
// into the strip, never used for arithmetic.
type sectionMeta struct {
	key   string
	label string
}

// appChromeHeight is the number of terminal rows the app draws
// above every top-level screen: the tab strip line and the
// divider beneath it. The app subtracts this from the height it
// forwards to each screen so a screen sizes its own content to
// the rows it actually owns.
const appChromeHeight = 2

// sessionsScreen adapts the session-list model to the Screen
// interface. The adapter holds the model by value and threads the
// updated model back through itself, so the app can treat it like
// any other section. IsFiltering is exposed beyond the interface
// because the app suppresses global keys (quit, section switches)
// while the user is typing into the list's filter, and that is a
// fact only the session list knows.
type sessionsScreen struct {
	model sessions.Model
}

func (s sessionsScreen) Init() tea.Cmd { return s.model.Init() }

func (s sessionsScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	updated, cmd := s.model.Update(msg)
	s.model = updated
	return s, cmd
}

func (s sessionsScreen) View() string { return s.model.View() }

func (s sessionsScreen) IsFiltering() bool { return s.model.IsFiltering() }

// statsScreen adapts the stats model to the Screen interface. It
// is the same thin wrapper as sessionsScreen without the
// filter-mode method, because the stats view has no text input to
// capture keys.
type statsScreen struct {
	model stats.Model
}

func (s statsScreen) Init() tea.Cmd { return s.model.Init() }

func (s statsScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	updated, cmd := s.model.Update(msg)
	s.model = updated
	return s, cmd
}

func (s statsScreen) View() string { return s.model.View() }

// renderChrome paints the two-row top chrome the app draws above
// every top-level screen: the tab strip and a full-width divider.
// The result is always exactly appChromeHeight lines so the
// height the app forwards to the active screen stays accurate.
func renderChrome(width int, order []section, meta map[section]sectionMeta, active section, t theme.Theme) string {
	w := width
	if w < minChromeWidth {
		w = minChromeWidth
	}
	strip := renderTabStrip(w, order, meta, active, t)
	divider := t.Muted.Render(strings.Repeat("─", w))
	return strip + "\n" + divider
}

// minChromeWidth is the floor the chrome renders at. Below it the
// terminal is too narrow for chronicle to lay anything out
// sensibly, so the chrome stops shrinking and the terminal clips
// the overflow rather than the renderer producing a broken line.
const minChromeWidth = 20

// brand is the product name the tab strip leads with, so the user
// always sees what program they are in regardless of which
// section is active.
const brand = "chronicle"

// renderTabStrip renders the one-line section navigation. It has
// two tiers so it stays on a single line at any width. The full
// tier shows the brand and every section as "<key> <label>", with
// the active section painted in the accent and the inactive ones
// muted. When the full tier would overflow the width, the compact
// tier shows the brand, the active section's label, and the set
// of section numbers with the active one accented, which still
// tells the user where they are and how many sections exist.
func renderTabStrip(width int, order []section, meta map[section]sectionMeta, active section, t theme.Theme) string {
	full := renderTabStripFull(order, meta, active, t)
	if lipgloss.Width(full) <= width {
		return full
	}
	return renderTabStripCompact(order, meta, active, t)
}

func renderTabStripFull(order []section, meta map[section]sectionMeta, active section, t theme.Theme) string {
	tabs := make([]string, 0, len(order))
	for _, sec := range order {
		m := meta[sec]
		key := t.Muted.Render(m.key + " ")
		if sec == active {
			tabs = append(tabs, key+t.Accent.Render(m.label))
			continue
		}
		tabs = append(tabs, key+t.Subtitle.Render(m.label))
	}
	separator := t.Muted.Render("  ·  ")
	return t.Title.Render(brand) + "   " + strings.Join(tabs, separator)
}

func renderTabStripCompact(order []section, meta map[section]sectionMeta, active section, t theme.Theme) string {
	markers := make([]string, 0, len(order))
	activeLabel := ""
	for _, sec := range order {
		m := meta[sec]
		if sec == active {
			markers = append(markers, t.Accent.Render(m.key))
			activeLabel = m.label
			continue
		}
		markers = append(markers, t.Muted.Render(m.key))
	}
	return t.Title.Render(brand) + "  " +
		t.Accent.Render(activeLabel) + "  " +
		t.Muted.Render(strings.Join(markers, "·"))
}
