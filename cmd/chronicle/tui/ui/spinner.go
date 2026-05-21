package ui

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/theme"
)

// Spinner is the loading indicator every screen shows while an
// asynchronous fetch is in flight. The visual is a bubbles
// spinner glyph followed by a short full-sentence message and a
// live elapsed-time counter, so the user sees not only that
// chronicle is working but how long it has been working. A
// motionless "Loading…" message is the problem this component
// is built to avoid.
//
// The screen holds a Spinner value alongside its loading-state
// flag. TickCmd is batched into the screen's Init so the glyph
// begins animating. Update advances the glyph on every
// spinner.TickMsg. View renders the row whenever the screen is
// in its loading state. The screen flips to its ready or error
// state by changing its own status field, and View stops
// returning the spinner row.
type Spinner struct {
	inner   spinner.Model
	theme   theme.Theme
	message string
	started time.Time
}

// NewSpinner returns a Spinner configured for the chronicle
// theme. The message is the full-sentence prompt the loading
// row carries beside the glyph, and the elapsed-time counter
// resets from the moment NewSpinner returns, so the screen
// constructs its spinner at the same moment it enters the
// loading state.
func NewSpinner(t theme.Theme, message string) Spinner {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = t.Muted
	return Spinner{
		inner:   s,
		theme:   t,
		message: message,
		started: time.Now(),
	}
}

// TickCmd returns the command that starts the spinner's tick
// loop. The screen batches this into Init so the glyph animates
// from the first frame, and into any later command that flips
// the screen back to a loading state (a refresh, a retry) so
// the animation resumes without the screen reaching into the
// spinner package directly.
func (s Spinner) TickCmd() tea.Cmd {
	return s.inner.Tick
}

// Update advances the spinner one frame. The screen forwards
// every message through here. Non-tick messages are no-ops, so
// the call is safe to make on every Update without filtering.
func (s Spinner) Update(msg tea.Msg) (Spinner, tea.Cmd) {
	updated, cmd := s.inner.Update(msg)
	s.inner = updated
	return s, cmd
}

// View renders the spinner row. The result is one line: the
// animated glyph, the screen's message, and a parenthesised
// elapsed-time reading that refreshes with every tick. The
// elapsed reading switches units from seconds to minutes once
// the fetch passes a minute, so a chronically slow load reads
// as "(2m 04s)" rather than "(124.3 s)".
func (s Spinner) View() string {
	return s.inner.View() + " " +
		s.message + " " +
		s.theme.Muted.Render(formatElapsed(time.Since(s.started)))
}

// formatElapsed prints the duration in the shape the spinner
// row reads. Under one minute the value reads as a decisecond-
// precision count of seconds, the resolution a human can
// usefully track. Past one minute the value switches to
// "Xm YYs" so the row stays under ten characters even on a
// multi-minute load.
func formatElapsed(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("(%.1f s)", d.Seconds())
	}
	minutes := int(d / time.Minute)
	seconds := int((d % time.Minute) / time.Second)
	return fmt.Sprintf("(%dm %02ds)", minutes, seconds)
}
