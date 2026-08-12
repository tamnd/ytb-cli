package ytb

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The fixture is @RickAstleyYT's real Atom feed, as the site served it.

func loadFeed(t *testing.T) []Video {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "channel_feed.xml"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	videos, err := ParseChannelFeed(b, "https://www.youtube.com/feeds/videos.xml?channel_id=UCuAXFkgsw1L7xaCfnd5JJOw")
	if err != nil {
		t.Fatalf("parse feed: %v", err)
	}
	return videos
}

// Fifteen entries and no paging, ever. That is the whole shape of this surface
// and it is why the feed corrects a listing rather than replacing one.
func TestParseChannelFeedShape(t *testing.T) {
	videos := loadFeed(t)
	if len(videos) != 15 {
		t.Fatalf("feed carried %d entries, the format serves 15", len(videos))
	}
	for _, v := range videos {
		if v.VideoID == "" || v.Title == "" {
			t.Errorf("entry with no id or title: %+v", v)
		}
		if v.ChannelID != "UCuAXFkgsw1L7xaCfnd5JJOw" {
			// The feed level <id> has the UC stripped off and the per entry
			// yt:channelId does not. This reads the entry's.
			t.Errorf("channel id = %q, the per entry yt:channelId is the full one", v.ChannelID)
		}
		if v.Kind != "video" || !v.Surfaces.Has(SurfaceFeed) {
			t.Errorf("envelope = %+v", v.Envelope)
		}
		if len(v.Missed) == 0 {
			t.Error("a feed read should say what it did not see")
		}
	}
}

// The one thing this surface has that no listing does: a timestamp to the second,
// with the uploader's own offset on it rather than a Z time.
func TestParseChannelFeedExactTimes(t *testing.T) {
	videos := loadFeed(t)
	for _, v := range videos {
		if v.PublishedAt.IsZero() {
			t.Errorf("%s has no published time", v.VideoID)
			continue
		}
		if v.PublishedAt.Second() == 0 && v.PublishedAt.Minute() == 0 {
			t.Errorf("%s published at %v, which looks rounded rather than exact", v.VideoID, v.PublishedAt)
		}
		if v.Via["published_at"] == "" {
			t.Errorf("%s should say where its timestamp came from", v.VideoID)
		}
	}
	// Newest first is the order the feed serves and the order a caller expects.
	for i := 1; i < len(videos); i++ {
		if videos[i].PublishedAt.After(videos[i-1].PublishedAt) {
			t.Errorf("entry %d is newer than entry %d, so the feed order was not kept", i, i-1)
		}
	}
}

// The alternate link is /shorts/<id> for a short and /watch?v= for everything
// else, which is the only place outside the player that says so.
func TestParseChannelFeedMarksShorts(t *testing.T) {
	videos := loadFeed(t)
	shorts, longs := 0, 0
	for _, v := range videos {
		if v.IsShort == nil {
			t.Fatalf("%s: the feed answers this for every entry, so it is never absent", v.VideoID)
		}
		if *v.IsShort {
			shorts++
		} else {
			longs++
		}
	}
	if shorts == 0 {
		t.Error("this channel's newest fifteen include shorts and none were marked")
	}
	if shorts+longs != len(videos) {
		t.Fatal("counting is broken")
	}
}

// media:starRating count is a rating count, not a like count. Since dislikes went
// away the two are nearly the same and nearly is not the same, so it must not end
// up in like_count.
func TestParseChannelFeedDoesNotInventLikes(t *testing.T) {
	for _, v := range loadFeed(t) {
		if v.LikeCount != 0 {
			t.Errorf("%s has like_count %d and the feed states no like count", v.VideoID, v.LikeCount)
		}
		if v.ViewCount <= 0 {
			t.Errorf("%s has no view count and media:statistics states an exact one", v.VideoID)
		}
	}
}

// The cross read. A listing row keeps its duration and its rounded text, and gets
// the feed's exact numbers on top, with the envelope saying both surfaces
// answered.
func TestApplyFeedExact(t *testing.T) {
	entries := loadFeed(t)
	index := feedIndex(entries)
	if len(index) != len(entries) {
		t.Fatalf("index dropped entries: %d of %d", len(index), len(entries))
	}
	entry := entries[0]

	row := *NewVideo(entry.VideoID, SurfaceInnerTube)
	row.DurationText = "3:33"
	row.ViewCountText = "22K views"
	row.ViewCount = 22000
	row.PublishedText = "4 days ago"
	applyFeedExact(&row, entry)

	if !row.PublishedAt.Equal(entry.PublishedAt) {
		t.Errorf("published at = %v, want %v", row.PublishedAt, entry.PublishedAt)
	}
	if row.ViewCount != entry.ViewCount {
		t.Errorf("view count = %d, want the feed's exact %d", row.ViewCount, entry.ViewCount)
	}
	// What the feed has no answer for is left alone rather than blanked.
	if row.DurationText != "3:33" || row.ViewCountText != "22K views" || row.PublishedText != "4 days ago" {
		t.Errorf("the cross read overwrote fields the feed does not carry: %+v", row)
	}
	if !row.Surfaces.Has(SurfaceFeed) || !row.Surfaces.Has(SurfaceInnerTube) {
		t.Errorf("surfaces = %v, both reads should be named", row.Surfaces)
	}
	if row.Via["published_at"] == "" || row.Via["view_count"] == "" {
		t.Errorf("via = %v", row.Via)
	}
}

// An Atom time carries the uploader's offset. Parsing it as a Z time would move
// the moment.
func TestAtomTime(t *testing.T) {
	got := atomTime("2026-07-27T14:30:23+00:00")
	if !got.Equal(time.Date(2026, 7, 27, 14, 30, 23, 0, time.UTC)) {
		t.Errorf("atomTime = %v", got)
	}
	if !atomTime("").IsZero() || !atomTime("not a time").IsZero() {
		t.Error("an unparseable time should be the zero time, not a guess")
	}
}

// The feed url is the one channelMetadataRenderer names, so a read that has an id
// and no page builds the same address the site does.
func TestChannelFeedURL(t *testing.T) {
	want := "https://www.youtube.com/feeds/videos.xml?channel_id=UCuAXFkgsw1L7xaCfnd5JJOw"
	if got := ChannelFeedURL("UCuAXFkgsw1L7xaCfnd5JJOw"); got != want {
		t.Errorf("ChannelFeedURL = %q, want %q", got, want)
	}
}
