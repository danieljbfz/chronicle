package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/keys"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/theme"
	"github.com/danieljbfz/chronicle/composition"
)

// appModel is the top-level tea.Model. Its job is to own the
// state every screen shares (the composition.App handle, the
// theme, the key bindings, the current terminal dimensions) and
// to route incoming messages to whichever screen is active. The
// foundation build ships with a single welcome screen embedded
// directly in this file. Phase 1 introduces a Screen interface
// and a per-screen package, and this file shrinks to just the
// router.
type appModel struct {
	app     *composition.App
	keys    keys.KeyMap
	theme   theme.Theme
	version string

	width  int
	height int
}

func newAppModel(app *composition.App, k keys.KeyMap, t theme.Theme, version string) appModel {
	return appModel{
		app:     app,
		keys:    k,
		theme:   t,
		version: version,
	}
}

func (m appModel) Init() tea.Cmd {
	return nil
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyPressMsg:
		if key.Matches(msg, m.keys.Quit) {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m appModel) View() tea.View {
	title := m.theme.Title.Render(fmt.Sprintf("chronicle %s", m.version))

	body := strings.Join([]string{
		"The interactive terminal UI is under construction.",
		"Screens for the session list, transcripts, stats, doctor,",
		"trash, and memory will arrive one phase at a time.",
		"",
		"Run " + m.theme.Accent.Render("chronicle --help") + " for the CLI",
		"commands that already work.",
	}, "\n")

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		body,
		"",
		m.renderHelpBar(),
	)

	view := tea.NewView(content)
	view.AltScreen = true
	return view
}

// renderHelpBar prints the compact help line at the bottom of the
// screen. The line is currently short because the welcome screen
// only responds to the quit binding. As real screens come online
// this bar grows to cover the full set of bindings active for the
// current screen.
func (m appModel) renderHelpBar() string {
	q := m.theme.HelpKey.Render("q") + " " + m.theme.HelpDesc.Render("quit")
	return q
}
