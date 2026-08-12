package srv3

import (
	"os"
	"strings"
	"testing"
)

// The fixtures are two real tracks off dQw4w9WgXcQ, cut short and otherwise
// untouched:
//
//	asr_rollup.srv3.xml  the auto track, with a head, a window, rollups and words
//	manual.srv3.xml      the human track, which has none of those
//
// Both were fetched from a baseUrl off the ANDROID player, which already ends in
// &fmt=srv3.

func load(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestParseAutoTrack(t *testing.T) {
	doc, err := Parse(load(t, "asr_rollup.srv3.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Format != "3" {
		t.Errorf("Format = %q, want 3", doc.Format)
	}
	// 21 body nodes: one window, ten rollups and ten cues.
	if len(doc.Cues) != 10 {
		t.Errorf("got %d cues, want 10", len(doc.Cues))
	}
	if doc.Rollups != 10 {
		t.Errorf("Rollups = %d, want 10: an a=\"1\" node is the window redrawn, not a line", doc.Rollups)
	}
	// The window sits in the body next to the cues, so a reader that takes every
	// body child for a line counts it as a cue with no text.
	if len(doc.Windows) != 1 {
		t.Errorf("got %d windows, want 1", len(doc.Windows))
	}
	if len(doc.WindowStyles) != 2 || len(doc.WindowPositions) != 2 {
		t.Errorf("head = %d styles and %d positions, want 2 and 2", len(doc.WindowStyles), len(doc.WindowPositions))
	}
	if doc.WindowStyles[1].ScrollDir != 3 {
		t.Errorf("ws sd = %d, want 3", doc.WindowStyles[1].ScrollDir)
	}
	if doc.WindowPositions[1].Columns != 40 {
		t.Errorf("wp cc = %d, want 40", doc.WindowPositions[1].Columns)
	}

	if doc.Cues[0].Text != "[Music]" {
		t.Errorf("cue 0 = %q", doc.Cues[0].Text)
	}
	// The entities are unescaped exactly once. srv3 writes "&#39;" where the
	// older fmt-less response writes "&amp;#39;", so a reader tuned to that one
	// ships the entity.
	if doc.Cues[1].Text != "We're no strangers to" {
		t.Errorf("cue 1 = %q, want the apostrophe unescaped once", doc.Cues[1].Text)
	}
	words := doc.Cues[1].Words
	if len(words) != 4 {
		t.Fatalf("cue 1 has %d words, want 4", len(words))
	}
	if words[0].OffsetMS != 0 || words[1].OffsetMS != 239 {
		t.Errorf("word offsets = %d %d, want 0 and 239", words[0].OffsetMS, words[1].OffsetMS)
	}
	// The first word states no offset at all, so 0 has to be the parsed value and
	// not the absent one.
	if words[0].StartMS(doc.Cues[1]) != 18800 {
		t.Errorf("word 0 starts at %d, want the cue's own start", words[0].StartMS(doc.Cues[1]))
	}
	// Confidence is 0 on every word here, which is a value and not a gap, so it
	// has to survive the round trip.
	if words[0].ASRConfidence == nil || *words[0].ASRConfidence != 0 {
		t.Errorf("word 0 confidence = %v, want a stated 0", words[0].ASRConfidence)
	}
}

func TestParseManualTrack(t *testing.T) {
	doc, err := Parse(load(t, "manual.srv3.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Cues) != 10 {
		t.Errorf("got %d cues, want 10", len(doc.Cues))
	}
	// A human track has no head, no window, no rollup and no word timing. Telling
	// the two apart is what those counts are for.
	if doc.Rollups != 0 || len(doc.Windows) != 0 || len(doc.WindowStyles) != 0 {
		t.Errorf("a human track carries none of the rollup machinery: %d rollups, %d windows, %d styles",
			doc.Rollups, len(doc.Windows), len(doc.WindowStyles))
	}
	for _, c := range doc.Cues {
		if len(c.Words) != 0 {
			t.Errorf("cue %q claims word timings, which only an auto track states", c.Text)
		}
	}
	// The caption was drawn in a narrow box and broke its own lines to fit. A
	// transcript is prose, so the break is not part of the text.
	if got := doc.Cues[2].Text; got != "♪ You know the rules and so do I ♪" {
		t.Errorf("cue 2 = %q, want the caption's line break collapsed", got)
	}
}

// The endpoint answers a request it will not serve with 200 and an empty body,
// which is a refusal. The watch page's own baseUrl does it every single time.
func TestParseEmptyIsRefusal(t *testing.T) {
	if _, err := Parse(nil); err != ErrEmpty {
		t.Errorf("Parse(nil) = %v, want ErrEmpty", err)
	}
	if _, err := Parse([]byte("  \n ")); err != ErrEmpty {
		t.Errorf("Parse(whitespace) = %v, want ErrEmpty", err)
	}
}

// The stated durations on an auto track overlap by design, because a rollup line
// stays on screen while the next is typed under it. Written straight into an srt
// that is two subtitles drawn on top of each other.
func TestSubtitleEndsDoNotOverlap(t *testing.T) {
	doc, err := Parse(load(t, "asr_rollup.srv3.xml"))
	if err != nil {
		t.Fatal(err)
	}
	overlapping := 0
	for i, c := range doc.Cues[:len(doc.Cues)-1] {
		if c.EndMS() > doc.Cues[i+1].StartMS {
			overlapping++
		}
	}
	if overlapping == 0 {
		t.Fatal("the fixture is supposed to have overlapping stated durations")
	}
	lines := doc.timed()
	for i := range lines[:len(lines)-1] {
		if lines[i].EndMS > lines[i+1].StartMS {
			t.Errorf("line %d ends at %d and line %d starts at %d", i, lines[i].EndMS, i+1, lines[i+1].StartMS)
		}
		if lines[i].EndMS <= lines[i].StartMS {
			t.Errorf("line %d has no duration: %d to %d", i, lines[i].StartMS, lines[i].EndMS)
		}
	}
}

func TestRenderFormats(t *testing.T) {
	doc, err := Parse(load(t, "manual.srv3.xml"))
	if err != nil {
		t.Fatal(err)
	}

	text, err := doc.Render(FormatText)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(strings.TrimSpace(text), "\n") + 1; got != 10 {
		t.Errorf("text has %d lines, want one per cue", got)
	}

	srt, err := doc.Render(FormatSRT)
	if err != nil {
		t.Fatal(err)
	}
	// The first cue is at t=1360 d=1680 and the next at 18640, so nothing clamps
	// and the stated end stands.
	if !strings.HasPrefix(srt, "1\n00:00:01,360 --> 00:00:03,040\n[♪♪♪]\n") {
		t.Errorf("srt starts:\n%s", srt[:80])
	}
	if strings.Count(srt, "-->") != 10 {
		t.Errorf("srt has %d cues, want 10", strings.Count(srt, "-->"))
	}

	vtt, err := doc.Render(FormatVTT)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(vtt, "WEBVTT\n\n00:00:01.360 --> 00:00:03.040\n") {
		t.Errorf("vtt starts:\n%s", vtt[:80])
	}
	// srt and vtt differ in the decimal separator and the header, and in nothing
	// else, because they are written off the same resolved lines.
	if strings.Count(vtt, "-->") != strings.Count(srt, "-->") {
		t.Error("srt and vtt disagree about how many cues there are")
	}

	out, err := doc.Render(FormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"start_ms": 1360`) {
		t.Error("json should carry the millisecond timings the file states")
	}

	if _, err := doc.Render("ass"); err == nil {
		t.Error("an unknown format should be an error and not a silent default")
	}
}

func TestStamp(t *testing.T) {
	for _, tc := range []struct {
		ms   int
		want string
	}{
		{0, "00:00:00,000"},
		{1360, "00:00:01,360"},
		{3661001, "01:01:01,001"},
		{-5, "00:00:00,000"},
	} {
		if got := stamp(tc.ms, ','); got != tc.want {
			t.Errorf("stamp(%d) = %q, want %q", tc.ms, got, tc.want)
		}
	}
}
