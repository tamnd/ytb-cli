package youtube

import "testing"

func TestNormalizeVideoURL(t *testing.T) {
	const want = BaseURL + "/watch?v=dQw4w9WgXcQ"
	cases := []string{
		"dQw4w9WgXcQ",
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ&list=PL123",
		"https://youtu.be/dQw4w9WgXcQ",
		"https://www.youtube.com/shorts/dQw4w9WgXcQ",
	}
	for _, in := range cases {
		if got := NormalizeVideoURL(in); got != want {
			t.Errorf("NormalizeVideoURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExtractVideoID(t *testing.T) {
	cases := map[string]string{
		"dQw4w9WgXcQ":                                 "dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ":                "dQw4w9WgXcQ",
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ": "dQw4w9WgXcQ",
		"https://www.youtube.com/shorts/dQw4w9WgXcQ":  "dQw4w9WgXcQ",
	}
	for in, want := range cases {
		if got := ExtractVideoID(in); got != want {
			t.Errorf("ExtractVideoID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizePlaylistURL(t *testing.T) {
	const want = BaseURL + "/playlist?list=PLabc"
	cases := []string{
		"PLabc",
		"https://www.youtube.com/playlist?list=PLabc",
		"https://www.youtube.com/watch?v=x&list=PLabc",
	}
	for _, in := range cases {
		if got := NormalizePlaylistURL(in); got != want {
			t.Errorf("NormalizePlaylistURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExtractPlaylistID(t *testing.T) {
	cases := map[string]string{
		"PLabcdefghij": "PLabcdefghij",
		"RDabcdefghij": "RDabcdefghij",
		"https://www.youtube.com/playlist?list=PLxyz": "PLxyz",
		"PL":           "", // too short to be a bare id
		"notaplaylist": "",
	}
	for in, want := range cases {
		if got := ExtractPlaylistID(in); got != want {
			t.Errorf("ExtractPlaylistID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeChannelURL(t *testing.T) {
	cases := map[string]string{
		"@RickAstleyYT":                BaseURL + "/@RickAstleyYT/videos",
		"UCuAXFkgsw1L7xaCfnd5JJOw":     BaseURL + "/channel/UCuAXFkgsw1L7xaCfnd5JJOw/videos",
		"https://www.youtube.com/@foo": "https://www.youtube.com/@foo/videos",
	}
	for in, want := range cases {
		if got := NormalizeChannelURL(in); got != want {
			t.Errorf("NormalizeChannelURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeChannelID(t *testing.T) {
	cases := map[string]string{
		"@RickAstleyYT":                               "RickAstleyYT",
		"UCuAXFkgsw1L7xaCfnd5JJOw":                    "UCuAXFkgsw1L7xaCfnd5JJOw",
		"https://www.youtube.com/@foo/videos":         "foo",
		"https://www.youtube.com/channel/UC123/about": "channel", // first path segment
	}
	for in, want := range cases {
		if got := NormalizeChannelID(in); got != want {
			t.Errorf("NormalizeChannelID(%q) = %q, want %q", in, got, want)
		}
	}
}
