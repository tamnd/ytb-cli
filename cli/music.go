package cli

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/tamnd/ytb-cli/youtube"
)

func newMusicCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "music",
		Short: "YouTube Music (artists, albums, songs)",
		Long:  `Search and browse YouTube Music via the WEB_REMIX client context.`,
	}
	cmd.AddCommand(
		newMusicSearchCmd(app),
		newMusicArtistCmd(app),
		newMusicAlbumCmd(app),
		newMusicPlaylistCmd(app),
		newMusicSongCmd(app),
	)
	return cmd
}

func newMusicSearchCmd(app *App) *cobra.Command {
	var typ string
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search artists, albums and songs",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			query := strings.Join(args, " ")
			var n int
			err := app.Client.MusicSearch(ctx, query, typ, app.PageOptions(false), func(v any) error {
				n++
				return app.Out.Emit(anyRow(v))
			})
			if err != nil && err != youtube.ErrStop {
				return err
			}
			if n == 0 {
				return noResults("no results")
			}
			return app.Out.Flush()
		},
	}
	cmd.Flags().StringVar(&typ, "type", "", "song|album|artist|playlist")
	return cmd
}

func newMusicArtistCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "artist <browseId|url>",
		Short: "Artist profile with albums and top songs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			artist, albums, songs, err := app.Client.FetchArtist(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if artist == nil {
				return noResults("artist not found")
			}
			if err := app.Out.Emit(artistRow(*artist)); err != nil {
				return err
			}
			for _, a := range albums {
				if err := app.Out.Emit(albumRow(a)); err != nil {
					return err
				}
			}
			for _, s := range songs {
				if err := app.Out.Emit(songRow(s)); err != nil {
					return err
				}
			}
			return app.Out.Flush()
		},
	}
}

func newMusicAlbumCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "album <browseId|url>",
		Short: "Album header and track list",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			album, songs, err := app.Client.FetchAlbum(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if album == nil {
				return noResults("album not found")
			}
			if err := app.Out.Emit(albumRow(*album)); err != nil {
				return err
			}
			for _, s := range songs {
				if err := app.Out.Emit(songRow(s)); err != nil {
					return err
				}
			}
			return app.Out.Flush()
		},
	}
}

func newMusicPlaylistCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "playlist <id|url>",
		Short: "Music playlist and tracks",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			header, songs, err := app.Client.FetchMusicPlaylist(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if header != nil {
				if err := app.Out.Emit(albumRow(*header)); err != nil {
					return err
				}
			}
			if len(songs) == 0 {
				return noResults("empty playlist")
			}
			for _, s := range songs {
				if err := app.Out.Emit(songRow(s)); err != nil {
					return err
				}
			}
			return app.Out.Flush()
		},
	}
}

func newMusicSongCmd(app *App) *cobra.Command {
	var lyrics bool
	cmd := &cobra.Command{
		Use:   "song <video-id>",
		Short: "Song detail (with --lyrics if available)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			song, err := app.Client.FetchSong(cmd.Context(), args[0], lyrics)
			if err != nil {
				return err
			}
			if song == nil {
				return noResults("song not found")
			}
			if err := app.Out.Emit(songRow(*song)); err != nil {
				return err
			}
			if err := app.Out.Flush(); err != nil {
				return err
			}
			if lyrics && song.Lyrics != "" {
				return app.Out.Line(song.Lyrics)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&lyrics, "lyrics", false, "fetch lyrics if available")
	return cmd
}
