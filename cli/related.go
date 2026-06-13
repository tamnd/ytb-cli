package cli

import (
	"github.com/spf13/cobra"
	"github.com/tamnd/ytb-cli/youtube"
)

func newRelatedCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "related <video-id|url>",
		Short: "The related-videos graph",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			store, err := app.Store()
			if err != nil {
				return err
			}
			res, err := app.Client.FetchVideo(ctx, args[0], youtube.VideoOptions{Player: false, Next: true})
			if err != nil {
				return err
			}
			if res == nil || len(res.Related) == 0 {
				return noResults("no related videos")
			}
			limit := app.Limit
			for i, r := range res.Related {
				if limit > 0 && i >= limit {
					break
				}
				if store != nil {
					_ = store.UpsertRelatedVideo(r)
				}
				if err := app.Out.Emit(relatedRow(r)); err != nil {
					return err
				}
			}
			return app.Out.Flush()
		},
	}
	return cmd
}
