package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/danieljbfz/chronicle/composition"
)

// newCleanDanglingCmd builds the `chronicle clean dangling`
// subcommand. Each supported provider keeps a user-wide
// config file with a per-project subsection (Claude's
// ~/.claude.json holds a `projects` map keyed by absolute
// working directory). Over time, those maps accumulate
// entries that point at directories the user has since
// deleted from disk. The pointer is still in the file but
// the target is gone, which is the textbook definition of
// a dangling reference. This subcommand finds those
// entries and (with --apply) removes them.
//
// Like the other clean subcommands, the default is
// dry-run. The user passes --apply to perform the actual
// removal.
//
// "Dangling" is the standard CS term for a reference
// whose target no longer exists (dangling pointer,
// dangling symlink). It reads as a real English single-
// word adjective alongside the sibling subcommands
// (abandoned, orphans, stale).
func newCleanDanglingCmd() *cobra.Command {
	var (
		apply        bool
		providerFlag string
	)
	cmd := &cobra.Command{
		Use:   "dangling",
		Short: "Remove per-project config entries whose directory has gone (dry-run by default)",
		Long: `chronicle clean dangling inspects each provider's user-wide
config file (such as Claude's ~/.claude.json) and finds
per-project entries whose directory no longer exists on
disk. Such entries are dangling references: the pointer is
still in the config but the target is gone.

With --apply, the entries are removed. The original config
file is backed up next to any existing rotated backups
before the edit, and the edit uses surgical JSON path
deletion so every other field in the file stays
byte-identical (formatting, key order, and number
precision are preserved).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := composition.New()
			if err != nil {
				return fail("init: %v", err)
			}
			listings, err := app.ListConfigProjects(providerFlag)
			if err != nil {
				return fail("list: %v", err)
			}
			dangling := filterDanglingEntries(listings)
			return runDanglingCleanup(app, dangling, apply, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "Actually remove entries (default is dry-run)")
	cmd.Flags().StringVar(&providerFlag, "provider", "", `Limit to one provider by name (see chronicle doctor for the list)`)
	return cmd
}

// filterDanglingEntries keeps only the listings whose
// underlying directory has gone. We do the filter in the
// CLI rather than in composition so a future TUI or API
// consumer can choose its own filter (for example "show
// all entries, not just the dangling ones") without
// composition having to expose multiple knobs.
func filterDanglingEntries(all []composition.ConfigProjectListing) []composition.ConfigProjectListing {
	var dangling []composition.ConfigProjectListing
	for _, l := range all {
		if !l.Entry.Exists {
			dangling = append(dangling, l)
		}
	}
	return dangling
}

// runDanglingCleanup prints the planned removals to the
// writer in a format the user can review at a glance.
// When apply is true, the function then performs the
// removal and reports the backup path the provider wrote.
//
// We split this body out of the cobra wiring so a future
// test can exercise the rendering and the apply path with
// a fake App, without going through the cobra machinery.
func runDanglingCleanup(app *composition.App, dangling []composition.ConfigProjectListing, apply bool, stdout io.Writer) error {
	if len(dangling) == 0 {
		fmt.Fprintln(stdout, "No dangling config-project entries. Every project the config file mentions still exists on disk.")
		return nil
	}

	var totalBytes int64
	for _, l := range dangling {
		totalBytes += l.Entry.SizeBytes
	}

	fmt.Fprintf(stdout, "Found %d dangling config-project %s (%s total).\n\n",
		len(dangling), composition.Pluralize(len(dangling), "entry", "entries"), composition.HumanBytes(totalBytes))
	for _, l := range dangling {
		fmt.Fprintf(stdout, "  %s  %s  (%s)\n",
			l.Provider, l.Entry.Key, composition.HumanBytes(l.Entry.SizeBytes))
	}
	fmt.Fprintln(stdout)

	if !apply {
		fmt.Fprintln(stdout, "(dry-run; pass --apply to remove these entries)")
		return nil
	}

	results, err := app.CleanConfigProjects(dangling)
	if err != nil {
		return fail("apply: %v", err)
	}
	for _, r := range results {
		fmt.Fprintf(os.Stderr, "Removed %d %s from %s. Backup at %s\n",
			len(r.RemovedKeys), composition.Pluralize(len(r.RemovedKeys), "entry", "entries"), r.Provider, r.BackupPath)
	}
	return nil
}
