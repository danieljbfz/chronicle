package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/danieljbfz/chronicle/composition"
	"github.com/danieljbfz/chronicle/contracts"
)

// newResumeCmd builds the `chronicle resume` subcommand.
// Resume is the convenience that turns "I see this old
// session in `chronicle list`, I want to keep working on
// it" into one command. Without it, the user has to
// remember the session id, switch terminals, cd into the
// original project directory, and type the underlying
// tool's resume invocation by hand.
//
// Resume is provider-agnostic by design. The composition
// layer asks the session's owning provider for a launch
// plan, and the CLI runs the plan with stdio attached to
// the terminal. Today only the Claude adapter implements
// the Resumable capability, because Copilot Chat lives
// inside VS Code with no external API to jump to a
// specific chat by identifier. Trying to resume a Copilot
// session prints a clear "this provider does not support
// resume" message instead of guessing at a workaround.
func newResumeCmd() *cobra.Command {
	var printOnly bool
	cmd := &cobra.Command{
		Use:   "resume <session-id>",
		Short: "Reopen a stored session in its original tool",
		Long: `chronicle resume locates the session, reads the working directory
the original tool recorded inside it, and re-launches the tool
with --resume in that directory.

Use --print to see the exact command and directory chronicle
would run, without launching anything. That is useful for
scripting (chronicle resume --print can feed shell automation),
for debugging an unexpected resume target, and for any case
where you want to launch the tool manually with extra flags.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := composition.New()
			if err != nil {
				return fail("init: %v", err)
			}
			result, err := app.Resume(contracts.SessionID(args[0]))
			if err != nil {
				return resumeFailure(err)
			}
			if printOnly {
				return writeResumePrint(cmd.OutOrStdout(), result)
			}
			return runResume(cmd.OutOrStdout(), cmd.ErrOrStderr(), result)
		},
	}
	cmd.Flags().BoolVar(&printOnly, "print", false, "Print the command and working directory instead of running them")
	return cmd
}

// resumeFailure turns the composition error into a
// user-facing message. The two error sentinels carry their
// own meaning: fs.ErrNotExist means "no provider knows that
// id," and ErrResumeUnsupported means "we found it but
// cannot relaunch from outside." Both deserve different
// guidance, so we branch on them rather than surfacing the
// raw wrapped error.
func resumeFailure(err error) error {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return fail("session not found in any provider. Use `chronicle list` to see what is available.")
	case errors.Is(err, composition.ErrResumeUnsupported):
		return fail("%v. Reopen this session inside the underlying tool's UI.", err)
	default:
		return fail("resume: %v", err)
	}
}

// writeResumePrint renders the plan as plain text. The
// format is a single label-value block plus a one-liner the
// user can copy and paste into a shell. The label-value
// block names the provider and the directory so the user
// can tell at a glance what is about to run, and the
// shell-line at the end is the actual command, properly
// quoted, ready to paste somewhere else.
func writeResumePrint(w io.Writer, result composition.ResumeResult) error {
	fmt.Fprintf(w, "Provider:    %s\n", result.Provider)
	fmt.Fprintf(w, "WorkingDir:  %s\n", result.Plan.WorkingDir)
	fmt.Fprintf(w, "Command:     %s\n", strings.Join(result.Plan.Command, " "))
	fmt.Fprintf(w, "\nShell:       cd %s && %s\n",
		shellQuote(result.Plan.WorkingDir),
		shellJoin(result.Plan.Command),
	)
	return nil
}

// runResume verifies the working directory and the
// executable, then execs the command with stdio attached to
// the terminal. The signal-forwarding semantics come for
// free from os/exec: a Ctrl-C in chronicle propagates to the
// child, and the child's exit code becomes chronicle's exit
// code through the *exec.ExitError unwrap.
//
// We run the verification step ahead of exec because a
// missing directory or a missing executable produces a
// much more useful error here than the cryptic "no such
// file or directory" the underlying syscall would emit.
func runResume(stdout io.Writer, stderr io.Writer, result composition.ResumeResult) error {
	if len(result.Plan.Command) == 0 {
		return fail("resume: empty command from provider %s", result.Provider)
	}
	if err := checkWorkingDir(result.Plan.WorkingDir); err != nil {
		return fail("%v", err)
	}
	if _, err := exec.LookPath(result.Plan.Command[0]); err != nil {
		return fail("cannot find %q on PATH (install the tool or add it to PATH): %v",
			result.Plan.Command[0], err)
	}

	fmt.Fprintf(stderr, "chronicle: resuming %s session in %s\n",
		result.Provider, result.Plan.WorkingDir)

	cmd := exec.Command(result.Plan.Command[0], result.Plan.Command[1:]...)
	cmd.Dir = result.Plan.WorkingDir
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fail("%s exited with error: %v", result.Plan.Command[0], err)
	}
	return nil
}

// checkWorkingDir verifies the cwd exists and is a
// directory before chronicle tries to chdir into it. The
// usual reason a recorded cwd is missing is that the user
// moved or deleted the project. We return an error that
// names both the directory and the likely cause, so the
// user can fix it by re-creating the directory or by
// resuming inside the new path manually.
func checkWorkingDir(dir string) error {
	if dir == "" {
		return fmt.Errorf("resume: provider returned an empty working directory")
	}
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("working directory %s is not reachable (was the project moved or deleted?): %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("working directory %s exists but is not a directory", dir)
	}
	return nil
}

// shellQuote wraps a path in single quotes if it contains
// any character that would otherwise need escaping. The
// goal is a string the user can paste safely into bash,
// not a perfect POSIX-compliant escape (which would
// require a much larger character class). For directory
// paths chronicle produces, single-quoting on demand is
// enough.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') &&
			r != '/' && r != '_' && r != '-' && r != '.' {
			return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
		}
	}
	return s
}

// shellJoin renders the argv slice as a shell-friendly
// command line. We single-quote each token through
// shellQuote and join them with spaces, so the output is
// safe to paste even when an argument contains spaces or
// other shell metacharacters.
func shellJoin(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		parts[i] = shellQuote(a)
	}
	return strings.Join(parts, " ")
}
