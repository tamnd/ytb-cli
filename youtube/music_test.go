package youtube

import (
	"strings"
	"testing"
)

// The fixtures are real WEB_REMIX responses with the tracking noise and the
// service menu items taken out, and the long lists cut short. Everything these
// tests read is untouched, because a shape invented by hand would only prove
// that the parser agrees with whoever invented it.

func TestMusicTargetKind(t *testing.T) {
	cases := []struct {
		name string
		in   musicTarget
		kind string
		id   string
	}{
		{"a typed artist page", musicTarget{BrowseID: "UCwZEU0wAwIyZb4x5G_KJp2w", PageType: musicPageArtist}, musicKindArtist, "UCwZEU0wAwIyZb4x5G_KJp2w"},
		{"a person's own channel is still an artist to read", musicTarget{BrowseID: "UCXrZi6TaRpKJnnxltD1iKiw", PageType: musicPageUserChannel}, musicKindArtist, "UCXrZi6TaRpKJnnxltD1iKiw"},
		{"a typed album", musicTarget{BrowseID: "MPREb_dcYZhAh5urI", PageType: musicPageAlbum}, musicKindAlbum, "MPREb_dcYZhAh5urI"},
		{"a typed playlist keeps its id and drops the VL", musicTarget{BrowseID: "VLRDCLAK5uy_x", PageType: musicPagePlaylist}, musicKindPlaylist, "RDCLAK5uy_x"},
		{"a podcast show is the playlist behind it", musicTarget{BrowseID: "MPSPPLtQiqTbYDL79", PageType: musicPagePodcast}, musicKindPlaylist, "PLtQiqTbYDL79"},
		{"a watch endpoint is a track whatever else it says", musicTarget{VideoID: "dQw4w9WgXcQ", PlaylistID: "RDAMVMdQw4w9WgXcQ"}, musicKindTrack, "dQw4w9WgXcQ"},
		{"an untyped MPRE falls back to its prefix", musicTarget{BrowseID: "MPREb_ewByopgML4F"}, musicKindAlbum, "MPREb_ewByopgML4F"},
		{"an endpoint that says nothing names nothing", musicTarget{}, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.in.kind(); got != c.kind {
				t.Errorf("kind = %q, want %q", got, c.kind)
			}
			if got := c.in.id(); got != c.id {
				t.Errorf("id = %q, want %q", got, c.id)
			}
		})
	}
}

// TestMusicSearchRowsAreTypedRecords reads ten rows of one real search for Rick
// Astley, one of every shape the page returned: a song, a video, a community
// playlist, one of YouTube Music's own, an album, an official artist channel, a
// small artist, a person's profile, a podcast episode and a podcast show.
func TestMusicSearchRowsAreTypedRecords(t *testing.T) {
	var got []any
	seen := map[string]bool{}
	if _, err := musicEmitRows(loadFixture(t, "music_search_rows.json"), "test", seen, func(v any) error {
		got = append(got, v)
		return nil
	}, 0); err != nil {
		t.Fatalf("emit: %v", err)
	}
	want := []struct {
		kind string
		id   string
	}{
		{"track", "lYBUbBu4W08"},
		{"track", "dQw4w9WgXcQ"},
		{"playlist", "PLkqz3S84Tw-TRybU2-IwWVueLiPG7QoVd"},
		{"playlist", "RDCLAK5uy_lMzHW51iFg1Kx0d_2EHpzbOgCrwtu8cgI"},
		{"album", "MPREb_ewByopgML4F"},
		{"artist", "UCSyCovCbUAnejYPJhvaP7sg"},
		{"artist", "UCLDdWcoADpxYYssGlUWLT8w"},
		{"artist", "UCXrZi6TaRpKJnnxltD1iKiw"},
		{"track", "i5KARwGiBRs"},
		{"playlist", "PLtQiqTbYDL79aDD07TbjRq798JKkgV_nF"},
	}
	if len(got) != len(want) {
		t.Fatalf("emitted %d records, want %d", len(got), len(want))
	}
	for i, w := range want {
		key := musicRecordKey(got[i])
		if key != w.kind+"/"+w.id {
			t.Errorf("row %d is %s, want %s/%s", i, key, w.kind, w.id)
		}
	}

	// The song and the video of it are two records with two ids, and only the
	// typed video type separates them. The page says "Song" and "Video" in words
	// next to both and neither word is read.
	song, ok := got[0].(Track)
	if !ok {
		t.Fatalf("row 0 is %T, want a Track", got[0])
	}
	if song.MusicVideoType != "ATV" {
		t.Errorf("the song is typed %q, want ATV", song.MusicVideoType)
	}
	if song.DurationSeconds != 214 || song.DurationText != "3:34" {
		t.Errorf("the song runs %q (%ds), want 3:34 (214s)", song.DurationText, song.DurationSeconds)
	}
	if song.PlaysText != "2B plays" {
		t.Errorf("plays = %q", song.PlaysText)
	}
	video, ok := got[1].(Track)
	if !ok {
		t.Fatalf("row 1 is %T, want a Track", got[1])
	}
	if video.MusicVideoType != "OMV" {
		t.Errorf("the video is typed %q, want OMV", video.MusicVideoType)
	}

	// Two playlists render two different unlabelled counts, "121 views" for the
	// community one and "116 songs" for YouTube Music's own. Neither is claimed
	// as an item count, because only the noun tells them apart.
	for i, want := range map[int]string{2: "121 views", 3: "116 songs"} {
		p, ok := got[i].(Playlist)
		if !ok {
			t.Fatalf("row %d is %T, want a Playlist", i, got[i])
		}
		if p.VideoCount != 0 {
			t.Errorf("row %d claims %d items off an unlabelled count", i, p.VideoCount)
		}
		if len(p.MetadataParts) != 1 || p.MetadataParts[0] != want {
			t.Errorf("row %d kept %v, want [%q]", i, p.MetadataParts, want)
		}
	}

	// The same holds for an artist: one row renders a monthly audience and the
	// next renders subscribers, and both mean a different field.
	for i, want := range map[int]string{5: "38.2M monthly audience", 6: "1.29K subscribers"} {
		a, ok := got[i].(Artist)
		if !ok {
			t.Fatalf("row %d is %T, want an Artist", i, got[i])
		}
		if a.SubscriberCount != 0 || a.SubscriberCountText != "" || a.MonthlyListenersText != "" {
			t.Errorf("row %d claimed a typed count off %q", i, want)
		}
		if len(a.MetadataParts) != 1 || a.MetadataParts[0] != want {
			t.Errorf("row %d kept %v, want [%q]", i, a.MetadataParts, want)
		}
	}

	// A person's profile renders a handle where an artist renders a count, and a
	// handle has digits in it, so it has to be read by its shape and not by the
	// fact that there is a number somewhere in the string.
	profile, ok := got[7].(Artist)
	if !ok {
		t.Fatalf("row 7 is %T, want an Artist", got[7])
	}
	if profile.Handle != "@rickhardt2237" {
		t.Errorf("handle = %q, want @rickhardt2237", profile.Handle)
	}
	if len(profile.MetadataParts) != 0 {
		t.Errorf("the handle also landed in %v", profile.MetadataParts)
	}

	// A podcast episode renders its publish date in the slot a song renders its
	// play count in. It has digits in it and it counts nothing, so it is kept as
	// what it is rather than filed as a number of plays.
	episode, ok := got[8].(Track)
	if !ok {
		t.Fatalf("row 8 is %T, want a Track", got[8])
	}
	if episode.PlaysText != "" {
		t.Errorf("the episode claims %q plays", episode.PlaysText)
	}
	if len(episode.MetadataParts) == 0 || episode.MetadataParts[0] != "Aug 26, 2022" {
		t.Errorf("the episode kept %v, want its date", episode.MetadataParts)
	}
}

// TestMusicTopResultCardIsARecord reads the card above a search, which names the
// best answer and is a record in its own right rather than a heading.
func TestMusicTopResultCardIsARecord(t *testing.T) {
	var got []any
	seen := map[string]bool{}
	if _, err := musicEmitRows(loadFixture(t, "music_search_card.json"), "test", seen, func(v any) error {
		got = append(got, v)
		return nil
	}, 0); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("emitted %d records, want the card and its two rows", len(got))
	}
	card, ok := got[0].(Artist)
	if !ok {
		t.Fatalf("the card is %T, want an Artist", got[0])
	}
	if card.ArtistID != "UCwZEU0wAwIyZb4x5G_KJp2w" || card.Name != "Rick Astley" {
		t.Errorf("the card names %s (%s)", card.Name, card.ArtistID)
	}

	// The rows under the card print "Song • 3:34" and no artist, because the card
	// above them already said whose songs they are. Their own menus still carry
	// "go to artist" and "go to album" as typed endpoints, so the ids survive
	// even though the names were never rendered.
	for i := 1; i < 3; i++ {
		track, ok := got[i].(Track)
		if !ok {
			t.Fatalf("row %d is %T, want a Track", i, got[i])
		}
		if len(track.ArtistIDs) != 1 || track.ArtistIDs[0] != "UCwZEU0wAwIyZb4x5G_KJp2w" {
			t.Errorf("row %d credits %v, want the artist off its menu", i, track.ArtistIDs)
		}
		if track.AlbumID != "MPREb_dcYZhAh5urI" {
			t.Errorf("row %d is on album %q, want the album off its menu", i, track.AlbumID)
		}
		if len(track.ArtistNames) != 0 {
			t.Errorf("row %d invented the name %v, which the page never rendered", i, track.ArtistNames)
		}
	}
}

// TestMusicAlbumPage reads Whenever You Need Somebody, which is ten songs and
// carries every field an album header has.
func TestMusicAlbumPage(t *testing.T) {
	alb, tracks := parseAlbumPage(loadFixture(t, "music_album.json"), "MPREb_dcYZhAh5urI")
	if alb.Title != "Whenever You Need Somebody" {
		t.Errorf("title = %q", alb.Title)
	}
	if alb.Year != "1987" || alb.AlbumType != "Album" {
		t.Errorf("type and year = %q %q, want Album 1987", alb.AlbumType, alb.Year)
	}
	// The OLAK playlist is the same ten songs under the id www knows them by, and
	// it is the join between an album on music and a playlist on the main site.
	if alb.PlaylistID != "OLAK5uy_nmDUsWOMoEcz0SsVqUwir0oxu-k1oUyXE" {
		t.Errorf("playlist id = %q", alb.PlaylistID)
	}
	if len(alb.ArtistIDs) != 1 || alb.ArtistIDs[0] != "UCwZEU0wAwIyZb4x5G_KJp2w" {
		t.Errorf("credited to %v %v", alb.ArtistNames, alb.ArtistIDs)
	}
	if len(tracks) != 10 || alb.TrackCount != 10 {
		t.Fatalf("read %d tracks and counted %d, want ten of each", len(tracks), alb.TrackCount)
	}
	// The header says "10 songs" and this read saw ten, so nothing is missing and
	// nothing should be reported as missing. The count parser used to answer 0 to
	// any noun it had not been told about, which made every album claim a hole.
	if len(alb.Missed) != 0 {
		t.Errorf("the album reports %v with all ten tracks in hand", alb.Missed)
	}
	if parseCountText(alb.TrackCountText) != 10 {
		t.Errorf("%q parsed as %d, want 10", alb.TrackCountText, parseCountText(alb.TrackCountText))
	}
	first := tracks[0]
	if first.VideoID != "dQw4w9WgXcQ" || first.Position != 1 {
		t.Errorf("track 1 is %s at %d", first.VideoID, first.Position)
	}
	// An album page lists the official videos, not the art tracks, and the type
	// on the endpoint is the only thing that says so.
	if first.MusicVideoType != "OMV" {
		t.Errorf("track 1 is typed %q, want OMV", first.MusicVideoType)
	}
	// A track row on an album page renders no artist of its own, so the credit
	// comes down off the header.
	if len(first.ArtistNames) != 1 || first.ArtistNames[0] != "Rick Astley" {
		t.Errorf("track 1 credits %v", first.ArtistNames)
	}
	if first.AlbumID != alb.AlbumID || first.AlbumTitle != alb.Title {
		t.Errorf("track 1 is on %q %q", first.AlbumID, first.AlbumTitle)
	}
}

// TestMusicArtistPage reads Rick Astley's artist page and checks that every
// shelf lands in the list its own contents belong in.
func TestMusicArtistPage(t *testing.T) {
	data := loadFixture(t, "music_artist.json")
	a := &Artist{}
	walkJSON(data, func(m map[string]any) {
		if h, ok := m["musicImmersiveHeaderRenderer"].(map[string]any); ok {
			musicFillArtistHeader(a, h)
		}
	})
	musicFillArtistShelves(a, data, "test")

	if a.Name != "Rick Astley" {
		t.Errorf("name = %q", a.Name)
	}
	// The artist page is where a monthly audience is stated as itself, which is
	// why a search row is right not to guess.
	if a.MonthlyListenersText != "15.6M monthly audience" {
		t.Errorf("monthly listeners = %q", a.MonthlyListenersText)
	}
	if !strings.HasPrefix(a.Description, "Richard Paul Astley") {
		t.Errorf("description = %.40q", a.Description)
	}
	// The header of an artist page carries no id of its own. The rows on it all
	// point at the artist, so the id most of them agree on is the page's own.
	if id := musicArtistIDFromShelves(data); id != "UCwZEU0wAwIyZb4x5G_KJp2w" {
		t.Errorf("the shelves point at %q", id)
	}

	// An albums lockup renders a year and a singles lockup renders its own type
	// word, which is how the page splits a discography and how this read does.
	if len(a.Albums) == 0 || len(a.Singles) == 0 {
		t.Fatalf("albums %d, singles %d, want both", len(a.Albums), len(a.Singles))
	}
	for _, item := range a.Albums {
		if item.AlbumType != "" {
			t.Errorf("album %q carries the type %q, so it belongs in singles", item.Title, item.AlbumType)
		}
	}
	for _, item := range a.Singles {
		if item.AlbumType == "" {
			t.Errorf("single %q carries no type, so it belongs in albums", item.Title)
		}
	}
	if a.Albums[0].ID != "MPREb_DBJN4xdH4Cn" || a.Albums[0].Year != "2026" {
		t.Errorf("the first album is %s (%s)", a.Albums[0].ID, a.Albums[0].Year)
	}

	// Videos come off more than one shelf, a videos one and a live one, and each
	// lockup keeps the heading it was under rather than being flattened.
	shelves := map[string]bool{}
	for _, item := range a.Videos {
		shelves[item.Shelf] = true
	}
	if !shelves["Videos"] || !shelves["Live performances"] {
		t.Errorf("videos came off %v, want both shelves kept apart", shelves)
	}

	// The top songs shelf is rows and not lockups, and it lists art tracks where
	// the videos shelf lists videos of the same songs.
	if len(a.TopTracks) == 0 {
		t.Fatal("no top tracks")
	}
	if a.TopTracks[0].VideoID != "lYBUbBu4W08" || a.TopTracks[0].MusicVideoType != "ATV" {
		t.Errorf("top track 1 is %s (%s)", a.TopTracks[0].VideoID, a.TopTracks[0].MusicVideoType)
	}
	if a.TopTracks[0].PlaysText != "2B plays" {
		t.Errorf("top track 1 has %q, and music says plays where www says views", a.TopTracks[0].PlaysText)
	}
}

// TestMusicLyrics covers both answers the lyrics tab gives.
func TestMusicLyrics(t *testing.T) {
	text, source := musicParseLyrics(loadFixture(t, "music_lyrics.json"))
	if source != "Source: Musixmatch" {
		t.Errorf("source = %q", source)
	}
	// A verse is lines. Reading it the way a title is read joins them with spaces
	// and turns a song into a paragraph.
	lines := strings.Split(text, "\n")
	if len(lines) < 40 {
		t.Fatalf("the lyrics came back as %d lines, so the line breaks were flattened", len(lines))
	}
	if lines[0] != "We're no strangers to love" {
		t.Errorf("first line = %q", lines[0])
	}

	// The tab exists for a video with no lyrics too, and answers in words. What
	// it says is the answer and is kept as the answer.
	none := loadFixture(t, "music_lyrics_none.json")
	if text, source := musicParseLyrics(none); text != "" || source != "" {
		t.Errorf("a refusal parsed as lyrics %q from %q", text, source)
	}
	if msg := musicMessage(none); msg != "Lyrics not available" {
		t.Errorf("message = %q", msg)
	}
}
