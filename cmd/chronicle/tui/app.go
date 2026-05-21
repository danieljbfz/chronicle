package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/keys"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/screens/sessions"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/screens/stats"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/screens/transcript"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/theme"
	"github.com/danieljbfz/chronicle/composition"
)

// appModel is the top-level tea.Model. It owns the state every
// screen shares, the section registry of built top-level screens,
// the transcript drill-down overlay, and the identifier of the
// active section. The app routes messages to the active screen,
// draws the tab strip above it, and intercepts a small set of
// cross-screen intents (section switches, OpenRequest, Back, Quit)
// before forwarding the rest.
//
// The transcript reader is not a section. It is a drill-down the
// user reaches by pressing Enter on a session row and leaves by
// pressing Esc, so it lives in its own field and takes over the
// whole window while it is open rather than sitting behind a tab.
type appModel struct {
	app          *composition.App
	keys         keys.KeyMap
	theme        theme.Theme
	version      string
	glamourStyle string

	width  int
	height int

	order   []section
	meta    map[section]sectionMeta
	screens map[section]Screen
	// initialised tracks which sections have already had their
	// Init run. Sections start their loads lazily, the moment
	// the user first activates them, so an expensive screen
	// (the stats summary, which walks every session on every
	// provider) does not block the program's first frame.
	initialised map[section]bool
	active      section

	transcript     transcript.Model
	showTranscript bool
}

func newAppModel(app *composition.App, k keys.KeyMap, t theme.Theme, version, glamourStyle string) appModel {
	order := []section{sectionSessions, sectionStats}
	meta := map[section]sectionMeta{
		sectionSessions: {key: "1", label: "sessions"},
		sectionStats:    {key: "2", label: "stats"},
	}
	screens := map[section]Screen{
		sectionSessions: sessionsScreen{model: sessions.New(app, k, t)},
		sectionStats:    statsScreen{model: stats.New(app, k, t)},
	}
	return appModel{
		app:          app,
		keys:         k,
		theme:        t,
		version:      version,
		glamourStyle: glamourStyle,
		order:        order,
		meta:         meta,
		screens:      screens,
		initialised:  map[section]bool{},
		active:       sectionSessions,
	}
}

// Init starts the active section's load. Other sections are
// initialised lazily, the first time the user activates them.
// Eagerly loading every section at startup turned out to be a
// real performance hazard: the stats screen walks every session
// on every provider to compute its summary, which on a
// realistic install (one or two hundred sessions across two
// providers) is a multi-second cost the user pays before any
// frame renders. Lazy initialisation lets the program show the
// active section's first frame immediately, and lets the load
// for an expensive section happen only when the user actually
// asks to see it.
func (m appModel) Init() tea.Cmd {
	return m.initSection(m.active)
}

// initSection runs the named section's Init exactly once, the
// first time the section becomes active. Subsequent activations
// are no-ops, so a user tabbing back and forth between sections
// does not re-trigger the loads. The initialised map is a
// reference type in Go, so the value receiver here still
// records the activation in the map every caller shares.
func (m appModel) initSection(sec section) tea.Cmd {
	if m.initialised[sec] {
		return nil
	}
	m.initialised[sec] = true
	return m.screens[sec].Init()
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// The top-level screens sit below the app's two-row
		// chrome, so they receive a height reduced by it. The
		// transcript overlay owns the whole window and receives
		// the full size. Forwarding the resize to every screen,
		// not just the active one, keeps a screen the user later
		// switches to correctly sized the moment it appears.
		screenHeight := msg.Height - appChromeHeight
		if screenHeight < 1 {
			screenHeight = 1
		}
		screenMsg := tea.WindowSizeMsg{Width: msg.Width, Height: screenHeight}
		cmds := make([]tea.Cmd, 0, len(m.order)+1)
		for _, sec := range m.order {
			var cmd tea.Cmd
			m.screens[sec], cmd = m.screens[sec].Update(screenMsg)
			cmds = append(cmds, cmd)
		}
		var transcriptCmd tea.Cmd
		m.transcript, transcriptCmd = m.transcript.Update(msg)
		cmds = append(cmds, transcriptCmd)
		return m, tea.Batch(cmds...)

	case tea.KeyPressMsg:
		// Quit is global, guarded against the session list's
		// filter mode so a user typing "quit" into the filter
		// does not exit the program.
		if key.Matches(msg, m.keys.Quit) && !m.isFiltering() {
			return m, tea.Quit
		}
		// Section navigation is a top-level action. It is
		// suppressed while the transcript overlay is open, where
		// Esc returns to the tabbed view first, and while the
		// session filter is capturing input.
		if !m.showTranscript && !m.isFiltering() {
			if next, ok := m.sectionForKeypress(msg); ok {
				m.active = next
				// Initialise the section on its first
				// activation. The command is nil for a
				// section the user has visited before, so a
				// repeat switch is free.
				return m, m.initSection(next)
			}
		}

	case sessions.OpenRequestMsg:
		// The user pressed Enter on a session row. Build a fresh
		// transcript reader for the chosen session and open it as
		// the drill-down overlay. Seed it with the current
		// dimensions so its viewport sizes correctly before the
		// next resize message arrives.
		m.transcript = transcript.New(m.app, m.keys, m.theme, m.glamourStyle, msg.SessionID, msg.ProjectID, msg.Provider)
		m.showTranscript = true
		var sizeCmd tea.Cmd
		if m.width > 0 && m.height > 0 {
			m.transcript, sizeCmd = m.transcript.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		}
		return m, tea.Batch(m.transcript.Init(), sizeCmd)

	case transcript.BackMsg:
		// The user pressed Esc inside the transcript reader.
		// Close the overlay and return to the section they came
		// from, with its state intact.
		m.showTranscript = false
		return m, nil
	}

	return m.forward(msg)
}

// forward sends the message to whichever view is active — the
// transcript overlay when it is open, otherwise the active
// section's screen.
func (m appModel) forward(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.showTranscript {
		var cmd tea.Cmd
		m.transcript, cmd = m.transcript.Update(msg)
		return m, cmd
	}
	var cmd tea.Cmd
	m.screens[m.active], cmd = m.screens[m.active].Update(msg)
	return m, cmd
}

// isFiltering reports whether the active section is the session
// list and it is currently capturing filter input. The app checks
// this before it claims a global keystroke (quit, a section
// switch) so those keys reach the filter input instead of acting
// on the program while the user is typing a query.
func (m appModel) isFiltering() bool {
	if m.showTranscript {
		return false
	}
	s, ok := m.screens[m.active].(sessionsScreen)
	return ok && s.IsFiltering()
}

// sectionForKeypress maps a keypress to the section it should
// activate, if any. A number key jumps straight to the section in
// that position, and Tab and Shift-Tab cycle forward and backward
// through the order.
func (m appModel) sectionForKeypress(msg tea.KeyPressMsg) (section, bool) {
	pressed := msg.String()
	for _, sec := range m.order {
		if m.meta[sec].key == pressed {
			return sec, true
		}
	}
	switch pressed {
	case "tab":
		return m.cycle(1), true
	case "shift+tab":
		return m.cycle(-1), true
	}
	return 0, false
}

// cycle returns the section delta steps away from the active one
// in the tab order, wrapping around either end so Tab from the
// last section lands on the first.
func (m appModel) cycle(delta int) section {
	index := 0
	for i, sec := range m.order {
		if sec == m.active {
			index = i
			break
		}
	}
	n := len(m.order)
	return m.order[(index+delta+n)%n]
}

func (m appModel) View() tea.View {
	var content string
	if m.showTranscript {
		content = m.transcript.View()
	} else {
		content = renderChrome(m.width, m.order, m.meta, m.active, m.theme) +
			"\n" + m.screens[m.active].View()
	}
	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = "chronicle"
	return view
}
