package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/danieljbfz/chronicle/composition"
	"github.com/danieljbfz/chronicle/contracts"
	"github.com/danieljbfz/chronicle/steps"
)

// newCopyCmd builds the `chronicle copy` subcommand. The user
// passes a session identifier, and chronicle puts the rendered
// Markdown into the system clipboard via OSC 52. Because OSC 52
// works over SSH, the copy reaches the user's local clipboard even
// when chronicle itself is running on a remote machine.
func newCopyCmd() *cobra.Command {
	var noTools, noThinking bool
	cmd := &cobra.Command{
		Use:   "copy <sessionId>",
		Short: "Copy a filtered Markdown transcript to the clipboard via OSC 52",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := composition.New()
			if err != nil {
				return fail("init: %v", err)
			}
			conv, err := app.ReadSession(contracts.SessionID(args[0]))
			if err != nil {
				return fail("read session %q: %v", args[0], err)
			}
			conv = steps.Filter(conv, steps.FilterOptions{
				HideTools:    noTools,
				HideThinking: noThinking,
			})
			md := steps.Markdown(conv)
			if err := steps.CopyOSC52(cmd.OutOrStdout(), md); err != nil {
				return fail("clipboard: %v", err)
			}
			fmt.Fprintf(os.Stderr, "Copied %d bytes to clipboard via OSC 52.\n", len(md))
			return nil
		},
	}
	cmd.Flags().BoolVar(&noTools, "no-tools", false, "Drop tool use and tool result blocks")
	cmd.Flags().BoolVar(&noThinking, "no-thinking", false, "Drop assistant thinking blocks")
	return cmd
}
