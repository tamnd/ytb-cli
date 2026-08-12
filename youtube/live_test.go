//go:build live

package youtube

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// live_test.go asks the one question the other three suites cannot: are these
// still the bytes YouTube sends.
//
//	go test ./youtube -tags live
//
// It is behind a build tag so it never runs in the default suite and never on a
// schedule that would hammer the site. Doc 06 section 5.
//
// Everything in here asserts a shape and not a value. A view count is different
// every hour, so a test that pins one fails every hour and gets deleted; a test
// that says the count is in the billions holds for years and still catches the
// day the parser starts reading a different number.
//
// A read YouTube refused is a skip and not a failure, and the skip quotes the
// refusal. Restricted Mode and an age gate are answers about this network and
// this session, not about the parser, and a suite that goes red when someone
// runs it from an address YouTube does not like is a suite nobody runs. Exit 4
// and exit 5 skip; everything else, including a network failure, still fails.
//
// YTB_COOKIES raises the suite to tier 1. The tests that need it skip without
// it, which is the ordinary case.

// The subjects. The same ones the fixtures were captured from, because a live
// failure is only useful next to the fixture it contradicts.
const (
	liveVideo    = "dQw4w9WgXcQ"
	liveChannel  = "@RickAstleyYT"
	liveVerified = "@BBCNews"
	liveMix      = "RDdQw4w9WgXcQ"
)

func liveClient(t *testing.T) *Client {
	t.Helper()
	cfg := DefaultConfig()
	// Slower than the default on purpose. This suite is run by hand and there is
	// no hurry, and the site is somebody else's.
	cfg.Delay = 2 * time.Second
	c := NewClient(cfg)
	if raw := strings.TrimSpace(os.Getenv("YTB_COOKIES")); raw != "" {
		c.SetSession(ParseCookies(raw))
	}
	return c
}

// liveTier1 returns a client with a session, or skips.
func liveTier1(t *testing.T) *Client {
	t.Helper()
	c := liveClient(t)
	if c.Tier() < 1 {
		t.Skip("no YTB_COOKIES in the environment, so this read would be tier 0")
	}
	return c
}

// skipRefusals turns YouTube's no into a skip, and leaves everything else alone.
func skipRefusals(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	if r, ok := AsRefusal(err); ok {
		t.Skipf("YouTube refused: %s", r.Message)
	}
	if strings.Contains(err.Error(), "429") || strings.Contains(strings.ToLower(err.Error()), "rate limit") {
		t.Skipf("rate limited after the retries: %v", err)
	}
	t.Fatal(err)
}

func liveContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// TestLiveVideo is the whole video read against the video every fixture in this
// directory was captured from.
func TestLiveVideo(t *testing.T) {
	c := liveClient(t)
	res, err := c.FetchVideo(liveContext(t), liveVideo, VideoOptions{})
	skipRefusals(t, err)

	v := res.Video
	if len(v.VideoID) != 11 {
		t.Errorf("video id %q is not eleven characters", v.VideoID)
	}
	if v.Title == "" || v.ChannelID == "" {
		t.Errorf("title %q, channel %q", v.Title, v.ChannelID)
	}
	if !strings.HasPrefix(v.ChannelID, "UC") || len(v.ChannelID) != 24 {
		t.Errorf("channel id %q is not a UC id", v.ChannelID)
	}
	// It passed a billion in 2021 and it is not going down.
	if v.ViewCount < 1_000_000_000 {
		t.Errorf("view count = %d, which is below where this video was five years ago", v.ViewCount)
	}
	// The two blocks on the page disagree by a second, and the answer is one of
	// them rather than a third number.
	if v.DurationSeconds != 213 && v.DurationSeconds != 214 {
		t.Errorf("duration = %d, want the 213 videoDetails says or the 214 microformat does", v.DurationSeconds)
	}
	if v.PublishedAt.Year() != 2009 {
		t.Errorf("published %v, and it was uploaded in 2009", v.PublishedAt)
	}
	if v.Envelope.Kind != "video" || len(v.Envelope.Surfaces) == 0 {
		t.Errorf("the envelope says kind %q from %v", v.Envelope.Kind, v.Envelope.Surfaces)
	}
}

// TestLiveTheTwoPlayersStillDisagree is the reason the format list is read from
// the mobile player. The watch page states sizes and no URLs, ANDROID states
// URLs, and the day that flips is the day format.go can be simplified.
func TestLiveTheTwoPlayersStillDisagree(t *testing.T) {
	c := liveClient(t)
	ctx := liveContext(t)

	android, err := NewInnerTube(c).AndroidPlayer(ctx, liveVideo)
	skipRefusals(t, err)
	if !streamingHasURLs(mapValue(android, "streamingData")) {
		t.Error("ANDROID answered with no urls at all, so nothing this tool can fetch is fetchable")
	}

	page, _, err := c.FetchPageData(ctx, NormalizeVideoURL(liveVideo))
	skipRefusals(t, err)
	web := page.PlayerResp
	if web == nil {
		t.Fatal("the watch page carried no player response")
	}
	sd := mapValue(web, "streamingData")
	if streamingHasURLs(sd) {
		t.Log("the watch page now serves plain urls, which it did not when format.go was written")
	}
	// Whatever it does about urls, the sizes are the reason to read it.
	var sized int
	for _, item := range arrayValue(sd["adaptiveFormats"]) {
		if m, ok := item.(map[string]any); ok && stringValue(m["contentLength"]) != "" {
			sized++
		}
	}
	if sized == 0 {
		t.Error("no adaptive format on the watch page states a contentLength")
	}
}

// TestLiveChannelCountsAddUp reads a channel and its four derived playlists. The
// three parts are the whole, and when they are not the record says so rather
// than printing four numbers that quietly disagree. Doc 01 section 2.5.
func TestLiveChannelCountsAddUp(t *testing.T) {
	c := liveClient(t)
	ch, err := c.FetchChannel(liveContext(t), liveChannel, ChannelOptions{Counts: true})
	skipRefusals(t, err)
	if ch.Counts == nil {
		t.Fatal("--counts read nothing")
	}
	if ch.Counts.Uploads == 0 {
		t.Fatal("the uploads playlist came back empty, and this channel has hundreds")
	}
	if !ch.Counts.Agrees {
		t.Errorf("%s. The record reports this as a miss, so this is a note about the site rather than about the parser.", ch.Counts)
	}
	if ch.VideoCount != int64(ch.Counts.Uploads) {
		t.Errorf("video_count = %d where the uploads playlist holds %d, and the playlist is meant to win",
			ch.VideoCount, ch.Counts.Uploads)
	}
}

// TestLiveLanguageIsHonoured is the locale rule from doc 01 section 1.3, checked
// against the site rather than against a fixture.
//
// Four places pin the language and all four have to agree, so this asks for a
// page in Vietnamese and checks that the rendered counts came back in
// Vietnamese. The parser reads those counts positionally and never by matching
// an English word, and this is what proves there is something to read.
//
// Asking with no language pinned at all is the more interesting test and it is
// not this one. On the network this was written on, an unpinned read comes back
// in English regardless of address, so the assertion would say more about the
// exit node than about YouTube.
func TestLiveLanguageIsHonoured(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HL, cfg.GL = "vi", "VN"
	cfg.Delay = 2 * time.Second
	c := NewClient(cfg)

	ch, err := c.FetchChannel(liveContext(t), liveVerified, ChannelOptions{NoAbout: true})
	skipRefusals(t, err)

	if strings.Contains(ch.SubscriberCountText, "subscriber") {
		t.Fatalf("asked for vi and got English back: %q. Everything below is untested.", ch.SubscriberCountText)
	}
	if ch.SubscriberCount < 1_000_000 {
		t.Errorf("subscriber count = %d from %q, so the count was rendered and not read",
			ch.SubscriberCount, ch.SubscriberCountText)
	}
	if ch.VideoCount == 0 {
		t.Errorf("video count = 0 from %q", ch.VideoCountText)
	}
	// The tabs are found by slug, so they are the same eight words in every
	// language even when their titles are not.
	var slugs []string
	for _, tab := range ch.Tabs {
		slugs = append(slugs, tab.Slug)
	}
	for _, want := range []string{"featured", "videos", "shorts"} {
		if !contains(slugs, want) {
			t.Errorf("no %s tab in %v, and a slug does not translate", want, slugs)
		}
	}
}

// TestLiveMixStillRefuses exists to notice a new thing rather than a broken one.
// The day this stops refusing is the day mixes become readable, and a whole
// command becomes possible. Doc 01 section 2.3.
func TestLiveMixStillRefuses(t *testing.T) {
	c := liveClient(t)
	resp, err := NewInnerTube(c).Browse(liveContext(t), "VL"+liveMix, "", "")
	if err != nil {
		if _, ok := AsRefusal(err); ok {
			return // still refused, which is the answer this test wants
		}
		skipRefusals(t, err)
	}
	if r := alertRefusal(resp, liveMix, "browse"); r != nil {
		return
	}
	if mapValue(resp, "contents") != nil {
		t.Error("the mix browse answered with contents. Mixes may be readable now, which would be worth a command.")
	}
}

// TestLiveSearch checks the one surface where the shape of a row is not fixed:
// a plain search returns videos, channels, playlists and shelves together.
func TestLiveSearch(t *testing.T) {
	c := liveClient(t)
	resp, err := NewInnerTube(c).Search(liveContext(t), "rick astley", SearchFilters{}, "")
	skipRefusals(t, err)

	items, token := ParseSearchResults(resp)
	if len(items) < 10 {
		t.Errorf("a plain search returned %d rows, and the page holds about twenty", len(items))
	}
	if token == "" {
		t.Error("no continuation, so search would stop after one page")
	}
	for _, item := range items {
		switch v := item.(type) {
		case Video:
			if len(v.VideoID) != 11 {
				t.Errorf("video row has id %q", v.VideoID)
			}
		case Channel:
			if !strings.HasPrefix(v.ChannelID, "UC") {
				t.Errorf("channel row has id %q", v.ChannelID)
			}
		case Playlist:
			if v.PlaylistID == "" {
				t.Error("playlist row has no id")
			}
		}
	}
}

// TestLiveTranscript reads the caption list and one track. The track list comes
// from ANDROID because WEB answers UNPLAYABLE for this video and a caption list
// nobody can fetch is not a caption list.
func TestLiveTranscript(t *testing.T) {
	c := liveClient(t)
	ctx := liveContext(t)

	resp, err := NewInnerTube(c).AndroidPlayer(ctx, liveVideo)
	skipRefusals(t, err)
	tracks := ParseCaptionTracks(resp, liveVideo)
	if len(tracks) == 0 {
		t.Skip("the video came back with no caption tracks, which is a refusal in all but name")
	}
	for _, track := range tracks {
		if track.LanguageCode == "" || track.BaseURL == "" {
			t.Errorf("track %q has no language or no url", track.Name)
		}
	}
}

// TestLiveCommentsNeedCookies is the tier 1 read. Without a session this address
// gets Restricted Mode and the refusal is the expected answer, which is why the
// test skips rather than fails when there are no cookies.
func TestLiveCommentsNeedCookies(t *testing.T) {
	c := liveTier1(t)
	var count int
	err := c.StreamComments(liveContext(t), liveVideo, CommentOptions{MaxPages: 1}, func(cm Comment) error {
		count++
		if cm.ID == "" || cm.TextDisplay == "" {
			t.Errorf("a comment came back with no id or no text: %+v", cm)
		}
		return nil
	})
	skipRefusals(t, err)
	if count == 0 {
		t.Error("a session was loaded and the read returned no comments and no refusal")
	}
}
