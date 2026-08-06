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
		Short: "YouTube Music (artists, albums, tracks)",
		Long: `Search and browse YouTube Music.

music.youtube.com is a second app over the same catalogue, on its own InnerTube
key as WEB_REMIX, and it answers differently: it counts plays where the main site
counts views, it credits every artist on a release rather than the uploader, and
it knows about albums, which the main site does not.

A track id is a video id. One song can have two of them, an art track and an
official video, and the two carry different numbers, which is why ytb keeps them
as two records joined by a seen_as claim rather than merging them.`,
		Sub: []kit.Command{
			newMusicSearchCmd(),
			newMusicArtistCmd(),
			newMusicAlbumCmd(),
			newMusicPlaylistCmd(),
			newMusicTrackCmd(),
		},
	}
}

func newMusicSearchCmd() kit.Command {
	var typ string
	return kit.Command{
		Use:   "search <query>",
		Short: "Search tracks, albums, artists and playlists",
		Long: `Search YouTube Music.

Without --type this searches everything and every row is classified by its own
endpoint, so tracks, albums, artists and playlists come back interleaved the way
the site returns them.`,
		Args: kit.MinimumNArgs(1),
		Flags: func(f *kit.FlagSet) {
			f.StringVar(&typ, "type", "", "song|video|album|artist|playlist|podcast|episode (default: everything)")
		},
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)
			query := strings.Join(args, " ")
			var n int
			err := app.Client.MusicSearch(ctx, query, typ, app.PageOptions(false), func(v any) error {
				n++
				return app.Out.Emit(musicResultRow(v))
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
		Short: "Artist profile with discography and top tracks",
		Long: `Read an artist page.

The table prints the artist, then their top tracks, then every lockup the page
carries: albums, singles and EPs, videos, playlists and related artists, each
labelled with the shelf it came from. Use -o json for the record with the lists
nested.`,
		Args: kit.ExactArgs(1),
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)
			artist, err := app.Client.FetchArtist(ctx, args[0])
			if err != nil {
				return err
			}
			if artist == nil {
				return noResults("artist not found")
			}
			if err := app.Out.Emit(musicShelfRow(*artist, "")); err != nil {
				return err
			}
			for _, t := range artist.TopTracks {
				if err := app.Out.Emit(musicShelfRow(t, "Songs")); err != nil {
					return err
				}
			}
			for _, list := range [][]youtube.MusicItem{
				artist.Albums, artist.Singles, artist.Videos, artist.Playlists, artist.RelatedArtists,
			} {
				for _, item := range list {
					if err := app.Out.Emit(musicShelfRow(item, "")); err != nil {
						return err
					}
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
			album, tracks, err := app.Client.FetchAlbum(ctx, args[0])
			if err != nil {
				return err
			}
			if album == nil {
				return noResults("album not found")
			}
			if err := app.Out.Emit(musicResultRow(*album)); err != nil {
				return err
			}
			for _, t := range tracks {
				if err := app.Out.Emit(musicResultRow(t)); err != nil {
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
			header, tracks, err := app.Client.FetchMusicPlaylist(ctx, args[0])
			if err != nil {
				return err
			}
			if header != nil {
				if err := app.Out.Emit(musicResultRow(*header)); err != nil {
					return err
				}
			}
			if len(tracks) == 0 {
				return noResults("empty playlist")
			}
			for _, t := range tracks {
				if err := app.Out.Emit(musicResultRow(t)); err != nil {
					return err
				}
			}
			return app.Out.Flush()
		},
	}
}

func newMusicTrackCmd() kit.Command {
	var lyrics bool
	return kit.Command{
		Use:   "track <video-id>",
		Short: "Track detail (with --lyrics if available)",
		Long: `Read one track through YouTube Music.

The id is a video id. Where the song has both an art track and an official
video, the music app answers about whichever of the two it files the song under,
so the id on the record is not always the id that was asked for.`,
		Args: kit.ExactArgs(1),
		Flags: func(f *kit.FlagSet) {
			f.BoolVar(&lyrics, "lyrics", false, "fetch lyrics if available (1 more request)")
		},
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)
			track, err := app.Client.FetchTrack(ctx, args[0], lyrics)
			if err != nil {
				return err
			}
			if track == nil {
				return noResults("track not found")
			}
			if err := app.Out.Emit(trackRow(*track)); err != nil { // one record, so the detailed shape
				return err
			}
			if err := app.Out.Flush(); err != nil {
				return err
			}
			if lyrics && track.Lyrics != "" {
				return app.Line(track.Lyrics)
			}
			return nil
		},
	}
}
