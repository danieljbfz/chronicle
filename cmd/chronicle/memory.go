package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/danieljbfz/chronicle/composition"
	"github.com/danieljbfz/chronicle/contracts"
)

// newMemoryCmd builds the `chronicle memory` parent command
// and its subcommands. The memory feature lets the user
// inspect, edit, and selectively prune the per-project
// memory files that AI coding assistants write to remember
// things across sessions.
//
// The motivating problem: when one of these memory files
// goes stale, the assistant keeps loading the old (now
// wrong) information at the start of every new session in
// that project. The user has no easy way today to figure
// out which file is responsible without rummaging through
// the filesystem. `chronicle memory` makes that visible.
//
// All four subcommands work across every provider that
// implements the contracts.MemoryStore optional interface.
// Today only Claude does, but the surface is ready for any
// future tool that ships per-project memory.
func newMemoryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "memory",
		Short: "Inspect, edit, and prune per-project memory files",
		Long: `chronicle memory manages the per-project memory files that AI
coding assistants write to remember things across sessions.

A typical memory directory has one MEMORY.md index file plus
topic files like architecture.md or debugging.md. The index is
loaded into context at the start of every session in that
project. Topic files load on demand based on the conversation.

When a memory file goes stale, the assistant keeps loading the
outdated information into every new session. Use 'chronicle
memory list' to see what is on disk, 'chronicle memory show' to
inspect one file, 'chronicle memory edit' to fix it, or
'chronicle memory clean <project>' to wipe a project's memory
entirely (the wipe goes through the trash, so a regretted clean
is recoverable).`,
	}
	cmd.AddCommand(newMemoryListCmd())
	cmd.AddCommand(newMemoryShowCmd())
	cmd.AddCommand(newMemoryEditCmd())
	cmd.AddCommand(newMemoryCleanCmd())
	return cmd
}

// newMemoryListCmd builds `chronicle memory list`. The output
// shows one line per memory file with the fields the user
// needs to decide what to inspect or prune: the project
// identifier, the filename, the size, and the
// last-modified date. A memory file that has not changed in
// months is a strong candidate for the prune workflow.
func newMemoryListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List every per-project memory file across providers",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := composition.New()
			if err != nil {
				return fail("init: %v", err)
			}
			entries, err := app.ListMemories()
			if err != nil {
				return fail("list: %v", err)
			}
			return writeMemoryList(cmd.OutOrStdout(), entries)
		},
	}
}

// writeMemoryList prints the entries one per line in plain
// text. The format is meant to be skimmed: provider, project,
// filename, size, and modification date in fixed-width
// columns so the eye can find the file the user wants.
func writeMemoryList(w io.Writer, entries []composition.MemoryListing) error {
	if len(entries) == 0 {
		fmt.Fprintln(w, "No per-project memory files found.")
		return nil
	}
	fmt.Fprintf(w, "%d memory file(s):\n\n", len(entries))
	for _, entry := range entries {
		fmt.Fprintf(w, "  %-8s %s/%s  (%s, %s)\n",
			entry.Provider,
			entry.ProjectID,
			entry.FileName,
			composition.HumanBytes(entry.SizeBytes),
			entry.ModifiedAt,
		)
	}
	return nil
}

// newMemoryShowCmd builds `chronicle memory show <project> <file>`.
// The output goes to stdout, ready to pipe into a pager
// (`chronicle memory show ... | less`) or grep.
func newMemoryShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <project> <file>",
		Short: "Print one memory file to stdout",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := composition.New()
			if err != nil {
				return fail("init: %v", err)
			}
			project, fileName := contracts.ProjectID(args[0]), args[1]
			if err := app.ShowMemory(project, fileName, cmd.OutOrStdout()); err != nil {
				return fail("show: %v", err)
			}
			return nil
		},
	}
}

// newMemoryEditCmd builds `chronicle memory edit <project> <file>`.
// The command spawns the user's `$EDITOR` on the file's
// absolute path. We resolve the path through composition,
// then exec the editor and connect its stdio to the
// terminal so the user gets the normal interactive
// experience.
//
// We default to `vi` when `$EDITOR` is unset, the same
// fallback git uses. A user who lands here without `$EDITOR`
// configured at least gets a working editor instead of an
// error message.
func newMemoryEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit <project> <file>",
		Short: "Open one memory file in $EDITOR",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := composition.New()
			if err != nil {
				return fail("init: %v", err)
			}
			project, fileName := contracts.ProjectID(args[0]), args[1]
			fullPath, err := app.EditMemoryPath(project, fileName)
			if err != nil {
				return fail("edit: %v", err)
			}
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vi"
			}
			editorCmd := exec.Command(editor, fullPath)
			editorCmd.Stdin = os.Stdin
			editorCmd.Stdout = os.Stdout
			editorCmd.Stderr = os.Stderr
			if err := editorCmd.Run(); err != nil {
				return fail("editor exited with error: %v", err)
			}
			return nil
		},
	}
}

// newMemoryCleanCmd builds `chronicle memory clean <project>`.
// The command moves every memory file in one project into
// the trash. Like the other clean commands, it defaults to
// dry-run and requires --apply to actually move files. The
// trash subsystem makes the operation reversible: a
// regretted clean can be undone with `chronicle trash
// restore <id>` until the trash entry ages out.
func newMemoryCleanCmd() *cobra.Command {
	var apply bool
	cmd := &cobra.Command{
		Use:   "clean <project>",
		Short: "Move every memory file in one project into the trash (dry-run by default)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := composition.New()
			if err != nil {
				return fail("init: %v", err)
			}
			project := contracts.ProjectID(args[0])
			planned, err := app.CleanProjectMemory(project)
			if err != nil {
				return fail("plan: %v", err)
			}
			return runClean(app, []composition.PlannedDeletion{planned}, apply, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "Actually move files (default is dry-run)")
	return cmd
}
