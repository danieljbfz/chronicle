package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/danieljbfz/chronicle/composition"
)

// newTrashCmd builds the `chronicle trash` parent command. Its
// three subcommands cover the lifecycle of a trashed entry:
// list shows what is currently sitting in the trash, restore
// puts one entry back where it came from, and empty
// permanently removes entries to reclaim disk.
//
// The empty subcommand is the only place in chronicle that
// performs an unrecoverable delete. Its default behaviour
// removes only entries past the retention window in the user's
// config. The --force flag is the explicit opt-in to remove
// everything regardless of age. Keeping the dangerous knob
// behind a flag means the default invocation cannot lose work.
func newTrashCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "trash",
		Short: "List, restore, or empty the chronicle trash",
		Long: `chronicle trash manages the directory where 'chronicle clean'
moves session data. Items stay there until you restore them or
until they age past the retention window from your config (30
days by default). Pass --force to 'trash empty' to remove
everything regardless of age.`,
	}
	cmd.AddCommand(newTrashListCmd())
	cmd.AddCommand(newTrashRestoreCmd())
	cmd.AddCommand(newTrashEmptyCmd())
	return cmd
}

// newTrashListCmd builds `chronicle trash list`. Each entry
// gets one line with the four fields a user needs to decide
// what to do next: the entry ID for use with restore or empty,
// the provider name and a short prefix of the session ID, the
// total recoverable size, and a relative age like "3h ago".
func newTrashListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List recoverable entries currently in the trash",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := composition.New()
			if err != nil {
				return fail("init: %v", err)
			}
			entries, err := app.TrashList()
			if err != nil {
				return fail("list: %v", err)
			}
			return writeTrashList(cmd.OutOrStdout(), entries)
		},
	}
}

// writeTrashList prints the entries one per line. The format
// is plain text because a trash listing is a quick "what is in
// here?" view. Structured output would be overkill for the
// amount of information involved.
func writeTrashList(w io.Writer, entries []composition.TrashEntry) error {
	if len(entries) == 0 {
		fmt.Fprintln(w, "Trash is empty.")
		return nil
	}
	fmt.Fprintf(w, "%d %s in the trash:\n\n",
		len(entries), composition.Pluralize(len(entries), "entry", "entries"))
	for _, entry := range entries {
		fmt.Fprintf(w, "  %s\n", entry)
	}
	return nil
}

// newTrashRestoreCmd builds `chronicle trash restore <id>`. The
// user gets the entry ID from `chronicle trash list`. Restore
// refuses to overwrite an existing file at the destination, so
// if a session has somehow reappeared at the same path while
// the entry sat in the trash, the restore will fail with a
// clear error. The user has to move the existing file aside
// before they can restore on top of it.
func newTrashRestoreCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restore <entry-id>",
		Short: "Move one trashed entry back to its original location",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := composition.New()
			if err != nil {
				return fail("init: %v", err)
			}
			if err := app.TrashRestore(args[0]); err != nil {
				return fail("restore: %v", err)
			}
			fmt.Fprintf(os.Stderr, "Restored entry %s.\n", args[0])
			return nil
		},
	}
}

// newTrashEmptyCmd builds `chronicle trash empty`. By default,
// the command removes only entries past the retention window
// in the user's config. Pass --force to remove every entry
// regardless of age. In both cases the command prints the IDs
// of the entries it removed, so the user has a record of what
// just disappeared from disk.
func newTrashEmptyCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "empty",
		Short: "Permanently remove trash entries past the retention window",
		Long: `Without --force, 'trash empty' removes only entries older than
the retention window from your config (30 days by default). With
--force, it removes every entry regardless of age. This is the
only chronicle command that performs an unrecoverable delete.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := composition.New()
			if err != nil {
				return fail("init: %v", err)
			}
			removed, err := app.TrashEmpty(composition.TrashEmptyOptions{Force: force})
			if err != nil {
				return fail("empty: %v", err)
			}
			if len(removed) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Nothing to remove. Every entry is within its retention window.")
				return nil
			}
			fmt.Fprintf(os.Stderr, "Removed %d %s:\n",
				len(removed), composition.Pluralize(len(removed), "entry", "entries"))
			for _, id := range removed {
				fmt.Fprintf(os.Stderr, "  %s\n", id)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Remove every entry regardless of age (unrecoverable)")
	return cmd
}
