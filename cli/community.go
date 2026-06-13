package cli

import (
	"github.com/spf13/cobra"
	"github.com/tamnd/ytb-cli/youtube"
)

func newCommunityCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "community <channel-id|@handle>",
		Short: "Community / posts tab",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			store, err := app.Store()
			if err != nil {
				return err
			}
			var n int
			err = app.Client.StreamCommunity(ctx, args[0], app.PageOptions(false), func(p youtube.CommunityPost) error {
				n++
				if store != nil {
					_ = store.UpsertCommunityPost(p)
				}
				return app.Out.Emit(communityRow(p))
			})
			if err != nil && err != youtube.ErrStop {
				return err
			}
			if n == 0 {
				return noResults("no community posts")
			}
			return app.Out.Flush()
		},
	}
	return cmd
}
