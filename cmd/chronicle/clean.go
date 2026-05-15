package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/danieljbfz/chronicle/composition"
)

// newCleanCmd builds the `chronicle clean` subcommand. Each
// cleanup category that chronicle knows about gets its own
// subcommand underneath. So the user types
// `chronicle clean abandoned` to sweep abandoned sessions, and
// future categories will land as `chronicle clean stale`,
// `chronicle clean orphans`, and so on.
//
// Every clean subcommand defaults to dry-run mode. The user has
// to pass --apply to actually move files. Defaulting to safe
// behaviour is the right shape for any destructive feature,
// especially one that can sweep up hundreds of files in a
// single command.
func newCleanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Find and (with --apply) move stale data into the trash",
		Long: `chronicle clean has one subcommand per cleanup category.

Every clean command defaults to dry-run mode: it prints what
would be moved and exits without touching the filesystem. Pass
--apply to actually perform the move. Trashed items stay
recoverable through 'chronicle trash restore' until they age out
or you run 'chronicle trash empty --force'.`,
	}
	cmd.AddCommand(newCleanAbandonedCmd())
	return cmd
}

// newCleanAbandonedCmd builds the `chronicle clean abandoned`
// subcommand. "Abandoned" means a session with zero real user
// prompts: the user opened the session, ran a meta command like
// /clear, and never typed anything. On a typical chronicle
// install, abandoned sessions are about one-fifth of every
// session file on disk.
func newCleanAbandonedCmd() *cobra.Command {
	var (
		apply        bool
		providerFlag string
	)
	cmd := &cobra.Command{
		Use:   "abandoned",
		Short: "Find sessions with zero real user prompts (dry-run by default)",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := composition.New()
			if err != nil {
				return fail("init: %v", err)
			}
			planned, err := app.PlanCleanup([]composition.CleanCategory{composition.CategoryAbandoned}, providerFlag)
			if err != nil {
				return fail("plan: %v", err)
			}
			return runClean(app, planned, apply, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "Actually move files (default is dry-run)")
	cmd.Flags().StringVar(&providerFlag, "provider", "", `Limit to one provider, like "claude"`)
	return cmd
}

// runClean prints the planned deletions to the writer in a
// format a person can review at a glance. When the caller
// passed --apply, the function then runs the cleanup against
// the filesystem. When --apply is missing, the function stops
// after printing and reminds the user to pass --apply if they
// want the cleanup to actually run.
//
// Splitting this body out of the cobra wiring lets future
// tests exercise the rendering and the apply path with a fake
// App, without going through the cobra command machinery.
func runClean(app *composition.App, planned []composition.PlannedDeletion, apply bool, stdout io.Writer) error {
	if len(planned) == 0 {
		fmt.Fprintln(stdout, "Nothing to clean. Every session looks active.")
		return nil
	}

	var totalBytes int64
	for _, pd := range planned {
		totalBytes += pd.Plan.SizeBytes
	}

	fmt.Fprintf(stdout, "Found %d session(s) to clean (%s total).\n\n",
		len(planned), humanBytes(totalBytes))
	for _, pd := range planned {
		fmt.Fprintf(stdout, "  %s/%s  (%s)\n",
			pd.ProviderName(), pd.Plan.SessionID, humanBytes(pd.Plan.SizeBytes))
		for _, item := range pd.Plan.Items {
			fmt.Fprintf(stdout, "    - %-22s %s\n", item.Reason, item.Path)
		}
		fmt.Fprintln(stdout)
	}

	if !apply {
		fmt.Fprintln(stdout, "(dry-run; pass --apply to move these into the trash)")
		return nil
	}

	entries, err := app.ExecuteCleanup(planned)
	if err != nil {
		return fail("execute: %v", err)
	}
	fmt.Fprintf(os.Stderr, "Moved %d session(s) into the trash.\n", len(entries))
	for _, entry := range entries {
		fmt.Fprintf(os.Stderr, "  %s\n", entry.ID)
	}
	return nil
}

// humanBytes turns a byte count into a short human-readable
// string. We keep our own copy here, even though composition
// has a near-identical helper, because the composition copy is
// an internal detail of the trash subsystem. Exporting it just
// to share five lines of formatting code would mean the CLI
// reaches into composition's presentation layer, which is the
// wrong direction for the dependency arrow.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(n)/float64(div), "KMGTPE"[exp])
}
