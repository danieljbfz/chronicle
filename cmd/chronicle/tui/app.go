package tui

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/keys"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/screens/sessions"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/theme"
	"github.com/danieljbfz/chronicle/composition"
)

// appModel is the top-level tea.Model. Its job is to own the
// state every screen shares (the composition.App handle, the
// theme, the key bindings, the current terminal dimensions, the
// per-screen Models) and to route incoming messages to whichever
// screen is active. The model also handles the global key
// bindings — Quit today, with Help and Back joining the set as
// later phases need them — before forwarding the remaining
// messages to the active screen.
//
// Phase 1 ships one real screen, the session list. The app model
// keeps the value of the sessions screen as a direct field
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

	// status holds a short notice the app shows above the screen
	// content for the next few frames. The session list emits an
	// OpenRequestMsg on Enter, and phase 2 will switch screens
	// in response. Until then, the app surfaces the request as a
	// transient status line so a user can still verify that the
	// wiring runs end to end.
	status string
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
		// The sessions screen needs the dimensions minus the
		// header line the app model draws above it, so the
		// embedded list does not paint over the header on
		// resize.
		var cmd tea.Cmd
		m.sessions, cmd = m.sessions.Update(tea.WindowSizeMsg{
			Width:  msg.Width,
			Height: msg.Height - headerHeight,
		})
		return m, cmd
	case tea.KeyPressMsg:
		// Global keys take precedence over screen-level keys.
		// Today the only global is Quit, with Help and Back to
		// follow.
		if key.Matches(msg, m.keys.Quit) && !m.sessions.IsFiltering() {
			return m, tea.Quit
		}
	case sessions.OpenRequestMsg:
		// Phase 2 will replace this branch with a switch to the
		// transcript reader screen. Phase 1 surfaces the request
		// as a status notice so the user sees that Enter on a
		// session row produced the right intent.
		m.status = fmt.Sprintf("Transcript reader is wiring up. Press Enter on %s once it lands.", string(msg.SessionID))
		return m, nil
	}

	var cmd tea.Cmd
	m.sessions, cmd = m.sessions.Update(msg)
	return m, cmd
}

func (m appModel) View() tea.View {
	header := m.renderHeader()
	body := m.sessions.View()

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		body,
	)

	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = "chronicle"
	return view
}

// headerHeight is the number of terminal rows the app's header
// section uses up. The header has one line for the title or the
// status message, and one blank line below it for breathing
// room.
const headerHeight = 2

func (m appModel) renderHeader() string {
	if m.status != "" {
		return m.theme.Accent.Render(m.status) + "\n"
	}
	return m.theme.Title.Render(fmt.Sprintf("chronicle %s", m.version)) + "\n"
}
