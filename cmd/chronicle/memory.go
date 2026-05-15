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
		Short: "Inspect, edit, and prune memory files (per-project and user-global)",
		Long: `chronicle memory manages the memory files that AI coding
assistants write to remember things across sessions. Two
scopes are supported:

  Per-project memory: one MEMORY.md index file plus topic files
  (architecture.md, debugging.md, ...) under
  projects/<encoded-cwd>/memory/. Loaded into context at the start
  of every session in that project.

  User-global memory: one file (~/.claude/CLAUDE.md for Claude)
  loaded into every session regardless of project. Use --global
  on show, edit, and clean to target this scope.

When a memory file goes stale, the assistant keeps loading the
outdated information into every new session. Use 'chronicle
memory list' to see what is on disk, 'chronicle memory show' to
inspect one file, 'chronicle memory edit' to fix it, or
'chronicle memory clean' to wipe a scope (the wipe goes through
the trash, so a regretted clean is recoverable).`,
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
		Short: "List every memory file across providers (per-project and user-global)",
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
// columns so the eye can find the file the user wants. The
// project column shows "(global)" for user-global memory
// (entries with an empty ProjectID), so the user can tell
// at a glance which row is the global ~/.claude/CLAUDE.md
// versus a per-project MEMORY.md.
func writeMemoryList(w io.Writer, entries []composition.MemoryListing) error {
	if len(entries) == 0 {
		fmt.Fprintln(w, "No memory files found.")
		return nil
	}
	fmt.Fprintf(w, "%d memory %s:\n\n", len(entries), composition.Pluralize(len(entries), "file", "files"))
	for _, entry := range entries {
		project := string(entry.ProjectID)
		if project == "" {
			project = "(global)"
		}
		fmt.Fprintf(w, "  %-8s %s/%s  (%s, %s)\n",
			entry.Provider,
			project,
			entry.FileName,
			composition.HumanBytes(entry.SizeBytes),
			entry.ModifiedAt,
		)
	}
	return nil
}

// newMemoryShowCmd builds `chronicle memory show`. The
// command has two shapes:
//
//	chronicle memory show <project> <file>      # per-project
//	chronicle memory show --global [<file>]     # user-global
//
// The --global flag picks the user-wide memory file (the
// adapter decides the canonical name, like CLAUDE.md for
// Claude). When the flag is set, the positional project
// argument is dropped and the filename defaults to whatever
// the active provider declares. The two shapes route
// through different composition methods so the contracts
// layer can keep the per-project and global capabilities
// separate.
func newMemoryShowCmd() *cobra.Command {
	var global bool
	cmd := &cobra.Command{
		Use:   "show <project> <file>",
		Short: "Print one memory file to stdout (use --global for user-wide memory)",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := composition.New()
			if err != nil {
				return fail("init: %v", err)
			}
			if global {
				fileName, err := resolveGlobalMemoryName(app, args)
				if err != nil {
					return fail("show: %v", err)
				}
				if err := app.ShowGlobalMemory(fileName, cmd.OutOrStdout()); err != nil {
					return fail("show: %v", err)
				}
				return nil
			}
			if len(args) != 2 {
				return fail("show: pass <project> <file>, or --global [<file>]")
			}
			project, fileName := contracts.ProjectID(args[0]), args[1]
			if err := app.ShowMemory(project, fileName, cmd.OutOrStdout()); err != nil {
				return fail("show: %v", err)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&global, "global", false, "Show the user-wide memory file (the active provider names the default)")
	return cmd
}

// resolveGlobalMemoryName picks the global memory filename
// from either the user-supplied positional argument or, if
// none was given, the active provider's declared default.
// We centralise this so the show, edit, and clean commands
// share one consistent rule for "what does --global without
// a name mean?" and so the per-command bodies stay focused
// on their own work.
func resolveGlobalMemoryName(app *composition.App, args []string) (string, error) {
	if len(args) >= 1 {
		return args[0], nil
	}
	return app.DefaultGlobalMemoryFile()
}

// newMemoryEditCmd builds `chronicle memory edit`. Same
// two-shape pattern as show:
//
//	chronicle memory edit <project> <file>      # per-project
//	chronicle memory edit --global [<file>]     # user-global
//
// The command spawns the user's $EDITOR on the file's
// absolute path. We resolve the path through composition,
// then exec the editor and connect its stdio to the
// terminal so the user gets the normal interactive
// experience.
//
// We default to vi when $EDITOR is unset, the same fallback
// git uses. A user who lands here without $EDITOR
// configured at least gets a working editor instead of an
// error message.
func newMemoryEditCmd() *cobra.Command {
	var global bool
	cmd := &cobra.Command{
		Use:   "edit <project> <file>",
		Short: "Open one memory file in $EDITOR (use --global for user-wide memory)",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := composition.New()
			if err != nil {
				return fail("init: %v", err)
			}
			fullPath, err := resolveMemoryEditPath(app, global, args)
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
	cmd.Flags().BoolVar(&global, "global", false, "Edit the user-wide memory file (the active provider names the default)")
	return cmd
}

// resolveMemoryEditPath does the per-project vs global
// branching for `chronicle memory edit`. We split the
// resolution out of the cobra wiring so the editor-spawning
// part stays linear and so a future test can exercise the
// branching without spawning a real editor.
func resolveMemoryEditPath(app *composition.App, global bool, args []string) (string, error) {
	if global {
		fileName, err := resolveGlobalMemoryName(app, args)
		if err != nil {
			return "", err
		}
		return app.EditGlobalMemoryPath(fileName)
	}
	if len(args) != 2 {
		return "", fmt.Errorf("pass <project> <file>, or --global [<file>]")
	}
	return app.EditMemoryPath(contracts.ProjectID(args[0]), args[1])
}

// newMemoryCleanCmd builds `chronicle memory clean`. Two
// shapes:
//
//	chronicle memory clean <project>            # per-project
//	chronicle memory clean --global             # user-global
//
// The command moves every memory file at the requested
// scope into the trash. Like the other clean commands, it
// defaults to dry-run and requires --apply to actually move
// files. The trash subsystem makes the operation
// reversible: a regretted clean can be undone with
// `chronicle trash restore <id>` until the trash entry ages
// out.
func newMemoryCleanCmd() *cobra.Command {
	var apply, global bool
	cmd := &cobra.Command{
		Use:   "clean <project>",
		Short: "Move every memory file at the chosen scope into the trash (dry-run by default)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := composition.New()
			if err != nil {
				return fail("init: %v", err)
			}
			var planned composition.PlannedDeletion
			switch {
			case global:
				if len(args) != 0 {
					return fail("clean: --global takes no positional arguments")
				}
				planned, err = app.CleanGlobalMemory()
			case len(args) == 1:
				planned, err = app.CleanProjectMemory(contracts.ProjectID(args[0]))
			default:
				return fail("clean: pass <project> or --global")
			}
			if err != nil {
				return fail("plan: %v", err)
			}
			return runClean(app, []composition.PlannedDeletion{planned}, apply, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&global, "global", false, "Clean the user-wide memory file instead of a project")
	cmd.Flags().BoolVar(&apply, "apply", false, "Actually move files (default is dry-run)")
	return cmd
}
