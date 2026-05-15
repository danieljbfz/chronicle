package main

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/danieljbfz/chronicle/composition"
)

// newDoctorCmd builds the `chronicle doctor` subcommand. Doctor is
// the read-only diagnostic view. It tells the user which providers
// chronicle detected, which version of each one's storage was
// recognized, and surfaces any warnings, like an unknown storage
// fingerprint.
//
// The --json flag emits the same data as machine-readable JSON for
// scripting. The default output is plain text for humans.
func newDoctorCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Show detected providers, versions, fingerprints, and any warnings",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := composition.New()
			if err != nil {
				return fail("init: %v", err)
			}
			healths := app.Doctor()
			if asJSON {
				return writeDoctorJSON(cmd.OutOrStdout(), healths)
			}
			return writeDoctorText(cmd.OutOrStdout(), healths)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit results as JSON instead of text")
	return cmd
}

// writeDoctorJSON pretty-prints the health list as indented JSON.
// We use indented JSON because doctor output is read by humans even
// when --json is set, and the indentation makes the output readable
// without piping through jq.
func writeDoctorJSON(w io.Writer, healths []composition.ProviderHealth) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(healths)
}

// writeDoctorText renders the health list as a stack of plain-text
// blocks, one per provider. The format is meant to be skimmed:
// the provider name on its own line, followed by indented
// label-value pairs, then any errors and warnings as separate
// labeled lists.
func writeDoctorText(w io.Writer, healths []composition.ProviderHealth) error {
	if len(healths) == 0 {
		fmt.Fprintln(w, "No providers detected. Enable providers in ~/.config/chronicle/config.toml.")
		return nil
	}
	for _, h := range healths {
		fmt.Fprintf(w, "Provider: %s\n", h.Name)
		fmt.Fprintf(w, "  Root:        %s\n", h.Root)
		fmt.Fprintf(w, "  Version:     %s\n", h.Version.Version)
		if h.Version.Fingerprint != "" {
			fmt.Fprintf(w, "  Fingerprint: %s\n", h.Version.Fingerprint)
		}
		fmt.Fprintf(w, "  Reachable:   %v\n", h.Reachable)
		fmt.Fprintf(w, "  Sessions:    %d\n", h.SessionCount)
		for _, msg := range h.Errors {
			fmt.Fprintf(w, "  Error:       %s\n", msg)
		}
		for _, msg := range h.Warnings {
			fmt.Fprintf(w, "  Warning:     %s\n", msg)
		}
		fmt.Fprintln(w)
	}
	return nil
}
