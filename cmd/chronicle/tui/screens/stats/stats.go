// Package stats renders the cross-provider summary the chronicle
// TUI shows on its stats section. The screen reads one
// composition.Stats value — the same one the `chronicle stats`
// command renders — and lays it out as a totals block followed by
// three tables: a per-provider breakdown, the top projects by
// session count, and a by-model breakdown. The content lives in a
// scrolling viewport so a terminal too short to show every table
// at once still reaches all of it with the same j/k/u/d/g/G keys
// the transcript reader uses.
//
// The screen meets the project's accessibility bar: every action
// is reachable by keyboard, the loading and error states are full
// sentences, and the tables size themselves to the terminal width
// so a narrow window truncates cells rather than wrapping a row
// onto a second line.
package stats

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"

	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/keys"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/theme"
	"github.com/danieljbfz/chronicle/composition"
)

// Source is the small subset of composition.App the stats screen
// reads. Defining it here lets production pass a *composition.App
// and tests pass a fake without dragging the rest of composition
// along. *composition.App satisfies it through its Stats method.
type Source interface {
	Stats(composition.StatsOptions) (composition.Stats, error)
}

// status names the screen's current loading state. The three
// values are mutually exclusive: the model is loading, ready, or
// in an error state at any moment, never two at once.
type status int

const (
	statusLoading status = iota
	statusReady
	statusError
)

// Model is the stats screen's tea.Model. The fields fall into the
// same three groups every screen uses: inputs the constructor
// receives and never mutates, derived state the model owns, and
// the dimensions the runtime sets through WindowSizeMsg.
type Model struct {
	src   Source
	keys  keys.KeyMap
	theme theme.Theme

	viewport viewport.Model
	status   status
	err      error

	// stats is the summary the loader returned. The model keeps
	// it so a window resize can re-render the tables at the new
	// width without asking composition for the data again.
	stats composition.Stats

	width  int
	height int
}

// New returns a Model in its loading state. Init kicks off the
// asynchronous fetch through src.Stats, and the "Computing the
// summary" message stays on screen until the resulting loadedMsg
// or errMsg arrives.
func New(src Source, k keys.KeyMap, t theme.Theme) Model {
	vp := viewport.New(viewport.WithWidth(0), viewport.WithHeight(0))
	vp.MouseWheelEnabled = true

	return Model{
		src:      src,
		keys:     k,
		theme:    t,
		viewport: vp,
		status:   statusLoading,
	}
}

// Init returns the command that loads the summary for the first
// frame.
func (m Model) Init() tea.Cmd {
	return m.fetch(m.width)
}

// fetch returns a command that asks the Source for the summary,
// renders it at the given width, and yields a loadedMsg with the
// rendered output. A read error collapses into an errMsg so the
// screen has one error state rather than two. The width is a
// parameter rather than a model field read because the command
// runs after Update has returned, and a width of zero falls back
// to a sensible default so the first frame still produces
// something readable.
func (m Model) fetch(width int) tea.Cmd {
	src := m.src
	t := m.theme
	if width <= 0 {
		width = defaultRenderWidth
	}
	return func() tea.Msg {
		s, err := src.Stats(composition.StatsOptions{})
		if err != nil {
			return errMsg{err: fmt.Errorf("read stats: %w", err)}
		}
		return loadedMsg{stats: s, rendered: renderStats(s, width, t)}
	}
}

type loadedMsg struct {
	stats    composition.Stats
	rendered string
}

type errMsg struct {
	err error
}

const (
	footerLines        = 2
	defaultRenderWidth = 100
	minContentWidth    = 40
)

// Update advances the screen one frame. The screen handles the
// window resize, the asynchronous load result, and the top and
// bottom jumps. Every other message flows into the viewport so
// the line-by-line and page-by-page scrolling the bubbles
// viewport already implements works without extra wiring.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		viewportHeight := msg.Height - footerLines
		if viewportHeight < 1 {
			viewportHeight = 1
		}
		m.viewport.SetWidth(msg.Width)
		m.viewport.SetHeight(viewportHeight)
		// Re-render the tables at the new width, but only once
		// the summary is in hand. Before the load completes the
		// first render still runs through the fetch path.
		if m.status == statusReady {
			m.viewport.SetContent(renderStats(m.stats, m.contentWidth(), m.theme))
		}
	case loadedMsg:
		m.stats = msg.stats
		m.viewport.SetContent(msg.rendered)
		m.status = statusReady
		return m, nil
	case errMsg:
		m.err = msg.err
		m.status = statusError
		return m, nil
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keys.Top):
			m.viewport.GotoTop()
			return m, nil
		case key.Matches(msg, m.keys.Bottom):
			m.viewport.GotoBottom()
			return m, nil
		case key.Matches(msg, m.keys.Refresh):
			m.status = statusLoading
			return m, m.fetch(m.contentWidth())
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// contentWidth is the width the tables render against. The
// viewport spans the full terminal width, so the content width is
// the terminal width clamped to a floor that keeps the tables
// from collapsing into unreadable single-character columns on a
// very narrow window.
func (m Model) contentWidth() int {
	if m.width < minContentWidth {
		return minContentWidth
	}
	return m.width
}

// View renders the screen's content below the app's tab strip.
// The body is the loading or error sentence or the viewport, and
// the footer carries the divider and the short help line.
func (m Model) View() string {
	return m.renderBody() + "\n" + m.renderFooter()
}

func (m Model) renderBody() string {
	switch m.status {
	case statusLoading:
		return m.theme.Muted.Render("Computing the summary across every provider…")
	case statusError:
		return m.theme.Error.Render("Could not compute stats: "+m.err.Error()) +
			"\n\n" +
			m.theme.Muted.Render("Run `chronicle doctor` for the per-provider diagnostic, or press r to retry.")
	case statusReady:
		return m.viewport.View()
	}
	return ""
}

func (m Model) renderFooter() string {
	width := m.width
	if width < minContentWidth {
		width = minContentWidth
	}
	divider := m.theme.Muted.Render(strings.Repeat("─", width))
	return divider + "\n" + m.renderHelp()
}

// renderHelp prints the short binding hints the footer carries.
// The set is curated for the stats screen: it scrolls, it
// refreshes, and the global section and quit keys round out the
// line so the user can leave without guessing.
func (m Model) renderHelp() string {
	entries := []struct{ keyHint, desc string }{
		{"↑/k", "up"},
		{"↓/j", "down"},
		{"u/d", "half page"},
		{"g/G", "top/bottom"},
		{"r", "refresh"},
		{"1-5", "section"},
		{"q", "quit"},
	}
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		parts = append(parts, m.theme.HelpKey.Render(e.keyHint)+" "+m.theme.HelpDesc.Render(e.desc))
	}
	return strings.Join(parts, m.theme.Muted.Render("  ·  "))
}

// renderStats lays the summary out as a string the viewport
// scrolls. The order is totals first, then the per-provider,
// top-projects, and by-model tables, each only when it has rows.
// The width bounds every table so the whole document fits the
// terminal without a row spilling onto a second line.
func renderStats(s composition.Stats, width int, t theme.Theme) string {
	var b strings.Builder

	// Step 1: the totals block, the single most useful glance.
	b.WriteString(t.Title.Render("Totals"))
	b.WriteByte('\n')
	writeTotalsLine(&b, t, "Sessions", composition.HumanInt(s.Total.Sessions))
	writeTotalsLine(&b, t, "Messages", composition.HumanInt(s.Total.Messages))
	writeTotalsLine(&b, t, "Disk", composition.HumanBytes(s.Total.SizeBytes))
	if r := dateRange(s.Total); r != "" {
		writeTotalsLine(&b, t, "Active", r)
	}

	// Step 2: the per-provider, top-projects, and by-model
	// tables, each rendered only when it has rows so an install
	// with one provider or no model metadata does not show an
	// empty frame.
	if len(s.Providers) > 0 {
		b.WriteString("\n\n")
		b.WriteString(t.Title.Render("By provider"))
		b.WriteByte('\n')
		b.WriteString(providerTable(s.Providers, width, t))
	}
	if len(s.TopProjects) > 0 {
		b.WriteString("\n\n")
		b.WriteString(t.Title.Render(fmt.Sprintf("Top %d %s by session count",
			len(s.TopProjects), composition.Pluralize(len(s.TopProjects), "project", "projects"))))
		b.WriteByte('\n')
		b.WriteString(projectTable(s.TopProjects, width, t))
	}
	if len(s.ByModel) > 0 {
		b.WriteString("\n\n")
		b.WriteString(t.Title.Render("By model"))
		b.WriteByte('\n')
		b.WriteString(modelTable(s.ByModel, width, t))
	}

	return b.String()
}

// totalsLabelWidth is the column the totals labels pad to so the
// values line up in a clean second column.
const totalsLabelWidth = 10

func writeTotalsLine(b *strings.Builder, t theme.Theme, label, value string) {
	fmt.Fprintf(b, "  %s%s\n",
		t.Muted.Render(fmt.Sprintf("%-*s", totalsLabelWidth, label)),
		value)
}

// newTable returns a table pre-styled the way every stats table
// is: a muted normal border, accent-bold headers, and one space
// of padding on each side of a cell. Sharing the constructor
// keeps the three tables visually identical and the per-table
// code down to its headers and rows.
func newTable(width int, t theme.Theme) *table.Table {
	return table.New().
		Width(width).
		Border(lipgloss.NormalBorder()).
		BorderStyle(t.Muted).
		StyleFunc(func(row, _ int) lipgloss.Style {
			if row == table.HeaderRow {
				return t.Accent.Padding(0, 1)
			}
			return lipgloss.NewStyle().Padding(0, 1)
		})
}

func providerTable(rows []composition.ProviderStats, width int, t theme.Theme) string {
	tbl := newTable(width, t).Headers("PROVIDER", "SESSIONS", "MESSAGES", "DISK", "PROJECTS")
	for _, p := range rows {
		tbl.Row(
			p.Name,
			composition.HumanInt(p.Aggregate.Sessions),
			composition.HumanInt(p.Aggregate.Messages),
			composition.HumanBytes(p.Aggregate.SizeBytes),
			composition.HumanInt(p.Projects),
		)
	}
	return tbl.Render()
}

func projectTable(rows []composition.ProjectStats, width int, t theme.Theme) string {
	tbl := newTable(width, t).Headers("PROVIDER", "PROJECT", "SESSIONS", "DISK")
	for _, p := range rows {
		tbl.Row(
			p.Provider,
			projectLabel(p),
			composition.HumanInt(p.Aggregate.Sessions),
			composition.HumanBytes(p.Aggregate.SizeBytes),
		)
	}
	return tbl.Render()
}

func modelTable(rows []composition.ModelStats, width int, t theme.Theme) string {
	tbl := newTable(width, t).Headers("MODEL", "SESSIONS", "MESSAGES", "DISK")
	for _, m := range rows {
		tbl.Row(
			modelLabel(m.Model),
			composition.HumanInt(m.Aggregate.Sessions),
			composition.HumanInt(m.Aggregate.Messages),
			composition.HumanBytes(m.Aggregate.SizeBytes),
		)
	}
	return tbl.Render()
}

// projectLabel picks the most recognizable name for a project
// row. The on-disk path is the most useful when the adapter
// resolved one, the display name is the next best, and the raw
// identifier is the last resort so the row is never blank.
func projectLabel(p composition.ProjectStats) string {
	if p.Path != "" {
		return p.Path
	}
	if p.DisplayName != "" {
		return p.DisplayName
	}
	return string(p.ProjectID)
}

// modelLabel renders the model identifier, mapping the empty
// string to "(unknown)" the same way the CLI does, so sessions
// whose adapter recorded no model still read clearly in the
// table rather than as a blank cell.
func modelLabel(model string) string {
	if model == "" {
		return "(unknown)"
	}
	return model
}

// dateRange formats the oldest-to-newest span of the aggregate as
// one short line, or the empty string when no session contributed
// a timestamp so the caller can omit the line rather than print a
// confusing zero-value range.
func dateRange(a composition.Aggregate) string {
	if a.OldestAt.IsZero() || a.NewestAt.IsZero() {
		return ""
	}
	days := int(a.NewestAt.Sub(a.OldestAt).Hours() / 24)
	return fmt.Sprintf("%s → %s  (%d %s)",
		a.OldestAt.Format("2006-01-02"),
		a.NewestAt.Format("2006-01-02"),
		days,
		composition.Pluralize(days, "day", "days"),
	)
}
