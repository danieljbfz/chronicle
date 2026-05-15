package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

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
		Short: "Find unwanted data (abandoned, stale, orphan, dangling) and remove it with --apply",
		Long: `chronicle clean has one subcommand per cleanup category.

Every clean command defaults to dry-run mode: it prints what
would be moved and exits without touching the filesystem. Pass
--apply to actually perform the move. Trashed items stay
recoverable through 'chronicle trash restore' until they age out
or you run 'chronicle trash empty --force'.`,
	}
	cmd.AddCommand(newCleanAbandonedCmd())
	cmd.AddCommand(newCleanOrphansCmd())
	cmd.AddCommand(newCleanStaleCmd())
	cmd.AddCommand(newCleanDanglingCmd())
	return cmd
}

// newCleanOrphansCmd builds the `chronicle clean orphans`
// subcommand. An orphan is a file left behind on disk after the
// session that owned it is gone, or floating junk like old
// shell snapshots and rotated configuration backups that have
// nothing to do with a specific session. Each adapter knows
// its own list of orphan kinds.
//
// The command is the safest way to recover disk space, because
// orphans by definition do not belong to any live session, so
// removing them cannot affect anything the user is currently
// using. Even so, the command defaults to dry-run, so the user
// always sees the plan before any file moves.
func newCleanOrphansCmd() *cobra.Command {
	var (
		apply        bool
		providerFlag string
	)
	cmd := &cobra.Command{
		Use:   "orphans",
		Short: "Find files left behind from gone sessions and floating junk (dry-run by default)",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := composition.New()
			if err != nil {
				return fail("init: %v", err)
			}
			planned, err := app.PlanCleanup([]composition.CleanCategory{composition.CategoryOrphans}, providerFlag)
			if err != nil {
				return fail("plan: %v", err)
			}
			return runClean(app, planned, apply, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "Actually move files (default is dry-run)")
	cmd.Flags().StringVar(&providerFlag, "provider", "", `Limit to one provider by name (see chronicle doctor for the list)`)
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
	cmd.Flags().StringVar(&providerFlag, "provider", "", `Limit to one provider by name (see chronicle doctor for the list)`)
	return cmd
}

// defaultStaleAge is the threshold the user gets when they
// pass --older-than with no value. Thirty days matches
// Claude's own cleanupPeriodDays default, so a Claude user
// who runs `chronicle clean stale` against a fresh install
// (with the upstream cleaner already enabled) sees an empty
// plan rather than surprise deletions of sessions Claude
// itself would have left alone.
const defaultStaleAge = "30d"

// newCleanStaleCmd builds the `chronicle clean stale`
// subcommand. A stale session is one whose last activity
// timestamp is older than --older-than. The default of 30d
// matches Claude Code's own retention default, so chronicle
// stays consistent with the upstream tool's notion of stale.
//
// The command is most useful for Copilot, which has no
// equivalent auto-cleaner. A Claude user with the upstream
// cleaner disabled (cleanupPeriodDays = 0 retains forever)
// can also use this to run an on-demand sweep.
func newCleanStaleCmd() *cobra.Command {
	var (
		apply        bool
		providerFlag string
		olderThan    string
	)
	cmd := &cobra.Command{
		Use:   "stale",
		Short: "Find sessions whose last activity is older than --older-than (dry-run by default)",
		RunE: func(cmd *cobra.Command, args []string) error {
			threshold, err := parseDayDuration(olderThan)
			if err != nil {
				return fail("--older-than: %v", err)
			}
			app, err := composition.New()
			if err != nil {
				return fail("init: %v", err)
			}
			planned, err := app.PlanCleanupStale(threshold, providerFlag)
			if err != nil {
				return fail("plan: %v", err)
			}
			return runClean(app, planned, apply, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "Actually move files (default is dry-run)")
	cmd.Flags().StringVar(&providerFlag, "provider", "", `Limit to one provider by name (see chronicle doctor for the list)`)
	cmd.Flags().StringVar(&olderThan, "older-than", defaultStaleAge,
		`Cutoff for "stale", written as a Go duration with an extra "d" suffix (e.g. "30d", "12h", "90d")`)
	return cmd
}

// parseDayDuration parses a duration string with a small
// extension over the standard time.ParseDuration: the "d"
// suffix is recognised and means days. We add this because
// retention thresholds are usually expressed in days, and
// time.ParseDuration's largest built-in unit is hour, which
// would force users to type "720h" for 30 days.
//
// The function delegates to time.ParseDuration for any
// input without the "d" suffix, so existing duration
// formats (1h30m, 5m, etc.) keep working. Negative
// durations are rejected because "older than -3 days" has
// no meaningful semantics in chronicle's cleanup flow.
func parseDayDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, fmt.Errorf("expected integer days before the 'd' suffix in %q", s)
		}
		if days < 0 {
			return 0, fmt.Errorf("negative duration %q", s)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	if d < 0 {
		return 0, fmt.Errorf("negative duration %q", s)
	}
	return d, nil
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

	fmt.Fprintf(stdout, "Found %d %s to clean (%s total).\n\n",
		len(planned), composition.Pluralize(len(planned), "session", "sessions"), composition.HumanBytes(totalBytes))
	for _, pd := range planned {
		fmt.Fprintf(stdout, "  %s/%s  (%s)\n",
			pd.ProviderName(), pd.Plan.SessionID, composition.HumanBytes(pd.Plan.SizeBytes))
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
	fmt.Fprintf(os.Stderr, "Moved %d %s into the trash.\n",
		len(entries), composition.Pluralize(len(entries), "session", "sessions"))
	for _, entry := range entries {
		fmt.Fprintf(os.Stderr, "  %s\n", entry.ID)
	}
	return nil
}
