package ytb

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The fixtures are two real channel pages, reduced to the four blocks the parser
// reads: metadata, microformat, header and the tab strip. Every assertion below
// is a value that came off the wire.
//
// Two channels rather than one, because the interesting fields are the ones only
// one of them has. @RickAstleyYT is an official artist channel with no banner and
// ten external links; @BBCNews is a verified channel with a banner, a podcasts
// tab and no artist bio at all. A parser tested against one of them alone would
// pass while getting the other completely wrong.

func loadChannelPage(t *testing.T, name string) *PageData {
	t.Helper()
	return &PageData{InitialData: loadFixture(t, name)}
}

func TestParseChannelRecordArtist(t *testing.T) {
	ch := ParseChannelRecord(loadChannelPage(t, "channel_page_artist.json"), "https://www.youtube.com/@RickAstleyYT")
	if ch == nil {
		t.Fatal("no channel record parsed")
	}
	if ch.ChannelID != "UCuAXFkgsw1L7xaCfnd5JJOw" {
		t.Errorf("id = %q", ch.ChannelID)
	}
	if ch.Handle != "@RickAstleyYT" {
		t.Errorf("handle = %q", ch.Handle)
	}
	// YouTube spells vanityChannelUrl with an http scheme. The record normalises it
	// and keeps the site's own spelling in canonical_url.
	if !strings.HasPrefix(ch.HandleURL, "https://") {
		t.Errorf("handle url should be normalised to https, got %q", ch.HandleURL)
	}
	if ch.UploadsPlaylistID != "UUuAXFkgsw1L7xaCfnd5JJOw" {
		t.Errorf("uploads playlist = %q", ch.UploadsPlaylistID)
	}
	if ch.RSSURL == "" {
		t.Error("rss url should come from the page rather than be built here")
	}
	if ch.IsFamilySafe == nil || !*ch.IsFamilySafe {
		t.Errorf("is_family_safe = %v", ch.IsFamilySafe)
	}
	// 249 country codes is a channel with no restriction at all, which is still an
	// answer worth carrying.
	if len(ch.AvailableCountryCodes) < 200 {
		t.Errorf("available country codes = %d, expected the full list", len(ch.AvailableCountryCodes))
	}
	if len(ch.Avatar) == 0 {
		t.Error("no avatar renditions")
	}
	// This channel has no banner key on its header at all. Absent and empty are
	// different claims and the record makes the absent one.
	if len(ch.Banner) != 0 {
		t.Errorf("this channel has no banner, got %d renditions", len(ch.Banner))
	}
	if !ch.IsArtist {
		t.Error("the AUDIO_BADGE beside the title says official artist channel")
	}
	if ch.IsVerified {
		t.Error("an official artist channel carries AUDIO_BADGE, not CHECK_CIRCLE_FILLED")
	}
	if !ch.SubscriberCountIsApproximate {
		t.Error("YouTube rounds every subscriber count it publishes, so this is always true")
	}
}

// The header shows one link and says "and 9 more links". sameAs is the whole
// list, at tier 0, with no continuation and no about panel read.
func TestParseChannelRecordLinksFromSameAs(t *testing.T) {
	ch := ParseChannelRecord(loadChannelPage(t, "channel_page_artist.json"), "https://www.youtube.com/@RickAstleyYT")
	if len(ch.Links) < 5 {
		t.Fatalf("links = %d, expected the full sameAs list", len(ch.Links))
	}
	for _, l := range ch.Links {
		if strings.Contains(l.URL, "youtube.com/redirect") {
			t.Errorf("sameAs urls are already unwrapped, got %q", l.URL)
		}
		if l.URL == "" {
			t.Error("a link with no url")
		}
	}
	if ch.Via["links"] == "" {
		t.Error("via should name the surface the links came from")
	}
}

// FollowAction's userInteractionCount is the only place on the page that states
// subscribers as a number. It is still rounded to three significant figures,
// which is what is_approximate is for.
func TestParseChannelRecordSubscriberCount(t *testing.T) {
	ch := ParseChannelRecord(loadChannelPage(t, "channel_page_artist.json"), "https://www.youtube.com/@RickAstleyYT")
	if ch.SubscriberCount < 1_000_000 {
		t.Fatalf("subscriber count = %d", ch.SubscriberCount)
	}
	if ch.SubscriberCount%1000 != 0 {
		t.Errorf("a rounded count should not end in three significant digits of noise, got %d", ch.SubscriberCount)
	}
	if !strings.Contains(ch.SubscriberCountText, "subscriber") {
		t.Errorf("subscriber count text = %q", ch.SubscriberCountText)
	}
}

func TestParseChannelRecordVerified(t *testing.T) {
	ch := ParseChannelRecord(loadChannelPage(t, "channel_page_verified.json"), "https://www.youtube.com/@BBCNews")
	if ch == nil {
		t.Fatal("no channel record parsed")
	}
	if !ch.IsVerified {
		t.Error("the CHECK_CIRCLE_FILLED beside the title says verified")
	}
	if ch.IsArtist {
		t.Error("a news channel is not an official artist channel")
	}
	if len(ch.Banner) == 0 {
		t.Error("this channel has a banner and the record dropped it")
	}
	// The badge is matched on the image name and not on the accessibility label,
	// because the label is translated and the image name is not.
	if ch.Title == "" || ch.ChannelID == "" {
		t.Errorf("id/title = %q / %q", ch.ChannelID, ch.Title)
	}
}

// The tab strip is data rather than a table compiled into this tool, which is
// what lets a read of a missing tab name the tabs the channel really has.
func TestParseChannelRecordTabs(t *testing.T) {
	cases := map[string][]string{
		"channel_page_artist.json":   {"featured", "videos", "shorts", "streams", "playlists", "posts", "releases", "search"},
		"channel_page_verified.json": {"featured", "videos", "shorts", "streams", "playlists", "posts", "podcasts", "search"},
	}
	for name, want := range cases {
		ch := ParseChannelRecord(loadChannelPage(t, name), "https://www.youtube.com/")
		got := map[string]bool{}
		for _, tab := range ch.Tabs {
			got[tab.Slug] = true
		}
		for _, slug := range want {
			if !got[slug] {
				t.Errorf("%s: missing tab %q, got %v", name, slug, slugs(ch.Tabs))
			}
		}
	}
}

// The census. channelMetadataRenderer and microformatDataRenderer are the two
// blocks this record is built from, and every key in them is accounted for in
// the maps: read, or named and dismissed with a reason. A key YouTube adds shows
// up here as a failure rather than as a field that quietly never appears.
func TestChannelBlockCensus(t *testing.T) {
	for _, name := range []string{"channel_page_artist.json", "channel_page_verified.json"} {
		root := loadFixture(t, name)
		blocks := []struct {
			label  string
			got    map[string]any
			census map[string]string
		}{
			{"channelMetadataRenderer", mapValue(mapValue(root, "metadata"), "channelMetadataRenderer"), channelMetadataFields},
			{"microformatDataRenderer", mapValue(mapValue(root, "microformat"), "microformatDataRenderer"), channelMicroformatFields},
		}
		for _, b := range blocks {
			if b.got == nil {
				t.Fatalf("%s: %s missing from the page", name, b.label)
			}
			for key := range b.got {
				if _, ok := b.census[key]; !ok {
					t.Errorf("%s: %s has key %q that no census entry covers", name, b.label, key)
				}
			}
		}
	}
}

// The ld+json block in the HTML and the profilePage inside ytInitialData are the
// same object with different key spellings, @type against type. The parsed one is
// preferred; this checks the fallback reads the same channel.
func TestChannelProfilePageFromHTML(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "channel_ldjson.html"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	profile := channelProfilePage(nil, string(b))
	if profile == nil {
		t.Fatal("no ProfilePage found in the html block")
	}
	if got := stringValue(profile["type"]); got != "ProfilePage" {
		t.Errorf("@type should be folded to type, got %q", got)
	}
	me := mapValue(profile, "mainEntity")
	if stringValue(me["alternateName"]) != "@RickAstleyYT" {
		t.Errorf("handle = %q", stringValue(me["alternateName"]))
	}
	if len(stringSlice(me["sameAs"])) == 0 {
		t.Error("sameAs is the whole external link list and it came back empty")
	}
	if followerCount(me["interactionStatistic"]) == 0 {
		t.Error("FollowAction should give a subscriber count")
	}
}

// keywords is one space separated string with quoted phrases in it, so splitting
// on spaces would turn one phrase into three keywords.
func TestSplitKeywords(t *testing.T) {
	got := splitKeywords(`Official Rick Astley "rick astley" "never gonna give you up" meme`)
	want := []string{"Official", "Rick", "Astley", "rick astley", "never gonna give you up", "meme"}
	if len(got) != len(want) {
		t.Fatalf("got %d keywords %q, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("keyword %d = %q, want %q", i, got[i], want[i])
		}
	}
	if len(splitKeywords("")) != 0 {
		t.Error("no keywords should be no keywords, not one empty one")
	}
}

// The panel writes the join date two ways and which one it picks follows the
// content country. Both have to parse or the tool works in one country.
func TestParseJoinedDate(t *testing.T) {
	want := time.Date(2015, 2, 1, 0, 0, 0, 0, time.UTC)
	for _, s := range []string{"Joined Feb 1, 2015", "Joined 1 Feb 2015", "Joined February 1, 2015"} {
		if got := parseJoinedDate(s); !got.Equal(want) {
			t.Errorf("parseJoinedDate(%q) = %v, want %v", s, got, want)
		}
	}
	// en-GB writes September as Sept, four letters where every other month is
	// three.
	if got := parseJoinedDate("Joined 12 Sept 2009"); !got.Equal(time.Date(2009, 9, 12, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("Sept did not parse, got %v", got)
	}
	// A panel in a language this cannot read is a zero time, not a guessed date.
	for _, s := range []string{"", "Beigetreten am 1. Feb. 2015 irgendwas", "no date here"} {
		if got := parseJoinedDate(s); !got.IsZero() {
			t.Errorf("parseJoinedDate(%q) = %v, want the zero time", s, got)
		}
	}
}

// A channelRenderer is a listing row for a channel, not a read of one, and the
// envelope has to say so or a consumer will store it as if it were a record.
func TestParseChannelRendererIsARow(t *testing.T) {
	c := parseChannelRenderer(map[string]any{
		"channelId":           "UCuAXFkgsw1L7xaCfnd5JJOw",
		"title":               map[string]any{"simpleText": "Rick Astley"},
		"subscriberCountText": map[string]any{"simpleText": "4.52M subscribers"},
	})
	if c.ChannelID == "" || c.Title != "Rick Astley" {
		t.Fatalf("row = %+v", c)
	}
	if c.SubscriberCount != 4_520_000 {
		t.Errorf("subscriber count = %d", c.SubscriberCount)
	}
	if !c.SubscriberCountIsApproximate {
		t.Error("a listing row's count is rounded like every other one")
	}
	if len(c.Missed) == 0 {
		t.Error("a row should say what a real read would add")
	}
	if len(c.Tabs) != 0 || c.RSSURL != "" {
		t.Error("a row has no tab strip and no rss url and should not invent them")
	}
}

// The three kinds partition the uploads playlist, so the sum is a check on the
// whole derivation. The String is what the table shows and it states the
// arithmetic rather than the verdict alone.
func TestChannelCountsString(t *testing.T) {
	c := ChannelCounts{Uploads: 435, Videos: 139, Shorts: 294, Streams: 2, Agrees: true}
	got := c.String()
	for _, want := range []string{"139", "294", "2", "435", "agrees"} {
		if !strings.Contains(got, want) {
			t.Errorf("String() = %q, missing %q", got, want)
		}
	}
	bad := ChannelCounts{Uploads: 435, Videos: 139, Shorts: 200, Streams: 2}
	if !strings.Contains(bad.String(), "does not agree") {
		t.Errorf("a sum that does not add up should say so, got %q", bad.String())
	}
}
