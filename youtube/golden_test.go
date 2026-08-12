package youtube

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// golden_test.go writes down what every parser returns, one file per fixture, and
// fails when the answer changes.
//
// The other tests assert the fields somebody thought to assert. A golden asserts
// all of them, including the ones nobody would think to check, which is where a
// refactor quietly drops a field or starts filling one with the wrong thing. The
// review happens in the diff: run with -update, read what moved, and either the
// change is the point of the commit or it is a bug that would otherwise ship.
//
//	go test ./youtube -run Golden -update
//
// Two rules keep these stable. The parsers are called directly rather than
// through the client, so nothing here goes near the network, and fetched_at is
// blanked on the way out, so nothing here comes off the clock. Doc 06 section 5.
var updateGolden = flag.Bool("update", false, "rewrite the golden files from what the parsers return now")

// goldenCases is one entry per fixture that parses to a record. Each returns
// whatever the parse produced, and the whole of it: a case that returns a field
// rather than the record is a case that stops noticing the other forty.
var goldenCases = map[string]func(t *testing.T) any{
	"about_channel": func(t *testing.T) any {
		return ParseChannelAbout(loadFixture(t, "about_channel.json"))
	},
	"channel_feed": func(t *testing.T) any {
		videos, err := ParseChannelFeed(readFixtureBytes(t, "channel_feed.xml"),
			"https://www.youtube.com/feeds/videos.xml?channel_id=UCuAXFkgsw1L7xaCfnd5JJOw")
		if err != nil {
			t.Fatalf("parse feed: %v", err)
		}
		return videos
	},
	"channel_ldjson": func(t *testing.T) any {
		return channelProfilePage(nil, string(readFixtureBytes(t, "channel_ldjson.html")))
	},
	"channel_page_artist": func(t *testing.T) any {
		return ParseChannelRecord(loadChannelPage(t, "channel_page_artist.json"), "https://www.youtube.com/@RickAstleyYT")
	},
	"channel_page_verified": func(t *testing.T) any {
		return ParseChannelRecord(loadChannelPage(t, "channel_page_verified.json"), "https://www.youtube.com/@BBCNews")
	},
	"channel_page_vi": func(t *testing.T) any {
		return ParseChannelRecord(loadChannelPage(t, "channel_page_vi.json"), "https://www.youtube.com/@BBCNews")
	},
	"channel_tabs": func(t *testing.T) any {
		return ChannelTabs(loadFixture(t, "channel_tabs.json"))
	},
	"comments_restricted": func(t *testing.T) any {
		section := loadFixture(t, "comments_restricted.json")
		return newRefusal("comments for dQw4w9WgXcQ", "watch page", commentsRefusalMessage(section))
	},
	"mix_unviewable": func(t *testing.T) any {
		return alertRefusal(loadFixture(t, "mix_unviewable.json"), "RDdQw4w9WgXcQ", "browse")
	},
	"music_album": func(t *testing.T) any {
		alb, tracks := parseAlbumPage(loadFixture(t, "music_album.json"), "MPREb_dcYZhAh5urI")
		return map[string]any{"album": alb, "tracks": tracks}
	},
	"music_artist": func(t *testing.T) any {
		data := loadFixture(t, "music_artist.json")
		a := &Artist{}
		walkJSON(data, func(m map[string]any) {
			if h, ok := m["musicImmersiveHeaderRenderer"].(map[string]any); ok {
				musicFillArtistHeader(a, h)
			}
		})
		musicFillArtistShelves(a, data, "golden")
		return a
	},
	"music_lyrics": func(t *testing.T) any {
		text, source := musicParseLyrics(loadFixture(t, "music_lyrics.json"))
		return map[string]any{"text": text, "source": source}
	},
	"music_lyrics_none": func(t *testing.T) any {
		data := loadFixture(t, "music_lyrics_none.json")
		text, source := musicParseLyrics(data)
		return map[string]any{"text": text, "source": source, "message": musicMessage(data)}
	},
	"music_search_card": func(t *testing.T) any {
		return musicRows(t, "music_search_card.json")
	},
	"music_search_rows": func(t *testing.T) any {
		return musicRows(t, "music_search_rows.json")
	},
	"player_web_nourl": func(t *testing.T) any {
		formats := ParseVideoFormats(loadFixture(t, "player_web_nourl.json"), "dQw4w9WgXcQ")
		// ParseVideoFormats deduplicates through a map, so the order it returns is
		// the map's. Every caller sorts before printing and so does this.
		sortFormats(formats)
		annotateFormats(formats)
		return formats
	},
	"playlist_header_album": func(t *testing.T) any {
		return ParsePlaylistRecord(loadFixture(t, "playlist_header_album.json"), "OLAK5uy_lGQfnMNGvYCRdDq9ZLzJV2BJL2aHQsz9Y", SurfaceInnerTube)
	},
	"playlist_header_uploads": func(t *testing.T) any {
		return ParsePlaylistRecord(loadFixture(t, "playlist_header_uploads.json"), "UUuAXFkgsw1L7xaCfnd5JJOw", SurfaceInnerTube)
	},
	"playlist_header_viewmodel": func(t *testing.T) any {
		return ParsePlaylistRecord(loadFixture(t, "playlist_header_viewmodel.json"), "PLlaN88a7y2_rosKX2WQt2VjFbjyDQXOkR", SurfaceInnerTube)
	},
	"playlist_items": func(t *testing.T) any {
		items, token := ParsePlaylistItems(loadFixture(t, "playlist_items.json"), 20)
		return map[string]any{"items": items, "has_continuation": token != ""}
	},
	"playlist_shorts_uush": func(t *testing.T) any {
		items, token := ParsePlaylistItems(loadFixture(t, "playlist_shorts_uush.json"), 0)
		return map[string]any{"items": items, "has_continuation": token != ""}
	},
	"posts_refusal": func(t *testing.T) any {
		return messageRefusal(loadFixture(t, "posts_refusal.json"), "posts for @Computerphile", "browse")
	},
	"search_plain": func(t *testing.T) any {
		items, token := ParseSearchResults(loadFixture(t, "search_plain.json"))
		return map[string]any{"items": items, "has_continuation": token != ""}
	},
	"search_playlists": func(t *testing.T) any {
		items, token := ParseSearchResults(loadFixture(t, "search_playlists.json"))
		return map[string]any{"items": items, "has_continuation": token != ""}
	},
	"video_captions_android": func(t *testing.T) any {
		return ParseCaptionTracks(loadFixture(t, "video_captions_android.json"), "dQw4w9WgXcQ")
	},
	"video_microdata": func(t *testing.T) any {
		return ParseVideoMicrodata(string(readFixtureBytes(t, "video_microdata.html")))
	},
	"video_player": func(t *testing.T) any {
		v := NewVideo("dQw4w9WgXcQ", SurfaceWatchHTML)
		ParsePlayerResponse(loadFixture(t, "video_player.json"), v, SurfaceWatchHTML)
		return v
	},
	"video_player_short": func(t *testing.T) any {
		v := NewVideo("Df5Y-2ndQyU", SurfaceWatchHTML)
		ParsePlayerResponse(loadFixture(t, "video_player_short.json"), v, SurfaceWatchHTML)
		return v
	},
	"video_player_stream": func(t *testing.T) any {
		v := NewVideo("201MyYKtXaQ", SurfaceWatchHTML)
		ParsePlayerResponse(loadFixture(t, "video_player_stream.json"), v, SurfaceWatchHTML)
		return v
	},
}

func TestGolden(t *testing.T) {
	for name, parse := range goldenCases {
		t.Run(name, func(t *testing.T) {
			got := marshalGolden(t, parse(t))
			path := filepath.Join("testdata", "golden", name+".json")
			if *updateGolden {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden: %v\nRun go test ./youtube -run Golden -update and read the diff.", err)
			}
			if string(got) != string(want) {
				t.Errorf("%s parses differently than its golden.\n%s\nRun go test ./youtube -run Golden -update and read the diff.",
					name, firstDifference(string(want), string(got)))
			}
		})
	}
}

// TestGoldenCoversEveryFixture is the same argument as the capture manifest. A
// fixture with no golden is a fixture whose parse nobody is watching, and it is
// easier to add the case now than to notice its absence in a year.
func TestGoldenCoversEveryFixture(t *testing.T) {
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "capture.txt" {
			continue
		}
		name := strings.TrimSuffix(strings.TrimSuffix(entry.Name(), ".json"), ".html")
		name = strings.TrimSuffix(name, ".xml")
		if _, ok := goldenCases[name]; !ok {
			t.Errorf("%s has no golden case, so nothing watches what it parses to", entry.Name())
		}
	}
}

// marshalGolden serialises a record the way a golden keeps it: indented so a diff
// is readable, and with the clock taken out.
func marshalGolden(t *testing.T, rec any) []byte {
	t.Helper()
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	blankTimestamps(tree)
	out, err := json.MarshalIndent(tree, "", "  ")
	if err != nil {
		t.Fatalf("marshal indented: %v", err)
	}
	return append(out, '\n')
}

// blankTimestamps empties every fetched_at in a record.
//
// It is the one field in an envelope that comes from the clock rather than from
// the response, and leaving it in would make every golden fail on every run,
// which is the same as having no goldens at all. Everything else in a golden is
// either in the fixture or derived from it.
func blankTimestamps(node any) {
	switch v := node.(type) {
	case map[string]any:
		for key, child := range v {
			if key == "fetched_at" {
				v[key] = ""
				continue
			}
			blankTimestamps(child)
		}
	case []any:
		for _, child := range v {
			blankTimestamps(child)
		}
	}
}

// firstDifference points at the line that moved rather than printing two records
// and leaving the reader to find it.
func firstDifference(want, got string) string {
	wantLines, gotLines := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(wantLines) && i < len(gotLines); i++ {
		if wantLines[i] != gotLines[i] {
			return "line " + itoa(i+1) + ":\n  golden: " + strings.TrimSpace(wantLines[i]) + "\n  parsed: " + strings.TrimSpace(gotLines[i])
		}
	}
	return "the goldens agree line for line up to line " + itoa(min(len(wantLines), len(gotLines))) +
		", and then one of them ends: golden has " + itoa(len(wantLines)) + " lines, the parse has " + itoa(len(gotLines))
}

func itoa(n int) string { return strconv.Itoa(n) }

func musicRows(t *testing.T, fixture string) []any {
	t.Helper()
	var got []any
	seen := map[string]bool{}
	if _, err := musicEmitRows(loadFixture(t, fixture), "golden", seen, func(v any) error {
		got = append(got, v)
		return nil
	}, 0); err != nil {
		t.Fatalf("emit: %v", err)
	}
	return got
}
