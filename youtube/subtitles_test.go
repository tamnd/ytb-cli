package youtube

import (
	"strings"
	"testing"
)

func TestRenderSubtitles(t *testing.T) {
	segs := []TranscriptSegment{
		{StartSeconds: 0, DurSeconds: 1.5, Text: "Hello world"},
		{StartSeconds: 1.5, DurSeconds: 2, Text: "second line"},
		{StartSeconds: 3661.250, DurSeconds: 1, Text: "after an hour"},
	}

	srt := RenderSubtitles(segs, SubSRT)
	if !strings.Contains(srt, "1\n00:00:00,000 --> 00:00:01,500\nHello world") {
		t.Errorf("srt first cue wrong:\n%s", srt)
	}
	if !strings.Contains(srt, "01:01:01,250 --> 01:01:02,250") {
		t.Errorf("srt hour timestamp wrong:\n%s", srt)
	}

	vtt := RenderSubtitles(segs, SubVTT)
	if !strings.HasPrefix(vtt, "WEBVTT\n\n") {
		t.Errorf("vtt missing header:\n%s", vtt)
	}
	if !strings.Contains(vtt, "00:00:00.000 --> 00:00:01.500") {
		t.Errorf("vtt cue wrong:\n%s", vtt)
	}

	txt := RenderSubtitles(segs, SubText)
	if txt != "Hello world\nsecond line\nafter an hour\n" {
		t.Errorf("txt wrong: %q", txt)
	}
}
