package youtube_test

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/ytb-cli/youtube"
)

// These tests exercise the kit driver wiring with no network: the import of the
// youtube package registers the domain, and Classify/Locate/Mint/Body/Resolve
// are pure string and reflection work over the registry.

func TestDomainInfo(t *testing.T) {
	info := youtube.Domain{}.Info()
	if info.Scheme != "youtube" {
		t.Errorf("scheme = %q, want youtube", info.Scheme)
	}
	wantAlias := map[string]bool{"yt": true, "ytb": true}
	for _, a := range info.Aliases {
		delete(wantAlias, a)
	}
	if len(wantAlias) != 0 {
		t.Errorf("aliases = %v, want yt and ytb present", info.Aliases)
	}
}

func TestClassify(t *testing.T) {
	d := youtube.Domain{}
	cases := []struct {
		in, typ, id string
	}{
		{"dQw4w9WgXcQ", "video", "dQw4w9WgXcQ"},
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", "video", "dQw4w9WgXcQ"},
		{"https://youtu.be/dQw4w9WgXcQ", "video", "dQw4w9WgXcQ"},
		{"https://www.youtube.com/shorts/abc123XYZ_-", "video", "abc123XYZ_-"},
		// A watch link with both ids resolves to the video, not the playlist.
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ&list=PLrAXtmErZgOeiKm4sgNOknGvNjby9efdf", "video", "dQw4w9WgXcQ"},
		{"PLrAXtmErZgOeiKm4sgNOknGvNjby9efdf", "playlist", "PLrAXtmErZgOeiKm4sgNOknGvNjby9efdf"},
		{"https://www.youtube.com/playlist?list=PLrAXtmErZgOeiKm4sgNOknGvNjby9efdf", "playlist", "PLrAXtmErZgOeiKm4sgNOknGvNjby9efdf"},
		{"@mkbhd", "channel", "@mkbhd"},
		{"UCBJycsmduvYEL83R_U4JriQ", "channel", "UCBJycsmduvYEL83R_U4JriQ"},
		{"https://www.youtube.com/channel/UCBJycsmduvYEL83R_U4JriQ", "channel", "UCBJycsmduvYEL83R_U4JriQ"},
		{"https://www.youtube.com/@mkbhd", "channel", "@mkbhd"},
		{"https://www.youtube.com/c/mkbhd", "channel", "mkbhd"},
		{"https://www.youtube.com/user/marquesbrownlee", "channel", "marquesbrownlee"},
	}
	for _, c := range cases {
		typ, id, err := d.Classify(c.in)
		if err != nil || typ != c.typ || id != c.id {
			t.Errorf("Classify(%q) = %q/%q/%v, want %q/%q", c.in, typ, id, err, c.typ, c.id)
		}
	}
	if _, _, err := d.Classify("   "); err == nil {
		t.Error("Classify(blank) = nil error, want error")
	}
}

func TestLocate(t *testing.T) {
	d := youtube.Domain{}
	cases := []struct {
		typ, id, want string
	}{
		{"video", "dQw4w9WgXcQ", "https://www.youtube.com/watch?v=dQw4w9WgXcQ"},
		{"playlist", "PLabc", "https://www.youtube.com/playlist?list=PLabc"},
		{"channel", "UCBJycsmduvYEL83R_U4JriQ", "https://www.youtube.com/channel/UCBJycsmduvYEL83R_U4JriQ/videos"},
		{"channel", "@mkbhd", "https://www.youtube.com/@mkbhd/videos"},
	}
	for _, c := range cases {
		loc, err := d.Locate(c.typ, c.id)
		if err != nil || loc != c.want {
			t.Errorf("Locate(%q,%q) = %q/%v, want %q", c.typ, c.id, loc, err, c.want)
		}
	}
	if _, err := d.Locate("nonsense", "x"); err == nil {
		t.Error("Locate(nonsense) = nil error, want error")
	}
}

func TestHostMintBodyResolve(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := h.Domain("youtube"); !ok {
		t.Fatal("youtube not mounted on host")
	}

	v := &youtube.Video{
		VideoID:     "dQw4w9WgXcQ",
		Title:       "Never Gonna Give You Up",
		Description: "The official video.",
		URL:         "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
	}
	minted, err := h.Mint(v)
	if err != nil || minted.String() != "youtube://video/dQw4w9WgXcQ" {
		t.Errorf("Mint(video) = %q/%v, want youtube://video/dQw4w9WgXcQ", minted.String(), err)
	}
	if body, ok := h.Body(v); !ok || body != "The official video." {
		t.Errorf("Body = %q/%v, want the description", body, ok)
	}

	u, err := h.ResolveOn("youtube", "dQw4w9WgXcQ")
	if err != nil || u.String() != "youtube://video/dQw4w9WgXcQ" {
		t.Errorf("ResolveOn(bare) = %q/%v", u.String(), err)
	}
	u, err = h.Resolve("https://youtu.be/dQw4w9WgXcQ")
	if err != nil || u.String() != "youtube://video/dQw4w9WgXcQ" {
		t.Errorf("Resolve(youtu.be) = %q/%v", u.String(), err)
	}
	// The yt alias canonicalizes to the youtube scheme.
	u, err = h.ResolveOn("yt", "dQw4w9WgXcQ")
	if err != nil || u.String() != "youtube://video/dQw4w9WgXcQ" {
		t.Errorf("ResolveOn(alias) = %q/%v", u.String(), err)
	}
}
