package cli

import (
	"github.com/spf13/cobra"
	"github.com/tamnd/ytb-cli/youtube"
)

func newExportCmd(app *App) *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "export [channel-id|@handle]",
		Short: "Render the store as interlinked Markdown (needs --db)",
		Long: `Render the stored data as an interlinked Markdown site under --out: per-video
pages with YAML frontmatter, chapter lists, transcripts, related sidebars, and
channel/playlist index pages. With no argument, every channel in the store is
exported.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			store, err := app.RequireStore()
			if err != nil {
				return err
			}
			if out == "" {
				return usageErr("--out <dir> is required")
			}
			var channel string
			if len(args) == 1 {
				channel = args[0]
			}
			if app.dryRun {
				app.logf("would export %q to %s", orAll(channel), out)
				return nil
			}
			if err := youtube.Export(store, channel, out); err != nil {
				return err
			}
			app.logf("exported to %s", out)
			return nil
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "output directory for the Markdown site")
	return cmd
}

func orAll(s string) string {
	if s == "" {
		return "all channels"
	}
	return s
}
