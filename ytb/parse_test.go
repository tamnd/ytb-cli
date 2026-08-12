package ytb

import (
	"strings"
	"testing"
)

func TestLooksLikeCount(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"101K", true},
		{"1.2M", true},
		{"1.2B", true},
		{"423", true},
		{"1,234", true},
		{"5K ", true},
		{"Rick Astley", false},
		{"101K views", false},
		{"1mo ago", false},
		{"Playlist", false},
		{"", false},
		{"4K Remaster", false},
	}
	for _, c := range cases {
		if got := looksLikeCount(c.in); got != c.want {
			t.Errorf("looksLikeCount(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestIsRelativeTimeText(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"1 month ago", true},
		{"1mo ago", true},
		{"Streamed 2 days ago", true},
		{"Premiered 5 hours ago", true},
		{"Scheduled for later", true},
		{"101K", false},
		{"Rick Astley", false},
	}
	for _, c := range cases {
		if got := isRelativeTimeText(c.in); got != c.want {
			t.Errorf("isRelativeTimeText(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestFormatDurationClock(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, ""},
		{-5, ""},
		{59, "0:59"},
		{213, "3:33"},
		{3600, "1:00:00"},
		{3661, "1:01:01"},
		{36000, "10:00:00"},
	}
	for _, c := range cases {
		if got := formatDurationClock(c.in); got != c.want {
			t.Errorf("formatDurationClock(%d) = %q, want %q", c.in, got, c.want)
		}
	}
	// formatDurationClock should round-trip through parseDurationSeconds.
	for _, sec := range []int{59, 213, 3661, 36000} {
		if got := parseDurationSeconds(formatDurationClock(sec)); got != sec {
			t.Errorf("round-trip %d: parseDurationSeconds(%q) = %d", sec, formatDurationClock(sec), got)
		}
	}
}

// TestParseLockupViewModelCompactCounts verifies the /browse continuation lockup
// format, where a bare "101K" carries the view count with no "views" word and must
// not be mistaken for the channel name.
func TestParseLockupViewModelCompactCounts(t *testing.T) {
	r := map[string]any{
		"contentType": "LOCKUP_CONTENT_TYPE_VIDEO",
		"contentId":   "LjmOX9jGoR4",
		"metadata": map[string]any{
			"lockupMetadataViewModel": map[string]any{
				"title": map[string]any{"content": "Rick Astley - Raindrops"},
				"metadata": map[string]any{
					"contentMetadataViewModel": map[string]any{
						"metadataRows": []any{
							map[string]any{"metadataParts": []any{
								map[string]any{"text": map[string]any{"content": "101K"}},
								map[string]any{"text": map[string]any{"content": "1mo ago"}},
							}},
						},
					},
				},
			},
		},
	}
	v := parseLockupViewModel(r)
	if v.VideoID != "LjmOX9jGoR4" {
		t.Fatalf("VideoID = %q", v.VideoID)
	}
	if v.ViewCount != 101000 {
		t.Errorf("ViewCount = %d, want 101000", v.ViewCount)
	}
	if v.PublishedText != "1mo ago" {
		t.Errorf("PublishedText = %q, want %q", v.PublishedText, "1mo ago")
	}
	if v.ChannelTitle != "" {
		t.Errorf("ChannelTitle = %q, want empty (no owner part present)", v.ChannelTitle)
	}
}

// TestParseLockupViewModelFullCounts verifies the HTML-page lockup format, where
// the view count and relative time carry their unit words.
//
// The owner part carries a commandRun through to the channel's browseEndpoint,
// which is what identifies it as the owner. That is not a hopeful reading of the
// shape: a census over 299 video lockups on five live responses, from a channel's
// uploads, three playlists and a search, found the run on every single one. So the
// channel name is read from its link and never from "the first fragment nothing
// else matched".
func TestParseLockupViewModelFullCounts(t *testing.T) {
	r := map[string]any{
		"contentType": "LOCKUP_CONTENT_TYPE_VIDEO",
		"contentId":   "abc12345678",
		"metadata": map[string]any{
			"lockupMetadataViewModel": map[string]any{
				"metadata": map[string]any{
					"contentMetadataViewModel": map[string]any{
						"metadataRows": []any{
							map[string]any{"metadataParts": []any{
								map[string]any{"text": map[string]any{
									"content": "Some Channel",
									"commandRuns": []any{map[string]any{
										"startIndex": 0.0,
										"length":     12.0,
										"onTap": map[string]any{"innertubeCommand": map[string]any{
											"browseEndpoint": map[string]any{"browseId": "UCuAXFkgsw1L7xaCfnd5JJOw"},
										}},
									}},
								}},
							}},
							map[string]any{"metadataParts": []any{
								map[string]any{"text": map[string]any{"content": "1.2M views"}},
								map[string]any{"text": map[string]any{"content": "3 months ago"}},
								map[string]any{"text": map[string]any{"content": "Something YouTube has not shipped yet"}},
							}},
						},
					},
				},
			},
		},
	}
	v := parseLockupViewModel(r)
	if v.ViewCount != 1200000 {
		t.Errorf("ViewCount = %d, want 1200000", v.ViewCount)
	}
	if v.PublishedText != "3 months ago" {
		t.Errorf("PublishedText = %q", v.PublishedText)
	}
	if v.ChannelTitle != "Some Channel" {
		t.Errorf("ChannelTitle = %q, want %q", v.ChannelTitle, "Some Channel")
	}
	if v.ChannelID != "UCuAXFkgsw1L7xaCfnd5JJOw" {
		t.Errorf("ChannelID = %q, want the id off the owner's own tap command", v.ChannelID)
	}
	// The point of the catch-all: a fragment this parser does not recognise is kept
	// verbatim rather than dropped, so a shift in what YouTube renders shows up in
	// the output as an unclassified string instead of silently going missing.
	if len(v.MetadataParts) != 1 || v.MetadataParts[0] != "Something YouTube has not shipped yet" {
		t.Errorf("MetadataParts = %q, want the unrecognised fragment kept", v.MetadataParts)
	}
}

// A shorts lockup is the same view model on two pages and it is not the same
// object. On the Shorts tab the entityId is shorts-shelf-item-<id> and the
// overlay states "22K views"; in the UUSH shorts playlist the entityId is the
// opaque hash A4C99DA633F4D7CD and the overlay has no view count at all. Both
// forms are measured below, because a reader that only knows the first returns
// an empty playlist for a channel with 294 shorts in it.
func TestParseShortsLockupViewModelBothPages(t *testing.T) {
	onTap := map[string]any{
		"innertubeCommand": map[string]any{
			"commandMetadata":   map[string]any{"webCommandMetadata": map[string]any{"url": "/shorts/GdbjNGtWPe4"}},
			"reelWatchEndpoint": map[string]any{"videoId": "GdbjNGtWPe4"},
		},
	}
	thumb := map[string]any{"thumbnailViewModel": map[string]any{
		"image": map[string]any{"sources": []any{
			map[string]any{"url": "https://i.ytimg.com/vi/GdbjNGtWPe4/oar2.jpg", "width": float64(405), "height": float64(720)},
		}},
	}}
	a11y := "39 years of Never Gonna Give You Up, 22 thousand views - play Short"

	tab := parseShortsLockupViewModel(map[string]any{
		"entityId":           "shorts-shelf-item-GdbjNGtWPe4",
		"onTap":              onTap,
		"accessibilityText":  a11y,
		"thumbnailViewModel": thumb,
		"overlayMetadata": map[string]any{
			"primaryText":   map[string]any{"content": "39 years of Never Gonna Give You Up"},
			"secondaryText": map[string]any{"content": "22K views"},
		},
	})
	playlist := parseShortsLockupViewModel(map[string]any{
		"entityId":           "A4C99DA633F4D7CD",
		"onTap":              onTap,
		"accessibilityText":  a11y,
		"thumbnailViewModel": thumb,
		"overlayMetadata": map[string]any{
			"primaryText": map[string]any{"content": "39 years of Never Gonna Give You Up"},
		},
	})

	for label, v := range map[string]Video{"shorts tab": tab, "shorts playlist": playlist} {
		if v.VideoID != "GdbjNGtWPe4" {
			t.Errorf("%s: VideoID = %q, want GdbjNGtWPe4", label, v.VideoID)
		}
		if v.IsShort == nil || !*v.IsShort {
			t.Errorf("%s: nothing but a short is rendered as a shorts lockup", label)
		}
		if v.ViewCount != 22000 {
			t.Errorf("%s: ViewCount = %d, want 22000", label, v.ViewCount)
		}
		if len(v.Thumbnails) == 0 {
			t.Errorf("%s: no thumbnails, the sources live under thumbnailViewModel", label)
		}
		if v.Via["view_count"] == "" {
			t.Errorf("%s: via should name where the count came from", label)
		}
	}
	// The two pages state the count differently and via has to say which was read.
	if tab.Via["view_count"] == playlist.Via["view_count"] {
		t.Errorf("both pages claim the same source: %q", tab.Via["view_count"])
	}
}

// The accessibility label spells the scale as a word where the visible text uses
// the K/M suffix.
func TestA11yViewCount(t *testing.T) {
	cases := map[string]int64{
		"Title, 22 thousand views - play Short": 22_000,
		"Title, 1.4 million views - play Short": 1_400_000,
		"Title, 2 billion views - play Short":   2_000_000_000,
		"Title, 934 views - play Short":         934,
		"Title, 1,234 views - play Short":       1_234,
	}
	for label, want := range cases {
		if _, got := a11yViewCount(label); got != want {
			t.Errorf("a11yViewCount(%q) = %d, want %d", label, got, want)
		}
	}
	if _, got := a11yViewCount("no numbers here"); got != 0 {
		t.Errorf("a label with no count should give 0, got %d", got)
	}
}

// extractLines is for a body whose line breaks are part of it. extractText is
// for a title in a table cell and flattens one, which turns a verse into a
// paragraph.
func TestExtractLines(t *testing.T) {
	v := map[string]any{"runs": []any{
		map[string]any{"text": "first line\nsecond line\n\nafter a blank\n"},
	}}
	want := "first line\nsecond line\n\nafter a blank"
	if got := extractLines(v); got != want {
		t.Errorf("extractLines = %q, want %q", got, want)
	}
	if got := extractText(v); strings.Contains(got, "\n") {
		t.Errorf("extractText kept a line break: %q", got)
	}
}
