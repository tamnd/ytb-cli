package cli

import (
	"github.com/spf13/cobra"
	"github.com/tamnd/ytb-cli/youtube"
)

func newTrendingCmd(app *App) *cobra.Command {
	var category string
	cmd := &cobra.Command{
		Use:   "trending",
		Short: "What's hot right now",
		Long:  `Trending videos. --category selects music|gaming|movies|news.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			store, err := app.Store()
			if err != nil {
				return err
			}
			var n int
			err = app.Client.Trending(ctx, category, app.PageOptions(false), func(v youtube.Video) error {
				n++
				if store != nil {
					_ = store.UpsertVideo(v)
				}
				return app.Out.Emit(videoRow(v))
			})
			if err != nil && err != youtube.ErrStop {
				return err
			}
			if n == 0 {
				return noResults("no trending videos")
			}
			return app.Out.Flush()
		},
	}
	cmd.Flags().StringVar(&category, "category", "", "music|gaming|movies|news")
	return cmd
}
