package cli

import (
	"github.com/spf13/cobra"
	"github.com/tamnd/ytb-cli/youtube"
)

func newPlaylistCmd(app *App) *cobra.Command {
	var videos bool
	cmd := &cobra.Command{
		Use:   "playlist <id|url>",
		Short: "Playlist header and its items",
		Long: `Print the playlist header. With --videos stream the items (with positions)
enriched with the basic video fields, paging via continuation to -n/--max-pages.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			input := args[0]
			store, err := app.Store()
			if err != nil {
				return err
			}
			pl, err := app.Client.FetchPlaylist(ctx, input)
			if err != nil {
				return err
			}
			if pl == nil {
				return noResults("playlist not found")
			}
			if store != nil {
				_ = store.UpsertPlaylist(*pl)
			}
			if !videos {
				if err := app.Out.Emit(playlistRow(*pl)); err != nil {
					return err
				}
				return app.Out.Flush()
			}
			err = app.Client.StreamPlaylistItems(ctx, input, app.PageOptions(false), func(pv youtube.PlaylistVideo, v youtube.Video) error {
				if store != nil {
					_ = store.UpsertPlaylistVideo(pv)
					if v.VideoID != "" {
						_ = store.UpsertVideo(v)
					}
				}
				return app.Out.Emit(playlistVideoRow(pv, v))
			})
			if err != nil && err != youtube.ErrStop {
				return err
			}
			return app.Out.Flush()
		},
	}
	cmd.Flags().BoolVar(&videos, "videos", false, "stream the playlist items")
	return cmd
}
