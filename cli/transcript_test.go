package cli

import (
	"strings"
	"testing"

	"github.com/tamnd/ytb-cli/youtube"
)

func TestVTTStamp(t *testing.T) {
	cases := map[string]float64{
		"00:00:00.000": 0,
		"00:00:05.500": 5.5,
		"00:01:00.000": 60,
		"01:00:00.000": 3600,
		"01:02:03.000": 3723,
		"02:03.000":    123, // MM:SS form
	}
	for in, want := range cases {
		if got := vttStamp(in); got != want {
			t.Errorf("vttStamp(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestVTTClean(t *testing.T) {
	cases := map[string]string{
		"hello world":                       "hello world",
		"hello <00:00:01.000><c> world</c>": "hello world",
		"  spaced   out  ":                  "spaced out",
		"<c>tagged</c>":                     "tagged",
	}
	for in, want := range cases {
		if got := vttClean(in); got != want {
			t.Errorf("vttClean(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseVTT(t *testing.T) {
	const vtt = `WEBVTT
Kind: captions
Language: en

00:00:00.000 --> 00:00:02.000
Hello there

00:00:02.000 --> 00:00:04.000
Hello there

00:00:04.000 --> 00:00:06.000
<c>general</c> kenobi
`
	segs := parseVTT(strings.NewReader(vtt))
	// The duplicate "Hello there" rollup cue is deduped, leaving two segments.
	if len(segs) != 2 {
		t.Fatalf("parseVTT returned %d segments, want 2: %+v", len(segs), segs)
	}
	if segs[0].Text != "Hello there" {
		t.Errorf("seg0 text = %q, want %q", segs[0].Text, "Hello there")
	}
	if segs[0].StartSeconds != 0 || segs[0].DurSeconds != 2 {
		t.Errorf("seg0 timing = (%v,%v), want (0,2)", segs[0].StartSeconds, segs[0].DurSeconds)
	}
	if segs[1].Text != "general kenobi" {
		t.Errorf("seg1 text = %q, want %q", segs[1].Text, "general kenobi")
	}
	if segs[1].StartSeconds != 4 {
		t.Errorf("seg1 start = %v, want 4", segs[1].StartSeconds)
	}
}

func TestJoinSegmentText(t *testing.T) {
	got := joinSegmentText([]youtube.TranscriptSegment{
		{Text: "first"},
		{Text: "  "},
		{Text: "second"},
	})
	const want = "first\nsecond"
	if got != want {
		t.Errorf("joinSegmentText = %q, want %q", got, want)
	}
}
