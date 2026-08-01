package youtube

import (
	"strings"
	"testing"
)

// The fixtures are three real playlist responses, reduced to the blocks the
// parser reads. Three rather than one because YouTube serves two header shapes
// and the second one has a variant that names its owner nowhere the first does:
//
//	PLlaN88a7y2_rosKX2WQt2VjFbjyDQXOkR  a person's playlist  pageHeaderRenderer
//	UUuAXFkgsw1L7xaCfnd5JJOw            a channel's uploads  playlistHeaderRenderer
//	OLAK5uy_lGQfnMNGvYCRdDq9ZLzJV2BJL2aHQsz9Y  an album      playlistHeaderRenderer, no owner
//
// Every value asserted below came off the wire.

func TestParsePlaylistViewModelHeader(t *testing.T) {
	id := "PLlaN88a7y2_rosKX2WQt2VjFbjyDQXOkR"
	p := ParsePlaylistRecord(loadFixture(t, "playlist_header_viewmodel.json"), id, SurfaceInnerTube)
	if p == nil {
		t.Fatal("no playlist record parsed")
	}
	if p.Title != "Rick Astley: Complete" {
		t.Errorf("Title = %q", p.Title)
	}
	if p.ChannelID != "UCuAXFkgsw1L7xaCfnd5JJOw" || p.ChannelTitle != "Rick Astley" {
		t.Errorf("owner = %q %q", p.ChannelID, p.ChannelTitle)
	}
	// The header says 128 and the page serves 64. The count is the header's claim
	// and the difference is what the alert explains, so both are kept.
	if p.VideoCount != 128 {
		t.Errorf("VideoCount = %d, want 128", p.VideoCount)
	}
	if p.IsGenerated {
		t.Error("a PL id is a playlist somebody made")
	}
	if len(p.Thumbnails) == 0 {
		t.Error("no thumbnails, the sources live under heroImage.contentPreviewImageViewModel")
	}
	// The alert is quoted rather than matched, because it is localized and it does
	// not always state the count.
	if !hasMiss(p.Missed, "64 unavailable videos are hidden") {
		t.Errorf("the hidden videos alert should be quoted into missed, got %q", p.Missed)
	}
	if !hasMiss(p.Missed, "no updated_text") {
		t.Errorf("this shape carries no Updated line and should say so, got %q", p.Missed)
	}
	if p.Via["header"] == "" {
		t.Error("via should name which of the two header shapes answered")
	}
}

func TestParsePlaylistUploadsHeader(t *testing.T) {
	id := "UUuAXFkgsw1L7xaCfnd5JJOw"
	p := ParsePlaylistRecord(loadFixture(t, "playlist_header_uploads.json"), id, SurfaceInnerTube)
	if p == nil {
		t.Fatal("no playlist record parsed")
	}
	if p.Title != "Uploads from Rick Astley" {
		t.Errorf("Title = %q", p.Title)
	}
	if p.ChannelID != "UCuAXFkgsw1L7xaCfnd5JJOw" || p.ChannelTitle != "Rick Astley" {
		t.Errorf("owner = %q %q", p.ChannelID, p.ChannelTitle)
	}
	if !p.IsGenerated {
		t.Error("a UU id is a list YouTube maintains, not one a person built")
	}
	// Only this header shape states when the playlist last changed.
	if p.UpdatedText == "" {
		t.Error("playlistHeaderRenderer carries lastUpdatedText and it should be read")
	}
	// The byline restates the counts the named keys already carry, and restating
	// them is not the same as finding something new.
	if len(p.MetadataParts) != 0 {
		t.Errorf("MetadataParts = %q, want nothing: every byline fragment here was recognised", p.MetadataParts)
	}
}

// An album is the one playlist that names its owner nowhere but a single
// rendered line, "Oasis • Album", with no link on it.
func TestParsePlaylistAlbumHeader(t *testing.T) {
	id := "OLAK5uy_lGQfnMNGvYCRdDq9ZLzJV2BJL2aHQsz9Y"
	p := ParsePlaylistRecord(loadFixture(t, "playlist_header_album.json"), id, SurfaceInnerTube)
	if p == nil {
		t.Fatal("no playlist record parsed")
	}
	if p.ChannelTitle != "Oasis" {
		t.Errorf("ChannelTitle = %q, want the artist off the subtitle", p.ChannelTitle)
	}
	if p.ChannelID != "" {
		t.Errorf("ChannelID = %q, want empty: the subtitle is text with no link on it", p.ChannelID)
	}
	if !hasMiss(p.Missed, "owner as text only") {
		t.Errorf("the missing channel id should be stated, got %q", p.Missed)
	}
	if p.VideoCount != 40 {
		t.Errorf("VideoCount = %d, want 40", p.VideoCount)
	}
	// YouTube marks its own album playlists unlisted and the microformat is where
	// it says so.
	if p.Visibility != "unlisted" {
		t.Errorf("Visibility = %q, want unlisted", p.Visibility)
	}
}

// A response with neither header shape on it is a mix, and the caller turns that
// into YouTube's own refusal rather than into an empty playlist.
func TestParsePlaylistRecordNoHeader(t *testing.T) {
	if p := ParsePlaylistRecord(map[string]any{"alerts": []any{}}, "RDdQw4w9WgXcQ", SurfaceInnerTube); p != nil {
		t.Errorf("a response with no header should parse to nothing, got %+v", p)
	}
}

// Position is the yield order counted from a base the caller carries across
// pages, because a lockup states no index of its own.
func TestParsePlaylistItems(t *testing.T) {
	items, token := ParsePlaylistItems(loadFixture(t, "playlist_items.json"), 20)
	if len(items) != 4 {
		t.Fatalf("got %d items, want the 4 lockups in the fixture", len(items))
	}
	for i, v := range items {
		if v.Position != 21+i {
			t.Errorf("item %d position = %d, want %d", i, v.Position, 21+i)
		}
		if v.VideoID == "" || v.Title == "" {
			t.Errorf("item %d is missing an id or a title: %+v", i, v)
		}
		if v.ChannelID == "" {
			t.Errorf("item %d has no channel id, and every video lockup carries one on its owner run", i)
		}
		if v.SetVideoID != "" {
			t.Errorf("item %d claims a set_video_id, which no lockup states", i)
		}
		if !hasMiss(v.Missed, "set_video_id is not available") {
			t.Errorf("item %d should say the edge id is gone, got %q", i, v.Missed)
		}
	}
	if token == "" {
		t.Error("the fixture ends with a continuationItemRenderer and its token should come back")
	}
}

func hasMiss(missed []string, substr string) bool {
	for _, m := range missed {
		if strings.Contains(m, substr) {
			return true
		}
	}
	return false
}
