package youtube

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// fixtures_test.go is the awkward shapes of doc 06 section 5: the responses that
// are a valid answer and look like a broken one.
//
// Each of these was a live read once, and each of them is a case where the shape
// changed under a parser that had only ever seen the ordinary one. They are here
// as committed bytes because the point is that the parser keeps reading them
// after nobody remembers why they were odd.

// A shorts playlist has none of the renderers a playlist is made of. The 83 rows
// of UUSH16niRr50-MSBwiO3YDb3RA are shortsLockupViewModel, there is not one
// lockupViewModel or playlistVideoRenderer in the response, and a parser that
// looks for either reports an empty playlist on a channel with hundreds of them.
func TestShortsPlaylistHasNoOrdinaryRenderers(t *testing.T) {
	raw := readFixtureBytes(t, "playlist_shorts_uush.json")
	for _, absent := range []string{`"lockupViewModel"`, `"playlistVideoRenderer"`, `"gridVideoRenderer"`} {
		if strings.Contains(string(raw), absent) {
			t.Fatalf("the fixture carries %s, so it no longer stands for the shorts-only case", absent)
		}
	}

	items, token := ParsePlaylistItems(loadFixture(t, "playlist_shorts_uush.json"), 0)
	if len(items) != 83 {
		t.Fatalf("parsed %d rows out of a page of 83 shorts", len(items))
	}
	if token != "" {
		t.Errorf("the page ends without a continuation and should hand back no token, got %q", token)
	}
	for i, v := range items {
		if len(v.VideoID) != 11 {
			t.Errorf("row %d has id %q, which is not a video id", i, v.VideoID)
		}
		if v.Title == "" {
			t.Errorf("row %d (%s) has no title", i, v.VideoID)
		}
		if v.Position != i+1 {
			t.Errorf("row %d is at position %d", i, v.Position)
		}
		// A shorts lockup states the views and nothing else. No duration, no
		// channel, no published date, and that is the shape rather than a miss.
		if v.ViewCount == 0 && v.ViewCountText == "" {
			t.Errorf("row %d (%s) has no view count, which is the one number a shorts lockup carries", i, v.VideoID)
		}
	}
}

// A continuation is never at the top of a response. In a real search it is six
// levels down, next to the results rather than beside them, and the synthetic
// shapes in continuation_test.go are all one level deep by construction. This is
// the same finder run over bytes YouTube actually sent.
func TestContinuationIsFoundWhereYouTubeBuriesIt(t *testing.T) {
	root := loadFixture(t, "search_plain.json")
	// Where it really is, so the test fails loudly if a later capture flattens it
	// and the finder stops being exercised at depth.
	deep := mapValue(mapValue(mapValue(root, "contents"), "twoColumnSearchResultsRenderer"), "primaryContents")
	if mapValue(deep, "sectionListRenderer") == nil {
		t.Fatal("the fixture no longer nests its results, so it no longer stands for the case")
	}
	token := FindContinuationToken(root)
	if token == "" {
		t.Fatal("no continuation token, so a search would stop after one page of twenty")
	}
	// Search hides a second token in the filter chips, and taking that one pages
	// through a filter nobody asked for. Doc 02 section 4.
	if chip := FindContinuationToken(mapValue(root, "header")); chip != "" && chip == token {
		t.Error("the token came off a filter chip rather than off the result list")
	}
}

// The watch page's own player response lists 27 formats and none of them can be
// fetched. Every adaptive entry carries a contentLength and no url, and the one
// muxed entry carries a signatureCipher that only YouTube's player JS can undo.
//
// This is the shape doc 01 section 6 is about, and it is why the format list is
// read from the mobile player and says so when it could not be. A list that looks
// complete and downloads nothing is worse than an empty one.
func TestWatchPlayerFormatsHaveNoURLs(t *testing.T) {
	pr := loadFixture(t, "player_web_nourl.json")
	sd := mapValue(pr, "streamingData")
	if streamingHasURLs(sd) {
		t.Fatal("a format in the fixture carries a url, so it no longer stands for the case")
	}

	formats := ParseVideoFormats(pr, "dQw4w9WgXcQ")
	if len(formats) < 20 {
		t.Fatalf("parsed %d formats out of 27", len(formats))
	}
	var sized, muxed int
	for _, f := range formats {
		if f.ITag == 0 {
			t.Errorf("a format came back with no itag: %+v", f)
		}
		if f.URL != "" {
			t.Errorf("itag %d has a url, and nothing in this response has one", f.ITag)
		}
		if f.ContentLength > 0 {
			sized++
		}
		if f.MediaKind == "muxed" {
			muxed++
		}
	}
	// The sizes are the reason to read this response at all: they are real, they
	// are exact, and the mobile player does not state them for the muxed itag.
	if sized < 20 {
		t.Errorf("only %d formats state a contentLength, and the adaptive ones all do", sized)
	}
	if muxed != 1 {
		t.Errorf("found %d muxed formats, want the one itag 18", muxed)
	}

	// The streams view of the same response, which is what a download would use.
	// The 26 adaptive formats are dropped on the way in because there is no way to
	// fetch them, and the one that survives is the cipher, which is refused by
	// name when someone asks for its URL.
	streams := parseStreamsWithUserAgent(pr, "")
	if len(streams) != 1 {
		t.Fatalf("got %d streams, want only the muxed itag 18 that carries a cipher", len(streams))
	}
	if streams[0].ITag != 18 || !streams[0].Muxed() {
		t.Errorf("the surviving stream is itag %d, muxed %v", streams[0].ITag, streams[0].Muxed())
	}
	url, err := (&Client{}).ResolveStreamURL(t.Context(), nil, &streams[0])
	if err == nil {
		t.Errorf("itag 18 resolved to %q, and undoing that cipher needs YouTube's player JS", url)
	} else if !strings.Contains(err.Error(), "signatureCipher") {
		t.Errorf("the refusal does not say what is wrong: %v", err)
	}
}

// A mix browse is the one refusal that arrives as a 200 with a body. Four keys,
// no contents, and one sentence. Doc 01 section 2.3.
func TestMixBrowseRefuses(t *testing.T) {
	data := loadFixture(t, "mix_unviewable.json")
	if _, ok := data["contents"]; ok {
		t.Fatal("the fixture has contents, so it is no longer the refusal")
	}
	r := alertRefusal(data, "RDdQw4w9WgXcQ", "browse")
	if r == nil {
		t.Fatal("the mix refusal was not found, so a mix would be reported as an empty playlist")
	}
	if !IsRefusal(r) {
		t.Error("the result is not a refusal, so the read would be retried")
	}
	if !strings.Contains(r.Message, "This playlist type is unviewable") {
		t.Errorf("the refusal does not quote YouTube: %q", r.Message)
	}
}

// The watch page's schema.org block, which is the free second opinion doc 01
// section 1.2 is about. Both counts are exact in here and both are compared
// against the payload rather than replacing it.
func TestWatchPageMicrodata(t *testing.T) {
	md := ParseVideoMicrodata(string(readFixtureBytes(t, "video_microdata.html")))
	if md == nil {
		t.Fatal("no VideoObject found in the block")
	}
	if md.Identifier != "dQw4w9WgXcQ" {
		t.Errorf("identifier = %q", md.Identifier)
	}
	// The tie the payload cannot break on its own: videoDetails says 213 and
	// microformat says 214 for this video.
	if md.Duration != "PT3M34S" || md.DurationSeconds != 214 {
		t.Errorf("duration = %q / %d, want PT3M34S / 214", md.Duration, md.DurationSeconds)
	}
	if md.ViewCount < 1_000_000_000 {
		t.Errorf("view count = %d, and this video passed a billion in 2021", md.ViewCount)
	}
	if md.LikeCount == 0 {
		t.Error("the LikeAction counter was not read")
	}
	// The page states the author url over plain http on a page served over TLS.
	if !strings.HasPrefix(md.AuthorURL, "https://") {
		t.Errorf("author url was not normalised: %q", md.AuthorURL)
	}
	if md.AuthorName == "" || len(md.Breadcrumb) == 0 {
		t.Errorf("author = %q, breadcrumb = %v", md.AuthorName, md.Breadcrumb)
	}
	if md.IsFamilyFriendly == nil || md.RequiresSubscription == nil {
		t.Error("the two booleans are spelled true and False in the same block and both have to parse")
	}
}

// TestWatchMicrodataCensus walks every itemprop in the block against
// microdataVideoFields, so an itemprop YouTube adds shows up as a failing test
// rather than as data quietly dropped on the floor.
func TestWatchMicrodataCensus(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(readFixtureBytes(t, "video_microdata.html"))))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	seen := map[string]bool{}
	doc.Find("[itemprop]").Each(func(_ int, s *goquery.Selection) {
		name, _ := s.Attr("itemprop")
		seen[name] = true
	})
	if len(seen) < 20 {
		t.Fatalf("only %d distinct itemprops in the fixture, so it is not the whole block", len(seen))
	}
	for name := range seen {
		if _, known := microdataVideoFields[name]; !known {
			t.Errorf("itemprop %q is on the page and in no census entry, so it is being dropped", name)
		}
	}
	// The other direction only holds for the entries that name a field. The
	// ignored ones are spellings other videos carry, unlisted and paid and
	// channelId among them, and dQw4w9WgXcQ is none of those things.
	for name, note := range microdataVideoFields {
		if !seen[name] && !strings.HasPrefix(note, "ignored:") {
			t.Errorf("the census reads itemprop %q into %s, and this page does not carry it", name, note)
		}
	}
}

// TestCaptureManifestCoversEveryFixture keeps testdata/capture.txt honest.
//
// The manifest is the only record of where a fixture came from, and a record
// nothing checks drifts within a month: a fixture gets added and not written
// down, or one gets deleted and its row stays, and either way the next person to
// re-capture one has to guess. Doc 06 section 6.
func TestCaptureManifestCoversEveryFixture(t *testing.T) {
	rows := map[string]bool{}
	for _, line := range strings.Split(string(readFixtureBytes(t, "capture.txt")), "\n") {
		// A row is a line with the four fields. The prose above the table carries no
		// pipes and the rule under the heading is all dashes.
		if strings.Count(line, "|") != 3 || strings.HasPrefix(line, "---") {
			continue
		}
		fields := strings.Split(line, "|")
		name := strings.TrimSpace(fields[0])
		if name == "file" {
			continue
		}
		for i, field := range fields {
			if strings.TrimSpace(field) == "" {
				t.Errorf("%s: column %d is empty, and a row with a blank column records nothing", name, i+1)
			}
		}
		if rows[name] {
			t.Errorf("%s has two rows", name)
		}
		rows[name] = true
	}
	if len(rows) == 0 {
		t.Fatal("the manifest parsed to no rows at all, so this test would pass against an empty file")
	}

	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	files := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "capture.txt" {
			continue
		}
		files[entry.Name()] = true
		if !rows[entry.Name()] {
			t.Errorf("%s has no row in capture.txt, so nobody can re-capture it when the shape changes", entry.Name())
		}
	}
	for name := range rows {
		if !files[name] {
			t.Errorf("capture.txt has a row for %s, which is not in testdata", name)
		}
	}
}

func readFixtureBytes(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return raw
}
