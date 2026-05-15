package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/danieljbfz/chronicle/composition"
)

// newConfigCmd builds the `chronicle config` parent command
// and its subcommands. Each subcommand answers one question
// the user routinely needs to ask about chronicle's own
// configuration:
//
//	show — what config is chronicle actually using right now?
//	edit — open it in $EDITOR
//	path — where is the file?
//
// We deliberately do not ship `get` or `set` subcommands.
// The config schema is small enough that the user can
// inspect it through `show` and edit it through `edit`,
// and dotted-path walkers would be configurable knobs
// nobody has asked for. If a future automation case shows
// up, we can add those then.
func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect, edit, or locate chronicle's configuration",
		Long: `chronicle config has three subcommands:

  show   Print the resolved config (defaults plus file overrides)
         as TOML. The output round-trips: pipe it back into
         the config file and you get the same Config back.
  edit   Open the config file in $EDITOR. Creates the parent
         directory if it does not exist yet.
  path   Print the absolute filesystem path of the config file.
         Useful for scripting.`,
	}
	cmd.AddCommand(newConfigShowCmd())
	cmd.AddCommand(newConfigEditCmd())
	cmd.AddCommand(newConfigPathCmd())
	return cmd
}

// newConfigShowCmd builds `chronicle config show`. The
// output is the resolved config as TOML, which means
// defaults merged with whatever the user's config file set.
// That is what the user actually wants to see when they ask
// "what is chronicle using right now?" The on-disk file
// alone would only show their overrides, hiding the values
// that come from defaults.
func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print the resolved config as TOML",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := composition.New()
			if err != nil {
				return fail("init: %v", err)
			}
			return writeConfigShow(cmd.OutOrStdout(), app)
		},
	}
}

// writeConfigShow renders the resolved config to the
// writer. We split the rendering out of the cobra wiring
// so a future test can call it with a fake App without
// going through the cobra command machinery.
func writeConfigShow(w io.Writer, app *composition.App) error {
	rendered, err := app.SettingsTOML()
	if err != nil {
		return fail("show: %v", err)
	}
	if _, err := io.WriteString(w, rendered); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// newConfigEditCmd builds `chronicle config edit`. The
// command spawns the user's $EDITOR on the config file. We
// create the parent directory and an empty file if either
// is missing, so a fresh-install user who runs the command
// before ever writing a config gets a working session
// instead of an error.
//
// The empty-file shape is safe because chronicle.Load
// already treats a missing file as "use defaults," and an
// empty TOML file is parsed the same way (zero key/value
// pairs to merge over the defaults). The user lands in
// their editor with a blank canvas they can fill in.
func newConfigEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit",
		Short: "Open the config file in $EDITOR",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := composition.New()
			if err != nil {
				return fail("init: %v", err)
			}
			path := app.Locations().ConfigFile
			if err := ensureConfigFileExists(path); err != nil {
				return fail("edit: %v", err)
			}
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vi"
			}
			editorCmd := exec.Command(editor, path)
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

// ensureConfigFileExists makes the parent directory and the
// file itself when either is missing. The function is
// idempotent: running it twice in a row is a no-op the
// second time. We prefer this over calling MkdirAll inline
// because the operation has two distinct concerns (parent
// dir, then file) that each get their own clear failure
// path.
func ensureConfigFileExists(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("ensure config file: %w", err)
	}
	return f.Close()
}

// newConfigPathCmd builds `chronicle config path`. The
// command prints the absolute filesystem path of the
// config file. Useful for scripts that want to back the
// file up before editing, or for users wondering where
// chronicle keeps its own configuration.
func newConfigPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the absolute path of the config file",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := composition.New()
			if err != nil {
				return fail("init: %v", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), app.Locations().ConfigFile)
			return nil
		},
	}
}
