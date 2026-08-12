package youtube

import (
	"testing"
	"time"
)

// videoparse_test.go checks the video read against three real captures rather
// than against handwritten JSON, because every defect worth catching here came
// from the site doing something a handwritten fixture would never do.
//
// The three: dQw4w9WgXcQ, a normal upload with keywords, a like count and 249
// available countries; Df5Y-2ndQyU, a short, which is a short because
// canonicalUrl says so; 201MyYKtXaQ, a finished eleven hour stream, which is the
// only one of the three carrying liveBroadcastDetails and the latency keys.
var playerCaptures = []string{
	"video_player.json",
	"video_player_short.json",
	"video_player_stream.json",
}

// TestPlayerResponseCensus is the test the two maps in videoparse.go exist for. A
// key YouTube adds to videoDetails or microformat fails here, which is the only way
// a new field gets noticed: a parser that reads eight of sixteen keys looks exactly
// like one that read all sixteen.
func TestPlayerResponseCensus(t *testing.T) {
	for _, name := range playerCaptures {
		pr := loadFixture(t, name)
		details := mapValue(pr, "videoDetails")
		if len(details) == 0 {
			t.Fatalf("%s: no videoDetails", name)
		}
		for key := range details {
			if _, ok := videoDetailsFields[key]; !ok {
				t.Errorf("%s: videoDetails.%s is not in videoDetailsFields, so nothing decided whether to read it", name, key)
			}
		}
		micro := mapValue(mapValue(pr, "microformat"), "playerMicroformatRenderer")
		if len(micro) == 0 {
			t.Fatalf("%s: no playerMicroformatRenderer", name)
		}
		for key := range micro {
			if _, ok := microformatFields[key]; !ok {
				t.Errorf("%s: microformat.%s is not in microformatFields", name, key)
			}
		}
	}

	// A key mapped to "" is a key somebody added to the census and then forgot to
	// say anything about, which is worse than a key that is missing.
	for _, m := range []map[string]string{videoDetailsFields, microformatFields} {
		for key, dest := range m {
			if dest == "" {
				t.Errorf("census key %s has no destination and no reason for being dropped", key)
			}
		}
	}
}

// TestParsePlayerResponse reads the normal upload and checks the fields that took
// a decision to get right.
func TestParsePlayerResponse(t *testing.T) {
	v := NewVideo("dQw4w9WgXcQ", SurfaceWatchHTML)
	ParsePlayerResponse(loadFixture(t, "video_player.json"), v, SurfaceWatchHTML)

	if v.Title == "" {
		t.Fatal("no title")
	}
	// microformat says 214 where videoDetails on the same page says 213, and the
	// page's own microdata says PT3M34S, which is 214. Via has to name the winner,
	// because "s1" alone would not say which of the two blocks answered.
	if v.DurationSeconds != 214 {
		t.Errorf("duration_seconds = %d, want 214 from microformat", v.DurationSeconds)
	}
	if got := v.Via["duration_seconds"]; got != "s1 microformat.lengthSeconds" {
		t.Errorf("via[duration_seconds] = %q", got)
	}
	if got := len(v.Keywords); got != 27 {
		t.Errorf("keywords = %d, want the uploader's 27", got)
	}
	if got := len(v.AvailableCountries); got != 249 {
		t.Errorf("available_countries = %d, want 249 kept whole", got)
	}
	if v.LikeCount == 0 {
		t.Error("no like count, which microformat is the only tier 0 source of")
	}
	// ownerProfileUrl is spelled http:// where every other block on the page says
	// https, and two spellings of one channel URL is two nodes in a graph.
	if got := v.ChannelURL; got != "https://www.youtube.com/@RickAstleyYT" {
		t.Errorf("channel_url = %q, want the https spelling", got)
	}
	if got := v.ChannelHandle; got != "@RickAstleyYT" {
		t.Errorf("channel_handle = %q", got)
	}
	if flagTrue(v.IsShort) {
		t.Error("a nine minute music video is not a short")
	}
	if v.Playability == nil || v.Playability.Status != "OK" {
		t.Errorf("playability = %+v, want status OK", v.Playability)
	}
	// OK is a real false. Absent would mean nobody asked.
	if v.AgeRestricted == nil || *v.AgeRestricted {
		t.Errorf("age_restricted = %v, want a stated false", v.AgeRestricted)
	}
	if v.LiveState != "" {
		t.Errorf("live_state = %q on a normal upload", v.LiveState)
	}
}

// TestParsePlayerResponseShort asserts a short is a short because canonicalUrl says
// /shorts/, not because isShortsEligible is true. Both are on the payload and only
// one of them is an answer: the eligibility flag is true on the nine minute music
// video too.
func TestParsePlayerResponseShort(t *testing.T) {
	v := NewVideo("Df5Y-2ndQyU", SurfaceWatchHTML)
	ParsePlayerResponse(loadFixture(t, "video_player_short.json"), v, SurfaceWatchHTML)
	if !flagTrue(v.IsShort) {
		t.Errorf("is_short = %v for a /shorts/ canonical url %q", v.IsShort, v.CanonicalURL)
	}

	full := NewVideo("dQw4w9WgXcQ", SurfaceWatchHTML)
	ParsePlayerResponse(loadFixture(t, "video_player.json"), full, SurfaceWatchHTML)
	if flagTrue(full.IsShort) {
		t.Error("isShortsEligible leaked into is_short")
	}
}

// TestParsePlayerResponseStream asserts the live state comes from
// liveBroadcastDetails and not from isLiveContent, which stays true forever on a
// finished stream and would report an eleven hour recording from yesterday as
// currently live.
func TestParsePlayerResponseStream(t *testing.T) {
	v := NewVideo("201MyYKtXaQ", SurfaceWatchHTML)
	ParsePlayerResponse(loadFixture(t, "video_player_stream.json"), v, SurfaceWatchHTML)
	if !flagTrue(v.IsLiveContent) {
		t.Error("is_live_content should be true on a stream, past or present")
	}
	if v.LiveState != "ended" {
		t.Errorf("live_state = %q, want ended: isLiveNow is false and there is an endTimestamp", v.LiveState)
	}
}

// TestSurfacesString asserts the surface list prints as ids. kit renders a slice
// column as its length, which is right for keywords and exactly wrong here: "1" is
// not an answer to which surface answered, and it reads as surface 1.
func TestSurfacesString(t *testing.T) {
	if got := (Surfaces{"s1"}).String(); got != "s1" {
		t.Errorf("Surfaces{s1} = %q, want s1", got)
	}
	if got := (Surfaces{"s1", "s3"}).String(); got != "s1, s3" {
		t.Errorf("Surfaces{s1,s3} = %q", got)
	}
}

// TestRenditionNameRefusesDerivatives asserts only a plain .jpg gets a rendition
// name. The watch page lists hqdefault.jpg?sqp=... four times, at 168x94, 196x110,
// 246x138 and 336x188, none of which is hqdefault's 480x360, and it lists
// vi_webp/maxresdefault.webp at 1920x1080 next to vi/maxresdefault.jpg at 1280x720.
// Naming those makes a caller that filters on name == "hqdefault" get five
// different images.
func TestRenditionNameRefusesDerivatives(t *testing.T) {
	cases := map[string]string{
		"https://i.ytimg.com/vi/dQw4w9WgXcQ/maxresdefault.jpg":            "maxresdefault",
		"https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg":                "hqdefault",
		"https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg?sqp=abc&rs=def": "",
		"https://i.ytimg.com/vi_webp/dQw4w9WgXcQ/maxresdefault.webp":      "",
		"https://i.ytimg.com/vi/dQw4w9WgXcQ/hq720.jpg":                    "",
	}
	for url, want := range cases {
		if got := renditionName(url); got != want {
			t.Errorf("renditionName(%s) = %q, want %q", url, got, want)
		}
	}
}

// TestRunChapterTitleBothLayouts asserts the title is found on either side of the
// timestamp. 3blue1brown writes "0:00 Introduction example" and koi64's guitar mix
// writes "Eyes To The Sky - 0:00:00", and reading only the run after the timestamp
// gave the second a chapter list with every title blank.
func TestRunChapterTitleBothLayouts(t *testing.T) {
	leading := []TextRun{
		{Text: "chapters\n"},
		{Text: "0:00", Kind: RunTimestamp},
		{Text: " Introduction example\n"},
		{Text: "1:07", Kind: RunTimestamp, StartSeconds: 67},
		{Text: " Series preview\n"},
	}
	got := chaptersFromRuns(leading, "x")
	if len(got) != 2 || got[0].Title != "Introduction example" || got[1].Title != "Series preview" {
		t.Fatalf("timestamp first: %+v", got)
	}

	trailing := []TextRun{
		{Text: "Tracklist/Timestamps:\nEyes To The Sky - "},
		{Text: "0:00:00", Kind: RunTimestamp},
		{Text: "\nHit The Hay - "},
		{Text: "0:02:20", Kind: RunTimestamp, StartSeconds: 140},
		{Text: "\nDaydreaming - "},
		{Text: "0:05:10", Kind: RunTimestamp, StartSeconds: 310},
	}
	got = chaptersFromRuns(trailing, "x")
	if len(got) != 3 {
		t.Fatalf("title first: got %d chapters", len(got))
	}
	for i, want := range []string{"Eyes To The Sky", "Hit The Hay", "Daydreaming"} {
		if got[i].Title != want {
			t.Errorf("title first [%d] = %q, want %q", i, got[i].Title, want)
		}
		if got[i].Origin != ChapterFromDescription {
			t.Errorf("origin [%d] = %q", i, got[i].Origin)
		}
	}

	// One timestamp is a link to a moment, not a chapter list.
	if got := chaptersFromRuns([]TextRun{{Text: "watch the bit at "}, {Text: "2:10", Kind: RunTimestamp, StartSeconds: 130}}, "x"); got != nil {
		t.Errorf("one timestamp became %d chapters", len(got))
	}
}

// TestChaptersFromDescriptionBothLayouts covers the flat text last resort, which is
// what a /player-only read has: shortDescription and no runs at all.
func TestChaptersFromDescriptionBothLayouts(t *testing.T) {
	leading := "intro blurb\n0:00 Introduction\n1:07 Series preview\n2:42 What are neurons?"
	got := chaptersFromDescription(leading, "x")
	if len(got) != 3 || got[0].Title != "Introduction" || got[2].StartSeconds != 162 {
		t.Fatalf("timestamp first: %+v", got)
	}

	trailing := "Tracklist/Timestamps:\nEyes To The Sky - 0:00:00\nHit The Hay - 0:02:20\nDaydreaming - 0:05:10"
	got = chaptersFromDescription(trailing, "x")
	if len(got) != 3 {
		t.Fatalf("title first: got %d chapters", len(got))
	}
	if got[1].Title != "Hit The Hay" || got[1].StartSeconds != 140 {
		t.Errorf("title first [1] = %q at %ds", got[1].Title, got[1].StartSeconds)
	}
}

// TestSortFormats asserts the list is in the same order twice. ParseVideoFormats
// deduplicates through a map, so before sorting the command printed the same 29
// formats in a different order on every run, which makes two outputs impossible to
// diff and looks like the site changed.
func TestSortFormats(t *testing.T) {
	formats := ParseVideoFormats(map[string]any{"streamingData": map[string]any{
		"formats": []any{
			map[string]any{"itag": 18.0, "mimeType": `video/mp4; codecs="avc1.42001E, mp4a.40.2"`, "bitrate": 444226.0},
		},
		"adaptiveFormats": []any{
			map[string]any{"itag": 137.0, "mimeType": `video/mp4; codecs="avc1.640028"`, "bitrate": 4334157.0},
			map[string]any{"itag": 251.0, "mimeType": `audio/webm; codecs="opus"`, "bitrate": 136544.0},
			map[string]any{"itag": 598.0, "mimeType": `video/webm; codecs="vp9"`, "bitrate": 24648.0},
			map[string]any{"itag": 140.0, "mimeType": `audio/mp4; codecs="mp4a.40.2"`, "bitrate": 130677.0},
		},
	}}, "x")
	sortFormats(formats)
	want := []int{140, 251, 598, 137, 18}
	for i, itag := range want {
		if formats[i].ITag != itag {
			t.Fatalf("order = %v, want audio then video then muxed: %v", itags(formats), want)
		}
	}
}

func itags(formats []VideoFormat) []int {
	out := make([]int, 0, len(formats))
	for _, f := range formats {
		out = append(out, f.ITag)
	}
	return out
}

// TestFormatDerivedFields asserts the derived fields, all of which read the mime
// type rather than an itag table.
func TestFormatDerivedFields(t *testing.T) {
	muxed := VideoFormat{
		ITag: 18, MimeType: `video/mp4; codecs="avc1.42001E, mp4a.40.2"`,
		MediaKind: "muxed", Container: "mp4", Codec: "avc1.42001E+mp4a.40.2",
		Quality: "medium", QualityLabel: "360p", AudioQuality: "AUDIO_QUALITY_LOW",
	}
	got := parseFormat(map[string]any{
		"itag":         18.0,
		"mimeType":     `video/mp4; codecs="avc1.42001E, mp4a.40.2"`,
		"quality":      "medium",
		"qualityLabel": "360p",
		"audioQuality": "AUDIO_QUALITY_LOW",
	}, "", false)
	if got == nil {
		t.Fatal("parseFormat returned nil for a real muxed format")
	}
	if got.MediaKind != muxed.MediaKind || got.Container != muxed.Container || got.Codec != muxed.Codec {
		t.Errorf("muxed: kind %q container %q codec %q", got.MediaKind, got.Container, got.Codec)
	}
	if q := got.QualityText(); q != "360p" {
		t.Errorf("muxed quality = %q, want the label rather than %q", q, got.Quality)
	}
	if !got.IsThrottledUnranged {
		t.Error("is_throttled_unranged is true on every format, including one with no url")
	}

	// An audio format's quality is "tiny" at every bitrate, so the answer is
	// audioQuality: itag 140 and itag 251 are both tiny and both medium.
	audio := parseFormat(map[string]any{
		"itag": 140.0, "mimeType": `audio/mp4; codecs="mp4a.40.2"`,
		"quality": "tiny", "audioQuality": "AUDIO_QUALITY_MEDIUM",
	}, "", true)
	if audio.MediaKind != "audio" {
		t.Errorf("audio kind = %q", audio.MediaKind)
	}
	if q := audio.QualityText(); q != "medium" {
		t.Errorf("audio quality = %q, want medium rather than tiny", q)
	}

	video := parseFormat(map[string]any{
		"itag": 313.0, "mimeType": `video/webm; codecs="vp9"`, "qualityLabel": "2160p",
	}, "", true)
	if video.MediaKind != "video" || video.Container != "webm" || video.Codec != "vp9" {
		t.Errorf("video: kind %q container %q codec %q", video.MediaKind, video.Container, video.Codec)
	}
}

// TestFormatQuotedNumbers asserts the integers YouTube spells as strings are read
// as numbers. contentLength, audioSampleRate, approxDurationMs and lastModified are
// all quoted in the response, so a plain float read gets zero from every one of
// them, and lastModified is microseconds: read as seconds it lands in 57 million AD.
func TestFormatQuotedNumbers(t *testing.T) {
	f := parseFormat(map[string]any{
		"itag":             140.0,
		"mimeType":         `audio/mp4; codecs="mp4a.40.2"`,
		"contentLength":    "3437753",
		"audioSampleRate":  "44100",
		"audioChannels":    2.0,
		"approxDurationMs": "213181",
		"averageBitrate":   129000.0,
		"lastModified":     "1766960953317159",
		"initRange":        map[string]any{"start": "0", "end": "631"},
		"indexRange":       map[string]any{"start": "632", "end": "1263"},
		"url":              "https://rr11---sn-abc.googlevideo.com/videoplayback?expire=1785409845&itag=140",
	}, "dQw4w9WgXcQ", true)
	if f.ContentLength != 3437753 {
		t.Errorf("content_length = %d", f.ContentLength)
	}
	if f.AudioSampleRate != 44100 || f.AudioChannels != 2 {
		t.Errorf("audio = %d Hz, %d channels", f.AudioSampleRate, f.AudioChannels)
	}
	if f.ApproxDurationMS != 213181 || f.AverageBitrate != 129000 {
		t.Errorf("duration %d ms, average bitrate %d", f.ApproxDurationMS, f.AverageBitrate)
	}
	if got := f.LastModified.UTC().Format("2006-01-02"); got != "2025-12-28" {
		t.Errorf("last_modified = %s (%s), want a date in 2025", got, f.LastModified)
	}
	if f.InitRange == nil || f.InitRange.End != 631 || f.IndexRange == nil || f.IndexRange.Start != 632 {
		t.Errorf("ranges = %+v, %+v", f.InitRange, f.IndexRange)
	}
	if f.ExpiresAt.Unix() != 1785409845 {
		t.Errorf("expires_at = %s, want the URL's own expire", f.ExpiresAt)
	}

	// A muxed format has no ranges at all, and absent has to read as absent rather
	// than as a range of zero to zero.
	muxed := parseFormat(map[string]any{"itag": 18.0, "mimeType": "video/mp4"}, "", false)
	if muxed.InitRange != nil || muxed.IndexRange != nil {
		t.Errorf("muxed ranges = %+v, %+v, want both absent", muxed.InitRange, muxed.IndexRange)
	}
	if !muxed.LastModified.IsZero() || !muxed.ExpiresAt.IsZero() {
		t.Error("no lastModified and no url means no times, not the epoch")
	}
}

// TestStreamExpiry asserts the deadline comes off the URL. The response carries
// both: expiresInSeconds said 21540 where the URL's expire was exactly 21600
// seconds out, and the CDN enforces the one in the URL.
func TestStreamExpiry(t *testing.T) {
	now := time.Unix(1785400000, 0)
	sd := map[string]any{
		"expiresInSeconds": "21540",
		"formats": []any{
			map[string]any{"itag": 18.0, "url": "https://rr11---sn-abc.googlevideo.com/videoplayback?expire=1785408784&ei=x"},
		},
	}
	if got := streamExpiry(sd, now).Unix(); got != 1785408784 {
		t.Errorf("expiry = %d, want the URL's 1785408784", got)
	}

	// With no URL anywhere, expiresInSeconds is all there is.
	noURL := map[string]any{"expiresInSeconds": "21540", "formats": []any{map[string]any{"itag": 18.0}}}
	if got := streamExpiry(noURL, now).Unix(); got != 1785400000+21540 {
		t.Errorf("fallback expiry = %d", got)
	}
	if !streamExpiry(map[string]any{}, now).IsZero() {
		t.Error("an empty streamingData should have no expiry rather than a made up one")
	}
	if streamingHasURLs(noURL) {
		t.Error("a format with no url is not fetchable")
	}
	if !streamingHasURLs(sd) {
		t.Error("a format with a url is fetchable")
	}
}

// TestUnsizedITags asserts the missing size is reported once for the whole read,
// because it is one fact about the client: ANDROID answers with no contentLength on
// a muxed format and with one on every adaptive format.
func TestUnsizedITags(t *testing.T) {
	formats := []VideoFormat{
		{ITag: 140, ContentLength: 3449447},
		{ITag: 18},
		{ITag: 22},
	}
	if got := unsizedITags(formats); got != "18, 22" {
		t.Errorf("unsizedITags = %q", got)
	}
	annotateFormats(formats)
	if formats[0].Note != "" || formats[1].Note != "size unknown" {
		t.Errorf("notes = %q, %q", formats[0].Note, formats[1].Note)
	}
}
