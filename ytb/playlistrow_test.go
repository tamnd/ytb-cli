package ytb

import (
	"testing"

	"github.com/tamnd/any-cli/kit/errs"
)

// A playlist row arrives as one of three renderers and the reader has to know all
// three, because which one a page serves is not something the caller picks.
//
// lockupViewModel is the modern shape and comes off the playlists and podcasts
// tabs. gridPlaylistRenderer comes off a shelf. playlistRenderer comes off the
// releases and courses tabs, and it was the one nobody read, so those two tabs
// answered with rows on the page and nothing in the output.
//
// The fixtures are trimmed copies of live responses: the lockup from
// @TED/podcasts, the grid from a shelf, and the renderer from
// @RickAstleyYT/releases. Trimmed, not invented, because a shape this test agrees
// with and YouTube does not is worse than no test at all.
func TestEveryPlaylistRowShapeIsRead(t *testing.T) {
	tree := map[string]any{"contents": []any{
		map[string]any{"lockupViewModel": map[string]any{
			"contentType": "LOCKUP_CONTENT_TYPE_PODCAST",
			"contentId":   "PLOGi5-fAu8bGttER00FXjs5mZTh4Rz1DZ",
			"metadata": map[string]any{"lockupMetadataViewModel": map[string]any{
				"title": map[string]any{"content": "How to Be a Better Human"},
			}},
		}},
		map[string]any{"gridPlaylistRenderer": map[string]any{
			"playlistId":     "PLlaN88a7y2_qHDbY9eQbuNTAuEJUSEeuu",
			"title":          map[string]any{"runs": []any{map[string]any{"text": "Rick Astley Live at The O2"}}},
			"videoCountText": map[string]any{"runs": []any{map[string]any{"text": "3 videos"}}},
		}},
		map[string]any{"playlistRenderer": map[string]any{
			"playlistId":     "OLAK5uy_lg82mxxXqK5ya1hjeBMyKlESfjo5qH93I",
			"title":          map[string]any{"simpleText": "50 (10th Anniversary Deluxe Edition)"},
			"videoCount":     "15",
			"videoCountText": map[string]any{"runs": []any{map[string]any{"text": "15"}, map[string]any{"text": " videos"}}},
			"longBylineText": map[string]any{"runs": []any{
				map[string]any{"text": "Rick Astley", "navigationEndpoint": map[string]any{
					"browseEndpoint": map[string]any{"browseId": "UCuAXFkgsw1L7xaCfnd5JJOw"},
				}},
				map[string]any{"text": " · "},
				map[string]any{"text": "Jul 6, 2026"},
			}},
			"thumbnailRenderer": map[string]any{"playlistCustomThumbnailRenderer": map[string]any{
				"thumbnail": map[string]any{"thumbnails": []any{
					map[string]any{"url": "https://i9.ytimg.com/s_p/OLAK5uy_l/maxresdefault.jpg", "width": 1200.0, "height": 1200.0},
				}},
			}},
			// The row's navigationEndpoint plays the first track. It is here so the
			// assertion below is about a url the fixture really offers.
			"navigationEndpoint": map[string]any{"commandMetadata": map[string]any{"webCommandMetadata": map[string]any{
				"url": "/watch?v=AC3Ejf7vPEY&list=OLAK5uy_lg82mxxXqK5ya1hjeBMyKlESfjo5qH93I",
			}}},
		}},
	}}

	got := parsePlaylistsFromTree(tree)
	if len(got) != 3 {
		t.Fatalf("read %d rows off a tree holding all three shapes, want 3: %+v", len(got), got)
	}
	byID := map[string]Playlist{}
	for _, p := range got {
		byID[p.PlaylistID] = p
	}

	podcast, ok := byID["PLOGi5-fAu8bGttER00FXjs5mZTh4Rz1DZ"]
	if !ok {
		t.Fatal("the podcast lockup was dropped, so the podcasts tab reads empty on a channel that has one")
	}
	if podcast.Title != "How to Be a Better Human" {
		t.Errorf("podcast title = %q", podcast.Title)
	}

	grid, ok := byID["PLlaN88a7y2_qHDbY9eQbuNTAuEJUSEeuu"]
	if !ok {
		t.Fatal("the grid row was dropped")
	}
	if grid.VideoCount != 3 {
		t.Errorf("grid video count = %d, want 3", grid.VideoCount)
	}

	album, ok := byID["OLAK5uy_lg82mxxXqK5ya1hjeBMyKlESfjo5qH93I"]
	if !ok {
		t.Fatal("the playlistRenderer row was dropped, which is what made releases and courses come back empty")
	}
	if album.Title != "50 (10th Anniversary Deluxe Edition)" {
		t.Errorf("album title = %q", album.Title)
	}
	// videoCount, not videoCountText. The text is translated and the field is not.
	if album.VideoCount != 15 {
		t.Errorf("album video count = %d, want 15", album.VideoCount)
	}
	if album.ChannelID != "UCuAXFkgsw1L7xaCfnd5JJOw" {
		t.Errorf("album channel id = %q", album.ChannelID)
	}
	// The byline is "Rick Astley · Jul 6, 2026" and only the linked run is the owner.
	if album.ChannelTitle != "Rick Astley" {
		t.Errorf("album channel title = %q, want %q: the release date leaked into the owner", album.ChannelTitle, "Rick Astley")
	}
	if len(album.MetadataParts) != 1 || album.MetadataParts[0] != "Jul 6, 2026" {
		t.Errorf("album metadata parts = %v, want the release date kept", album.MetadataParts)
	}
	// The row's own cover, not a frame from the first track.
	if len(album.Thumbnails) != 1 || album.Thumbnails[0].Width != 1200 {
		t.Errorf("album thumbnails = %+v, want the 1200px cover off thumbnailRenderer", album.Thumbnails)
	}
	// navigationEndpoint on this shape is a watch url. Taking it would give every
	// album a url that plays something instead of a url that is the album.
	if want := NormalizePlaylistURL(album.PlaylistID); album.URL != want {
		t.Errorf("album url = %q, want the playlist url %q", album.URL, want)
	}
}

// A podcast is a playlist of episodes and an album is a playlist of tracks. The
// search reader listed all three content types and then handed them to a parser
// that checked for PLAYLIST alone, so searching for a podcast returned nothing.
func TestAPodcastAndAnAlbumAreReadAsPlaylists(t *testing.T) {
	for _, ct := range []string{
		"LOCKUP_CONTENT_TYPE_PLAYLIST",
		"LOCKUP_CONTENT_TYPE_PODCAST",
		"LOCKUP_CONTENT_TYPE_ALBUM",
	} {
		p := parseLockupPlaylist(map[string]any{
			"contentType": ct,
			"contentId":   "PL0123456789",
			"metadata": map[string]any{"lockupMetadataViewModel": map[string]any{
				"title": map[string]any{"content": "Something"},
			}},
		})
		if p.PlaylistID == "" {
			t.Errorf("%s was dropped, so it is unreachable from a search and from a tab", ct)
		}
	}
	if p := parseLockupPlaylist(map[string]any{
		"contentType": "LOCKUP_CONTENT_TYPE_VIDEO",
		"contentId":   "dQw4w9WgXcQ",
	}); p.PlaylistID != "" {
		t.Error("a video lockup was read as a playlist")
	}
}

// A tab this tool does not read is a usage error and not an empty answer, and
// asking for a tab the channel does not have has to say so rather than quietly
// returning the home page, which is what YouTube serves in that case.
func TestAskingForATabThatIsNotAPlaylistGridIsAUsageError(t *testing.T) {
	c := NewClient(DefaultConfig())
	err := c.StreamChannelPlaylistTab(t.Context(), "@TED", "shorts", PageOptions{}, func(Playlist) error {
		t.Fatal("a shorts read reached the network and emitted a playlist")
		return nil
	})
	if err == nil {
		t.Fatal("asking for the shorts tab through the playlist reader was accepted")
	}
	if code := errs.ExitCode(err); code != 2 {
		t.Errorf("exit code = %d, want 2 (usage): %v", code, err)
	}
}
