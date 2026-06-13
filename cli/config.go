package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

// configPath returns the resolved config file path (XDG config dir).
func configPath() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "ytb", "config.toml")
}

const configTemplate = `# ytb CLI configuration
# Keys mirror the global flags. Uncomment to override the built-in defaults.

# output    = "auto"      # table|json|jsonl|csv|tsv|url|id|raw
# workers   = 4
# rate      = "1.5s"
# retries   = 3
# timeout   = "30s"
# hl        = "en"
# gl        = "US"
# db        = ""           # path to the optional SQLite store
# user_agent = ""
# yt_dlp_bin = "yt-dlp"
`

func newConfigCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "View and manage configuration",
	}
	cmd.AddCommand(
		newConfigShowCmd(app),
		newConfigPathCmd(app),
		newConfigInitCmd(app),
		newConfigEditCmd(app),
	)
	return cmd
}

func newConfigShowCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print the resolved configuration",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			c := app.Cfg
			_, _ = fmt.Fprintf(cmdOut, "workers = %d\n", c.Workers)
			_, _ = fmt.Fprintf(cmdOut, "rate    = %s\n", c.Delay)
			_, _ = fmt.Fprintf(cmdOut, "retries = %d\n", c.Retries)
			_, _ = fmt.Fprintf(cmdOut, "timeout = %s\n", c.Timeout)
			_, _ = fmt.Fprintf(cmdOut, "hl      = %s\n", c.HL)
			_, _ = fmt.Fprintf(cmdOut, "gl      = %s\n", c.GL)
			_, _ = fmt.Fprintf(cmdOut, "db      = %s\n", app.DBPath)
			return nil
		},
	}
}

func newConfigPathCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the config file path",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			_, _ = fmt.Fprintln(cmdOut, configPath())
			return nil
		},
	}
}

func newConfigInitCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Write a commented config template",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			path := configPath()
			if _, err := os.Stat(path); err == nil {
				if !confirm(app.yes, "Config already exists at "+path+". Overwrite?") {
					return usageErr("aborted")
				}
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(path, []byte(configTemplate), 0o644); err != nil {
				return err
			}
			app.logf("wrote %s", path)
			return nil
		},
	}
}

func newConfigEditCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "edit",
		Short: "Open the config file in $EDITOR",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vi"
			}
			path := configPath()
			if _, err := os.Stat(path); os.IsNotExist(err) {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(path, []byte(configTemplate), 0o644); err != nil {
					return err
				}
			}
			c := exec.CommandContext(cmd.Context(), editor, path)
			c.Stdin, c.Stdout, c.Stderr = os.Stdin, cmdOut, cmdErr
			return c.Run()
		},
	}
}
