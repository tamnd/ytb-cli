package youtube

import "testing"

// The fixtures are two real search responses reduced to the shapes they serve.
//
// search_plain.json is "rick astley" with no filter: one officialCardViewModel,
// two videoRenderer, a shelfRenderer holding a video and a gridShelfViewModel
// holding shorts, then the continuation. That mix is the point of the fixture.
// A plain search serves no channelRenderer at all any more, so a reader that
// knows only the old shapes returns the videos and silently drops the channel
// the query was about.
//
// search_playlists.json is the same query with --type playlist: lockups, and a
// channel row first, because YouTube puts one there.

func TestParseSearchResultsPlain(t *testing.T) {
	items, token := ParseSearchResults(loadFixture(t, "search_plain.json"))
	if len(items) != 6 {
		t.Fatalf("got %d rows, want 6: the card, two videos, the shelf's video and two shorts", len(items))
	}

	// Rank is data. The card the site drew first is the row emitted first.
	ch, ok := items[0].(Channel)
	if !ok {
		t.Fatalf("row 0 is %T, want the Channel off the officialCardViewModel", items[0])
	}
	if ch.ChannelID != "UCuAXFkgsw1L7xaCfnd5JJOw" {
		t.Errorf("ChannelID = %q", ch.ChannelID)
	}
	if ch.Handle != "@RickAstleyYT" {
		t.Errorf("Handle = %q", ch.Handle)
	}
	if ch.SubscriberCount == 0 || !ch.SubscriberCountIsApproximate {
		t.Errorf("subscriber count = %d %q, the card states a rounded one", ch.SubscriberCount, ch.SubscriberCountText)
	}
	if ch.VideoCount == 0 {
		t.Errorf("VideoCount = %d, the card states one", ch.VideoCount)
	}
	if ch.Description == "" {
		t.Error("the card carries a description and channelRenderer only carries a snippet")
	}
	if len(ch.Avatar) == 0 {
		t.Error("no avatar, the sources live under image.contentPreviewImageViewModel")
	}

	shorts := 0
	for _, item := range items[1:] {
		v, ok := item.(Video)
		if !ok {
			t.Fatalf("row is %T, want Video", item)
		}
		if v.VideoID == "" || v.Title == "" {
			t.Errorf("row is missing an id or a title: %+v", v)
		}
		if v.IsShort != nil && *v.IsShort {
			shorts++
			// A shorts row names nobody, anywhere in its subtree, and says so
			// rather than looking like a video with no uploader.
			if !hasMiss(v.Missed, "carries no owner") {
				t.Errorf("a shorts row should say it names no owner, got %q", v.Missed)
			}
			continue
		}
		if v.ChannelID == "" {
			t.Errorf("%q has no channel id and every video row but a short carries one", v.Title)
		}
	}
	if shorts != 2 {
		t.Errorf("found %d shorts, want the 2 in the grid shelf: a shelf is a box of rows and is opened in place", shorts)
	}
	if token == "" {
		t.Error("the fixture ends with a continuationItemRenderer and its token should come back")
	}
}

func TestParseSearchResultsPlaylists(t *testing.T) {
	items, _ := ParseSearchResults(loadFixture(t, "search_playlists.json"))
	if len(items) != 3 {
		t.Fatalf("got %d rows, want 3", len(items))
	}
	if _, ok := items[0].(Channel); !ok {
		t.Fatalf("row 0 is %T, want a Channel: a playlist search answers with one first", items[0])
	}
	for _, item := range items[1:] {
		p, ok := item.(Playlist)
		if !ok {
			t.Fatalf("row is %T, want Playlist", item)
		}
		if p.PlaylistID == "" || p.Title == "" {
			t.Errorf("row is missing an id or a title: %+v", p)
		}
		// The owner run comes first and the "Playlist" label that follows carries
		// the same browse id, so the first one has to win.
		if p.ChannelID == "" || p.ChannelTitle == "" {
			t.Errorf("%q has no owner: %q %q", p.Title, p.ChannelID, p.ChannelTitle)
		}
		if p.ChannelTitle == "Playlist" {
			t.Errorf("%q took the label for its owner", p.Title)
		}
		// The count is not in the metadata at all on a search row. It rides on the
		// collection thumbnail as an overlay badge, under a key a video lockup
		// does not use.
		if p.VideoCount == 0 {
			t.Errorf("%q has no video count, which the thumbnail badge states", p.Title)
		}
		if len(p.Thumbnails) == 0 {
			t.Errorf("%q has no thumbnails, which nest one level deeper than a video's", p.Title)
		}
	}
}

// The channel row on a search states its handle where the key says subscribers
// and its subscriber count where the key says videos. Whatever that is, it is
// not something to read by key name.
func TestParseChannelRendererShiftedFields(t *testing.T) {
	items, _ := ParseSearchResults(loadFixture(t, "search_playlists.json"))
	ch := items[0].(Channel)
	if ch.Handle == "" || ch.Handle[0] != '@' {
		t.Errorf("Handle = %q, want the @ handle YouTube put in subscriberCountText", ch.Handle)
	}
	if ch.SubscriberCountText == ch.Handle {
		t.Errorf("the handle was taken for the subscriber count: %q", ch.SubscriberCountText)
	}
	if ch.SubscriberCount == 0 {
		t.Errorf("SubscriberCount = 0, want the count YouTube put in videoCountText")
	}
}
