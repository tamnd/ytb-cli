package youtube

import "testing"

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
	if v.ChannelName != "" {
		t.Errorf("ChannelName = %q, want empty (no owner part present)", v.ChannelName)
	}
}

// TestParseLockupViewModelFullCounts verifies the HTML-page lockup format, where
// the view count and relative time carry their unit words.
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
								map[string]any{"text": map[string]any{"content": "Some Channel"}},
							}},
							map[string]any{"metadataParts": []any{
								map[string]any{"text": map[string]any{"content": "1.2M views"}},
								map[string]any{"text": map[string]any{"content": "3 months ago"}},
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
	if v.ChannelName != "Some Channel" {
		t.Errorf("ChannelName = %q, want %q", v.ChannelName, "Some Channel")
	}
}
