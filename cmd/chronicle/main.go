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
	"os"
	"time"

	"github.com/spf13/cobra"
)

// version is the chronicle version string. We bump it by hand for
// now. A later plan can switch to a build-time injected version
// once we have a release process.
var version = "0.1.0-plan-a"

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
func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "chronicle",
		Short:         "Browse, export, and clean the history of AI coding assistants",
		Long:          "chronicle reads ~/.claude (and in later plans VS Code Copilot storage),\nrenders sessions as Markdown, and helps you clean up the mess.",
		SilenceUsage:  true,
		SilenceErrors: false,
		Version:       version,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newExportCmd())
	cmd.AddCommand(newCopyCmd())
	cmd.AddCommand(newDoctorCmd())
	return cmd
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
// omitted field rather than as a misleading zero year.
func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
