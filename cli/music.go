package cli

import (
	"context"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/ytb-cli/youtube"
)

func newMusicCmd() kit.Command {
	return kit.Command{
		Use:   "music",
		Short: "YouTube Music (artists, albums, songs)",
		Long:  `Search and browse YouTube Music via the WEB_REMIX client context.`,
		Sub: []kit.Command{
			newMusicSearchCmd(),
			newMusicArtistCmd(),
			newMusicAlbumCmd(),
			newMusicPlaylistCmd(),
			newMusicSongCmd(),
		},
	}
}

func newMusicSearchCmd() kit.Command {
	var typ string
	return kit.Command{
		Use:   "search <query>",
		Short: "Search artists, albums and songs",
		Args:  kit.MinimumNArgs(1),
		Flags: func(f *kit.FlagSet) {
			f.StringVar(&typ, "type", "", "song|video|album|artist|playlist (default song)")
		},
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)
			query := strings.Join(args, " ")
			// The unfiltered "all" results page returns items under
			// itemSectionRenderer with no shelf titles to classify by, so it
			// yields nothing useful. Default to songs, the canonical result.
			if typ == "" {
				typ = "song"
			}
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
}

func newMusicArtistCmd() kit.Command {
	return kit.Command{
		Use:   "artist <browseId|url>",
		Short: "Artist profile with albums and top songs",
		Args:  kit.ExactArgs(1),
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)
			artist, albums, songs, err := app.Client.FetchArtist(ctx, args[0])
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

func newMusicAlbumCmd() kit.Command {
	return kit.Command{
		Use:   "album <browseId|url>",
		Short: "Album header and track list",
		Args:  kit.ExactArgs(1),
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)
			album, songs, err := app.Client.FetchAlbum(ctx, args[0])
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

func newMusicPlaylistCmd() kit.Command {
	return kit.Command{
		Use:   "playlist <id|url>",
		Short: "Music playlist and tracks",
		Args:  kit.ExactArgs(1),
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)
			header, songs, err := app.Client.FetchMusicPlaylist(ctx, args[0])
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

func newMusicSongCmd() kit.Command {
	var lyrics bool
	return kit.Command{
		Use:   "song <video-id>",
		Short: "Song detail (with --lyrics if available)",
		Args:  kit.ExactArgs(1),
		Flags: func(f *kit.FlagSet) {
			f.BoolVar(&lyrics, "lyrics", false, "fetch lyrics if available")
		},
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)
			song, err := app.Client.FetchSong(ctx, args[0], lyrics)
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
				return app.Line(song.Lyrics)
			}
			return nil
		},
	}
}
