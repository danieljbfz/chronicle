package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/danieljbfz/chronicle/composition"
	"github.com/danieljbfz/chronicle/contracts"
	"github.com/danieljbfz/chronicle/steps"
)

// newExportCmd builds the `chronicle export` subcommand. The user
// passes a session identifier and gets back a Markdown transcript,
// either on stdout or written to a file with -o.
func newExportCmd() *cobra.Command {
	var (
		noTools, noThinking, noMeta bool
		outPath                     string
	)
	cmd := &cobra.Command{
		Use:   "export <sessionId>",
		Short: "Write a filtered Markdown transcript to a file or stdout",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := composition.New()
			if err != nil {
				return fail("init: %v", err)
			}
			return runExport(app, contracts.SessionID(args[0]), exportOpts{
				noTools:    noTools,
				noThinking: noThinking,
				noMeta:     noMeta,
				outPath:    outPath,
			}, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&noTools, "no-tools", false, "Drop tool use and tool result blocks")
	cmd.Flags().BoolVar(&noThinking, "no-thinking", false, "Drop assistant thinking blocks")
	cmd.Flags().BoolVar(&noMeta, "no-meta", false, "Drop meta messages like slash-command echoes")
	cmd.Flags().StringVarP(&outPath, "out", "o", "", "Write to this file instead of stdout")
	return cmd
}

// exportOpts is the small bag of options runExport accepts. We
// pass them as one struct, not as separate function arguments, so
// the test code stays readable when new flags are added later.
type exportOpts struct {
	noTools, noThinking, noMeta bool
	outPath                     string
}

// runExport is the actual work of the export subcommand, separated
// from the cobra wiring so tests can call it with a fake App. It
// reads the session, applies the filters the user asked for,
// renders Markdown, and writes the result.
func runExport(app *composition.App, id contracts.SessionID, opts exportOpts, stdout io.Writer) error {
	conv, err := app.ReadSession(id)
	if err != nil {
		return fail("read session %q: %v", id, err)
	}
	conv = steps.Filter(conv, steps.FilterOptions{
		HideTools:    opts.noTools,
		HideThinking: opts.noThinking,
		HideMeta:     opts.noMeta,
	})
	md := steps.Markdown(conv)

	if opts.outPath == "" {
		_, err := io.WriteString(stdout, md)
		return err
	}
	if err := os.WriteFile(opts.outPath, []byte(md), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", opts.outPath, err)
	}
	fmt.Fprintf(os.Stderr, "Wrote %d bytes to %s\n", len(md), opts.outPath)
	return nil
}
