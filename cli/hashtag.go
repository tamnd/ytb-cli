package cli

import (
	"github.com/spf13/cobra"
	"github.com/tamnd/ytb-cli/youtube"
)

func newHashtagCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hashtag <tag>",
		Short: "A hashtag feed",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			store, err := app.Store()
			if err != nil {
				return err
			}
			var n int
			err = app.Client.StreamHashtag(ctx, args[0], app.PageOptions(false), func(v youtube.Video) error {
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
				return noResults("no videos for that hashtag")
			}
			return app.Out.Flush()
		},
	}
	return cmd
}
