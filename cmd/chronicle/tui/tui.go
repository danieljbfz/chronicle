// Package tui drives the chronicle terminal user interface. The
// package's job is to take a fully constructed composition.App and
// launch the Bubble Tea program that owns the screen lifecycle.
// Every screen lives in its own sub-package under screens/, and
// the orchestration between screens lives in this package's
// app.go. The presentation layer never imports an adapter
// directly — every action and every read goes through methods on
// composition.App.
package tui

import (
	"fmt"
	"io"

	tea "charm.land/bubbletea/v2"

	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/keys"
	"github.com/danieljbfz/chronicle/cmd/chronicle/tui/theme"
	"github.com/danieljbfz/chronicle/composition"
)

// Options carry the construction-time settings the caller chooses
// before the program runs. The struct is intentionally small. We
// add fields here only when a new option is reached for, not
// preemptively.
type Options struct {
	// Theme picks the colour scheme. The zero value follows the
	// terminal's native palette.
	Theme theme.Variant

	// Version is the chronicle build string, shown on the welcome
	// screen and inside the help overlay. Pass the value from
	// main rather than reading a package-level variable here.
	Version string

	// Output is the destination Bubble Tea writes to. The zero
	// value (nil) lets Bubble Tea pick stdout itself, which is
	// what main wants. A test can substitute a buffer to capture
	// the rendered frames.
	Output io.Writer
}

// Run launches the TUI and blocks until the user exits or an
// unrecoverable error occurs. The function takes the already-built
// composition.App rather than constructing one itself, so the
// caller can configure the App once (for tests, for a one-off
// debug run) and reuse it.
func Run(app *composition.App, opts Options) error {
	if app == nil {
		return fmt.Errorf("tui: composition.App is nil")
	}

	model := newAppModel(app, keys.Default(), theme.New(opts.Theme), opts.Version)

	// Bubble Tea v2 sets the alt-screen flag per frame on the
	// tea.View value the screen returns, rather than through a
	// program option. Each screen's own View() decides whether
	// the frame is full-window or inline. The top-level app
	// model enables alt-screen for every frame, so the program
	// is full-window for its whole lifetime.
	var progOpts []tea.ProgramOption
	if opts.Output != nil {
		progOpts = append(progOpts, tea.WithOutput(opts.Output))
	}

	if _, err := tea.NewProgram(model, progOpts...).Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}
