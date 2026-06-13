package cli

import (
	"github.com/spf13/cobra"
	"github.com/tamnd/ytb-cli/youtube"
)

func newChannelCmd(app *App) *cobra.Command {
	var (
		videos    bool
		shorts    bool
		streams   bool
		playlists bool
		enrich    bool
		all       bool
	)
	cmd := &cobra.Command{
		Use:   "channel <id|@handle|url>",
		Short: "Channel metadata and its content",
		Long: `Without a tab flag, print the channel record. With --videos/--shorts/--streams
stream the tab's uploads via /browse continuation. --playlists lists the
channel's playlists. --enrich fans out /player calls to fill view counts and
descriptions the grid omits.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			input := args[0]
			store, err := app.Store()
			if err != nil {
				return err
			}
			opt := app.PageOptions(enrich)
			if all {
				opt.Max = 0
				opt.MaxPages = 0
			}

			// Always resolve the channel header first.
			ch, err := app.Client.FetchChannel(ctx, input)
			if err != nil {
				return err
			}
			if ch == nil {
				return noResults("channel not found")
			}
			if store != nil {
				_ = store.UpsertChannel(*ch)
			}

			tab := selectedTab(videos, shorts, streams)
			switch {
			case playlists:
				err = app.Client.StreamChannelPlaylists(ctx, input, opt, func(p youtube.Playlist) error {
					if store != nil {
						_ = store.UpsertPlaylist(p)
					}
					return app.Out.Emit(playlistRow(p))
				})
			case tab != "":
				err = app.Client.StreamChannelTab(ctx, input, tab, opt, func(v youtube.Video) error {
					if store != nil {
						_ = store.UpsertVideo(v)
					}
					return app.Out.Emit(videoRow(v))
				})
			default:
				err = app.Out.Emit(channelRow(*ch))
			}
			if err != nil && err != youtube.ErrStop {
				return err
			}
			return app.Out.Flush()
		},
	}
	f := cmd.Flags()
	f.BoolVar(&videos, "videos", false, "stream the uploads tab")
	f.BoolVar(&shorts, "shorts", false, "stream the Shorts tab")
	f.BoolVar(&streams, "streams", false, "stream the past live streams tab")
	f.BoolVar(&playlists, "playlists", false, "list the channel's playlists")
	f.BoolVar(&enrich, "enrich", false, "call /player per video for full metadata")
	f.BoolVar(&all, "all", false, "remove the default page cap")
	return cmd
}

func selectedTab(videos, shorts, streams bool) string {
	switch {
	case shorts:
		return "shorts"
	case streams:
		return "streams"
	case videos:
		return "videos"
	default:
		return ""
	}
}
