package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/danieljbfz/chronicle/composition"
)

// newCleanConfigProjectsCmd builds the
// `chronicle clean config-projects` subcommand. Each
// supported provider keeps a user-wide config file with a
// per-project subsection (Claude's ~/.claude.json holds a
// `projects` map keyed by absolute working directory). The
// subcommand finds entries whose project directory has
// gone and offers to remove them. The original config
// file is backed up before any edit, and the edit
// preserves every other byte of the file untouched.
//
// Like the other clean subcommands, the default is
// dry-run. The user passes --apply to perform the actual
// removal.
//
// The subcommand is named "config-projects" instead of
// the shorter "config" because chronicle already has a
// top-level `config` command for chronicle's own
// configuration. The longer name disambiguates the two
// surfaces while staying close to natural reading
// ("clean config-projects [that have gone]").
func newCleanConfigProjectsCmd() *cobra.Command {
	var (
		apply        bool
		providerFlag string
	)
	cmd := &cobra.Command{
		Use:   "config-projects",
		Short: "Remove per-project config entries whose directory has gone (dry-run by default)",
		Long: `chronicle clean config-projects inspects each provider's
user-wide config file (such as Claude's ~/.claude.json) and
finds per-project entries whose directory no longer exists
on disk. With --apply, the entries are removed; the original
file is backed up next to any existing rotated backups before
the edit, and the edit uses surgical JSON path deletion so
every other field in the file stays byte-identical.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := composition.New()
			if err != nil {
				return fail("init: %v", err)
			}
			listings, err := app.ListConfigProjects(providerFlag)
			if err != nil {
				return fail("list: %v", err)
			}
			stale := filterStaleConfigEntries(listings)
			return runConfigProjectsCleanup(app, stale, apply, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "Actually remove entries (default is dry-run)")
	cmd.Flags().StringVar(&providerFlag, "provider", "", `Limit to one provider by name (see chronicle doctor for the list)`)
	return cmd
}

// filterStaleConfigEntries keeps only the listings whose
// underlying directory has gone. We do the filter in the
// CLI rather than in composition so a future TUI or API
// consumer can choose its own filter (e.g. "show all
// entries, not just the stale ones") without composition
// having to expose multiple knobs.
func filterStaleConfigEntries(all []composition.ConfigProjectListing) []composition.ConfigProjectListing {
	var stale []composition.ConfigProjectListing
	for _, l := range all {
		if !l.Entry.Exists {
			stale = append(stale, l)
		}
	}
	return stale
}

// runConfigProjectsCleanup prints the planned removals to
// the writer in a format the user can review at a glance.
// When apply is true, the function then performs the
// removal and reports the backup path the provider wrote.
//
// We split this body out of the cobra wiring so a future
// test can exercise the rendering and the apply path with
// a fake App, without going through the cobra machinery.
func runConfigProjectsCleanup(app *composition.App, stale []composition.ConfigProjectListing, apply bool, stdout io.Writer) error {
	if len(stale) == 0 {
		fmt.Fprintln(stdout, "No stale config-project entries. Every project the config file mentions still exists on disk.")
		return nil
	}

	var totalBytes int64
	for _, l := range stale {
		totalBytes += l.Entry.SizeBytes
	}

	fmt.Fprintf(stdout, "Found %d stale config-project entry(ies) (%s total).\n\n",
		len(stale), composition.HumanBytes(totalBytes))
	for _, l := range stale {
		fmt.Fprintf(stdout, "  %s  %s  (%s)\n",
			l.Provider, l.Entry.Key, composition.HumanBytes(l.Entry.SizeBytes))
	}
	fmt.Fprintln(stdout)

	if !apply {
		fmt.Fprintln(stdout, "(dry-run; pass --apply to remove these entries)")
		return nil
	}

	results, err := app.CleanConfigProjects(stale)
	if err != nil {
		return fail("apply: %v", err)
	}
	for _, r := range results {
		fmt.Fprintf(os.Stderr, "Removed %d entry(ies) from %s. Backup at %s\n",
			len(r.RemovedKeys), r.Provider, r.BackupPath)
	}
	return nil
}
