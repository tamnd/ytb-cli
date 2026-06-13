package youtube

import "testing"

func sampleStreams() []Stream {
	return []Stream{
		{ITag: 18, Container: "mp4", VideoCodec: "avc1", AudioCodec: "mp4a", Height: 360, Width: 640, FPS: 30, Bitrate: 500_000, HasVideo: true, HasAudio: true},
		{ITag: 22, Container: "mp4", VideoCodec: "avc1", AudioCodec: "mp4a", Height: 720, Width: 1280, FPS: 30, Bitrate: 2_000_000, HasVideo: true, HasAudio: true},
		{ITag: 137, Container: "mp4", VideoCodec: "avc1", Height: 1080, Width: 1920, FPS: 30, Bitrate: 4_000_000, HasVideo: true, IsAdaptive: true},
		{ITag: 248, Container: "webm", VideoCodec: "vp9", Height: 1080, Width: 1920, FPS: 30, Bitrate: 3_500_000, HasVideo: true, IsAdaptive: true},
		{ITag: 140, Container: "mp4", AudioCodec: "mp4a", Bitrate: 128_000, HasAudio: true, IsAdaptive: true},
		{ITag: 251, Container: "webm", AudioCodec: "opus", Bitrate: 160_000, HasAudio: true, IsAdaptive: true},
	}
}

func TestSelectFormat(t *testing.T) {
	streams := sampleStreams()
	cases := []struct {
		spec      string
		wantVideo int // itag, 0 = nil
		wantAudio int
	}{
		{"best", 22, 0},           // best progressive
		{"worst", 18, 0},          // worst overall (smallest)
		{"22", 22, 0},             // explicit itag
		{"bestvideo", 137, 0},     // highest video-only (avc 1080 > vp9 by bitrate tie? both 1080)
		{"bestaudio", 0, 251},     // highest audio-only by bitrate
		{"bv+ba", 137, 251},       // merge best video + best audio
		{"137+140", 137, 140},     // explicit merge
		{"bv[height<=720]", 0, 0}, // no video-only <=720 in set -> error path tested below
		{"bv*[height<=720]+ba", 22, 251},
		{"webm", 248, 0}, // webm video-only best (only video webm is 248)
	}
	for _, c := range cases {
		sel, err := SelectFormat(streams, c.spec)
		if c.spec == "bv[height<=720]" {
			if err == nil {
				t.Errorf("%q: expected error (no video-only <=720), got %+v", c.spec, sel)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: unexpected error: %v", c.spec, err)
			continue
		}
		gotV := itagOf(sel.Video)
		gotA := itagOf(sel.Audio)
		if gotV != c.wantVideo || gotA != c.wantAudio {
			t.Errorf("%q: got video=%d audio=%d, want video=%d audio=%d", c.spec, gotV, gotA, c.wantVideo, c.wantAudio)
		}
	}
}

func TestSelectFormatFallback(t *testing.T) {
	streams := sampleStreams()
	// First group has no match; second (b) does.
	sel, err := SelectFormat(streams, "999/b")
	if err != nil {
		t.Fatalf("fallback: %v", err)
	}
	if itagOf(sel.Video) != 22 {
		t.Fatalf("fallback: got %d, want 22", itagOf(sel.Video))
	}
}

func itagOf(s *Stream) int {
	if s == nil {
		return 0
	}
	return s.ITag
}

func TestParseMime(t *testing.T) {
	cases := []struct {
		mime                 string
		cont, vcodec, acodec string
	}{
		{`video/mp4; codecs="avc1.640028, mp4a.40.2"`, "mp4", "avc1.640028", "mp4a.40.2"},
		{`video/webm; codecs="vp9"`, "webm", "vp9", ""},
		{`audio/mp4; codecs="mp4a.40.2"`, "mp4", "", "mp4a.40.2"},
		{`audio/webm; codecs="opus"`, "webm", "", "opus"},
	}
	for _, c := range cases {
		cont, v, a := parseMime(c.mime)
		if cont != c.cont || v != c.vcodec || a != c.acodec {
			t.Errorf("parseMime(%q) = (%q,%q,%q), want (%q,%q,%q)", c.mime, cont, v, a, c.cont, c.vcodec, c.acodec)
		}
	}
}
