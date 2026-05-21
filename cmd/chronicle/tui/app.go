package tui

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/keys"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/screens/sessions"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/theme"
	"github.com/danieljbfz/chronicle/composition"
)

// appModel is the top-level tea.Model. Its job is to own the
// state every screen shares (the composition.App handle, the
// theme, the key bindings, the current terminal dimensions, the
// per-screen Models) and to route incoming messages to whichever
// screen is active. The app model also handles the global key
// bindings — Quit today, with Help and Back joining the set as
// later phases need them — before forwarding the remaining
// messages to the active screen.
//
// Phase 1 ships one real screen, the session list. The app model
// holds the value of the sessions screen as a direct field
// rather than behind a Screen interface, because there is no
// other screen to compare against yet. The Screen interface and
// the screen-switching logic land in phase 2 when the transcript
// reader joins.
type appModel struct {
	app     *composition.App
	keys    keys.KeyMap
	theme   theme.Theme
	version string

	width  int
	height int

	sessions sessions.Model
}

func newAppModel(app *composition.App, k keys.KeyMap, t theme.Theme, version string) appModel {
	return appModel{
		app:      app,
		keys:     k,
		theme:    t,
		version:  version,
		sessions: sessions.New(app, k, t),
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
		var cmd tea.Cmd
		m.sessions, cmd = m.sessions.Update(msg)
		return m, cmd
	case tea.KeyPressMsg:
		// Global keys take precedence over screen-level keys.
		// Today the only global is Quit, with Help and Back to
		// follow. The filter check stops a user typing into the
		// list's filter input from accidentally quitting the
		// program on a "q" keystroke.
		if key.Matches(msg, m.keys.Quit) && !m.sessions.IsFiltering() {
			return m, tea.Quit
		}
	case sessions.OpenRequestMsg:
		// Phase 2 will replace this branch with a switch to the
		// transcript reader screen. Phase 1 surfaces the request
		// through the list's transient status bar so the user
		// sees that Enter on a row produced the right intent.
		var cmd tea.Cmd
		notice := fmt.Sprintf("Transcript reader is wiring up · session %s queued", string(msg.SessionID))
		m.sessions, cmd = m.sessions.PublishStatusMessage(notice)
		return m, cmd
	}

	var cmd tea.Cmd
	m.sessions, cmd = m.sessions.Update(msg)
	return m, cmd
}

func (m appModel) View() tea.View {
	view := tea.NewView(m.sessions.View())
	view.AltScreen = true
	view.WindowTitle = "chronicle"
	return view
}
