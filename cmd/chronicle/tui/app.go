package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/keys"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/screens/sessions"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/screens/transcript"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/theme"
	"github.com/danieljbfz/chronicle/composition"
)

// screenID names one of the screens the app can be on. The app
// model holds the value for each screen in a dedicated field
// rather than behind an interface, because the per-screen
// behaviour differs enough that a type switch is more readable
// than a virtual call. Phases 3 through 6 add the remaining
// screen identifiers.
type screenID int

const (
	screenSessions screenID = iota
	screenTranscript
)

// appModel is the top-level tea.Model. The model owns the state
// every screen shares, the per-screen Models that exist so far,
// and the screen identifier that selects which one is active.
// The app routes messages to the active screen and intercepts a
// small set of cross-screen intents (OpenRequest, Back, Quit)
// before forwarding the rest.
type appModel struct {
	app          *composition.App
	keys         keys.KeyMap
	theme        theme.Theme
	version      string
	glamourStyle string

	width  int
	height int

	current  screenID
	sessions sessions.Model

	// transcript is the value of the transcript reader for
	// whichever session the user last opened. The zero value is
	// safe; the field is repopulated through transcript.New
	// every time the user opens a session, so the model always
	// reflects the most recently requested transcript.
	transcript transcript.Model
}

func newAppModel(app *composition.App, k keys.KeyMap, t theme.Theme, version, glamourStyle string) appModel {
	return appModel{
		app:          app,
		keys:         k,
		theme:        t,
		version:      version,
		glamourStyle: glamourStyle,
		current:      screenSessions,
		sessions:     sessions.New(app, k, t),
	}
}

func (m appModel) Init() tea.Cmd {
	return m.sessions.Init()
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Forward the resize to every screen that exists,
		// not just the active one, so a screen the user
		// later switches to already has the right dimensions
		// the moment it appears.
		var sessionsCmd, transcriptCmd tea.Cmd
		m.sessions, sessionsCmd = m.sessions.Update(msg)
		m.transcript, transcriptCmd = m.transcript.Update(msg)
		return m, tea.Batch(sessionsCmd, transcriptCmd)

	case tea.KeyPressMsg:
		// Global keys take precedence over screen-level keys.
		// The Quit binding is guarded against the session
		// list's filter mode so a user typing "quit" into the
		// filter does not accidentally exit the program.
		if key.Matches(msg, m.keys.Quit) && !m.sessions.IsFiltering() {
			return m, tea.Quit
		}

	case sessions.OpenRequestMsg:
		// The user pressed Enter on a session row. Build a
		// fresh transcript reader for the chosen session and
		// switch to it. The transcript's Init command kicks
		// off the read-and-render pipeline.
		m.transcript = transcript.New(m.app, m.keys, m.theme, m.glamourStyle, msg.SessionID, msg.ProjectID, msg.Provider)
		m.current = screenTranscript
		// Seed the new transcript with the current terminal
		// dimensions so its viewport sizes correctly before
		// the next resize message arrives.
		var sizeCmd tea.Cmd
		if m.width > 0 && m.height > 0 {
			m.transcript, sizeCmd = m.transcript.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		}
		return m, tea.Batch(m.transcript.Init(), sizeCmd)

	case transcript.BackMsg:
		// The user pressed Esc inside the transcript reader.
		// Return to the session list and let the list keep
		// the focus it had when the user opened the session.
		m.current = screenSessions
		return m, nil
	}

	return m.forward(msg)
}

// forward sends the message to whichever screen is active. The
// switch is the one place that needs an extra branch when a new
// screen joins.
func (m appModel) forward(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.current {
	case screenSessions:
		var cmd tea.Cmd
		m.sessions, cmd = m.sessions.Update(msg)
		return m, cmd
	case screenTranscript:
		var cmd tea.Cmd
		m.transcript, cmd = m.transcript.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m appModel) View() tea.View {
	var content string
	switch m.current {
	case screenSessions:
		content = m.sessions.View()
	case screenTranscript:
		content = m.transcript.View()
	}
	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = "chronicle"
	return view
}
