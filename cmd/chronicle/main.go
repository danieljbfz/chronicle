// chronicle is the local tool for browsing, exporting, and
// cleaning the on-disk history that AI coding assistants leave
// behind. The main package is intentionally thin. It builds the
// cobra command tree and runs it. All real work lives in the
// composition package and below.
//
// The cobra library is the de facto standard for Go CLIs. It gives
// us subcommands, flags, help text, and shell completion for free.
// In Python, the closest equivalent would be Click or Typer.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/danieljbfz/chronicle/cmd/chronicle/tui"
	"github.com/danieljbfz/chronicle/composition"
	"github.com/danieljbfz/chronicle/internal/config"
)

// version is the chronicle version string. We bump it by hand for
// now and can switch to a build-time injected version once we
// have a release process. The standard Go pattern is to declare
// `var version = "dev"` here and override it at build time with
// `go build -ldflags "-X main.version=1.2.3"`, which is exactly
// what we will do once the first release goes out.
var version = "0.1.0"

func main() {
	if err := newRootCmd().Execute(); err != nil {
		// Cobra has already printed the error to stderr by the
		// time we get here. Our only job is to exit non-zero so
		// the shell knows the command failed.
		os.Exit(1)
	}
}

// newRootCmd builds the top-level command. Subcommands are added
// from their own files in this same package, so each subcommand can
// keep its flags and its run function next to each other.
//
// The root command's RunE launches the interactive TUI. Cobra only
// calls RunE when no subcommand matches the user's arguments, so
// `chronicle` with no arguments enters the TUI while `chronicle
// list`, `chronicle export`, and the other subcommands keep their
// existing behaviour.
func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "chronicle",
		Short:         "Browse, export, and clean the history of AI coding assistants",
		Long:          "chronicle reads the on-disk history of AI coding assistants,\nrenders sessions as Markdown, and helps you clean up the mess.\n\nClaude Code is supported today. GitHub Copilot, Cursor, and others\nare coming.",
		SilenceUsage:  true,
		SilenceErrors: false,
		Version:       version,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Step 1: build the composition layer that every
			// screen reads through. composition.New already
			// loads the user's config and resolves paths
			// internally, so the App's Settings accessor is the
			// single source the TUI options reach for below.
			app, err := composition.New()
			if err != nil {
				return fail("init: %v", err)
			}

			// Step 2: translate the user's TUI config values
			// into the runtime options the tui package expects.
			// Unknown values fall back to the documented
			// defaults with a one-line stderr warning so the
			// user sees the substitution rather than wondering
			// why their choice did not take effect.
			opts := tuiOptionsFromConfig(app.Settings().UI.TUI, version, os.Stderr)
			return tui.Run(app, opts)
		},
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newExportCmd())
	cmd.AddCommand(newCopyCmd())
	cmd.AddCommand(newDoctorCmd())
	cmd.AddCommand(newSearchCmd())
	cmd.AddCommand(newStatsCmd())
	cmd.AddCommand(newResumeCmd())
	cmd.AddCommand(newCleanCmd())
	cmd.AddCommand(newTrashCmd())
	cmd.AddCommand(newMemoryCmd())
	cmd.AddCommand(newConfigCmd())
	return cmd
}

// tuiOptionsFromConfig translates the user's [ui.tui] section
// into the runtime options the tui package consumes. The
// function is the configuration boundary the rulebook
// describes: it validates the user-supplied values, warns when
// an unknown value cannot be honoured, and produces a struct
// the TUI internals can trust without re-checking.
//
// The warnings go to the provided writer (stderr in production,
// a buffer in any future test) on their own line, so a user
// who runs chronicle redirected to a script still sees what
// chronicle could not honour and why. Each warning quotes the
// value that failed, lists the values chronicle accepts, and
// names the fallback chronicle used in its place — the
// user-facing-error rules in section 4.4 of the project
// rulebook spell that shape out.
func tuiOptionsFromConfig(cfg config.TUIConfig, version string, warn io.Writer) tui.Options {
	// Step 1: resolve the colour scheme. The empty string and
	// "auto" both fall through to the terminal palette, which
	// is the chronicle default.
	themeName := strings.TrimSpace(cfg.Theme)
	variant, ok := tui.ParseTheme(themeName)
	if !ok {
		fmt.Fprintf(warn,
			"chronicle: the ui.tui.theme value %q is not a chronicle theme. Chronicle accepts %s, and falls back to the terminal palette.\n",
			themeName, tui.JoinKnownThemes())
	}

	// Step 2: resolve the glamour stylesheet. An empty value
	// keeps the documented default. An unknown value warns and
	// falls back to the same default rather than letting
	// glamour fail at render time with a less helpful message.
	style := strings.TrimSpace(cfg.GlamourStyle)
	switch {
	case style == "":
		style = tui.DefaultGlamourStyle
	case !tui.IsKnownGlamourStyle(style):
		fmt.Fprintf(warn,
			"chronicle: the ui.tui.glamour_style value %q is not a glamour v2 stylesheet. Chronicle accepts %s, and falls back to %q.\n",
			style, tui.JoinKnownGlamourStyles(), tui.DefaultGlamourStyle)
		style = tui.DefaultGlamourStyle
	}

	return tui.Options{
		Theme:        variant,
		GlamourStyle: style,
		Version:      version,
	}
}

// fail is the small helper subcommands use when they want to print
// a "chronicle: <message>" line to stderr and return a non-nil
// error. Returning the error tells cobra to exit non-zero, which
// makes chronicle play nicely with shell pipelines and CI.
func fail(format string, args ...any) error {
	fmt.Fprintf(os.Stderr, "chronicle: "+format+"\n", args...)
	return fmt.Errorf(format, args...)
}

// fmtTime formats a timestamp as RFC 3339, or returns the empty
// string for the zero value. We use this in the JSON output of the
// list command, where an empty timestamp should appear as an
// omitted field instead of a misleading zero year.
func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
