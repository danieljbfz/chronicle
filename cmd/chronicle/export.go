package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/danieljbfz/chronicle/composition"
	"github.com/danieljbfz/chronicle/contracts"
	"github.com/danieljbfz/chronicle/steps"
)

// newExportCmd builds the `chronicle export` subcommand. It
// has two modes that share the same filter and rendering
// pipeline. The default mode takes one session identifier
// and writes a Markdown transcript to a file or stdout.
// The bulk mode takes a project identifier and writes one
// Markdown file per session into a directory.
//
// We model the two modes through one cobra command rather
// than two subcommands because they share every flag that
// matters (the filters, the destination shape) and a user
// who knows `chronicle export <id>` already knows
// `chronicle export --bulk <project> -o dir` from the
// surface alone.
func newExportCmd() *cobra.Command {
	var (
		noTools, noThinking, noMeta bool
		noAwaySummaries, noFiles    bool
		outPath                     string
		bulkProject                 string
		providerName                string
	)
	cmd := &cobra.Command{
		Use:   "export <sessionId>",
		Short: "Write a filtered Markdown transcript to a file or stdout",
		Long: `chronicle export writes Markdown transcripts of one or many sessions.

Single-session mode (default):
  chronicle export <sessionId>          write to stdout
  chronicle export <sessionId> -o file  write to one file

Bulk mode (one project at a time):
  chronicle export --bulk <projectId> -o <directory>

Bulk writes one Markdown file per session into the destination
directory. Each file is named with the session's start date
and identifier so 'ls' shows them in chronological order.
The directory is created if it does not exist.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := composition.New()
			if err != nil {
				return fail("init: %v", err)
			}
			opts := exportOpts{
				noTools:         noTools,
				noThinking:      noThinking,
				noMeta:          noMeta,
				noAwaySummaries: noAwaySummaries,
				noFiles:         noFiles,
				outPath:         outPath,
			}
			if bulkProject != "" {
				return runBulkExport(app, contracts.ProjectID(bulkProject), providerName, opts, cmd.ErrOrStderr())
			}
			if len(args) != 1 {
				return fail("missing session id (or pass --bulk <projectId>)")
			}
			return runExport(app, contracts.SessionID(args[0]), opts, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().BoolVar(&noTools, "no-tools", false, "Drop tool use and tool result blocks")
	cmd.Flags().BoolVar(&noThinking, "no-thinking", false, "Drop assistant thinking blocks")
	cmd.Flags().BoolVar(&noMeta, "no-meta", false, "Drop meta messages like slash-command echoes")
	cmd.Flags().BoolVar(&noAwaySummaries, "no-away-summaries", false, "Drop step-away session summaries")
	cmd.Flags().BoolVar(&noFiles, "no-files", false, "Drop attached, edited, and selected file content")
	cmd.Flags().StringVarP(&outPath, "out", "o", "", "Write to this file (single mode) or directory (bulk mode)")
	cmd.Flags().StringVar(&bulkProject, "bulk", "", "Export every session in one project. Value is the project id.")
	cmd.Flags().StringVar(&providerName, "provider", "", `Disambiguate when more than one provider knows the project (see chronicle doctor for the list)`)
	return cmd
}

// exportOpts is the small bag of options runExport accepts.
// We pass them as one struct, not as separate function
// arguments, so the test code stays readable when new
// flags are added later. Bulk export reuses the same shape
// because the filter knobs apply to both modes identically.
type exportOpts struct {
	noTools, noThinking, noMeta bool
	noAwaySummaries, noFiles    bool
	outPath                     string
}

// runExport is the actual work of the single-session export
// subcommand, separated from the cobra wiring so tests can
// call it with a fake App. It reads the session, applies
// the filters the user asked for, renders Markdown, and
// writes the result.
//
// The rendered Markdown is the primary output. When no -o
// path is set it goes to stdout, ready to pipe. When a path
// is set the Markdown goes to the file and the "wrote N bytes"
// line goes to stderr as status, matching the runBulkExport
// sibling in this file. Both writers arrive as parameters so
// a test can capture either stream rather than the process's
// own os.Stdout and os.Stderr.
func runExport(app *composition.App, id contracts.SessionID, opts exportOpts, stdout, stderr io.Writer) error {
	conv, err := app.ReadSession(id)
	if err != nil {
		return fail("read session %q: %v", id, err)
	}
	conv = steps.Filter(conv, steps.FilterOptions{
		HideTools:         opts.noTools,
		HideThinking:      opts.noThinking,
		HideMeta:          opts.noMeta,
		HideAwaySummaries: opts.noAwaySummaries,
		HideFileContext:   opts.noFiles,
	})
	md := steps.Markdown(conv)

	if opts.outPath == "" {
		_, err := io.WriteString(stdout, md)
		return err
	}
	if err := os.WriteFile(opts.outPath, []byte(md), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", opts.outPath, err)
	}
	fmt.Fprintf(stderr, "Wrote %d bytes to %s\n", len(md), opts.outPath)
	return nil
}

// runBulkExport handles `chronicle export --bulk`. It
// validates the destination, creates it if missing, and
// streams sessions out through composition's BulkExport
// callback. The progress and summary lines go to stderr so
// stdout stays free in case a future flag wants to dump
// the manifest there for piping.
func runBulkExport(app *composition.App, projectID contracts.ProjectID, providerName string, opts exportOpts, stderr io.Writer) error {
	if opts.outPath == "" {
		return fail("bulk export requires -o <directory>")
	}
	if err := os.MkdirAll(opts.outPath, 0o755); err != nil {
		return fail("create destination %s: %v", opts.outPath, err)
	}

	bulkOpts := composition.BulkExportOptions{
		Provider:          providerName,
		HideTools:         opts.noTools,
		HideThinking:      opts.noThinking,
		HideMeta:          opts.noMeta,
		HideAwaySummaries: opts.noAwaySummaries,
		HideFileContext:   opts.noFiles,
	}

	var totalBytes int64
	count, err := app.BulkExport(projectID, bulkOpts, func(entry composition.BulkExportEntry) error {
		filename := bulkExportFilename(entry)
		full := filepath.Join(opts.outPath, filename)
		if err := os.WriteFile(full, []byte(entry.Content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", full, err)
		}
		totalBytes += int64(len(entry.Content))
		return nil
	})
	if err != nil {
		return bulkExportFailure(projectID, err)
	}
	fmt.Fprintf(stderr, "Wrote %d %s (%s) to %s\n",
		count, composition.Pluralize(count, "session", "sessions"), composition.HumanBytes(totalBytes), opts.outPath)
	return nil
}

// bulkExportFilename builds the per-session filename.
// Prefixing with the start date means a plain `ls` shows
// the directory in chronological order, which matches how
// users think about session histories. We fall back to the
// session id alone when the start timestamp is missing
// (older sessions or hand-edited fixtures), so the output
// always has a stable filename even when one piece of
// metadata is absent.
func bulkExportFilename(entry composition.BulkExportEntry) string {
	if entry.StartedAt.IsZero() {
		return string(entry.SessionID) + ".md"
	}
	return entry.StartedAt.UTC().Format("2006-01-02") + "_" + string(entry.SessionID) + ".md"
}

// bulkExportFailure translates the composition-layer error
// into a user-facing message. We branch on the two
// sentinel errors so the user sees actionable guidance
// instead of a wrapped chain that ends in
// "fs.ErrNotExist".
func bulkExportFailure(projectID contracts.ProjectID, err error) error {
	switch {
	case errors.Is(err, composition.ErrProjectAmbiguous):
		return fail("project %q exists in more than one provider. Pass --provider to choose one.", projectID)
	case errors.Is(err, fs.ErrNotExist):
		return fail("project %q not found. Use `chronicle list` to see what is available.", projectID)
	default:
		return fail("bulk export: %v", err)
	}
}
