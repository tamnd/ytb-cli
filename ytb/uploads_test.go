package ytb

import (
	"context"
	"strings"
	"testing"

	"github.com/tamnd/any-cli/kit/errs"
	"github.com/tamnd/ytb-cli/pkg/ytid"
)

// The three kinds partition the uploads playlist, which is the whole reason the
// derived ids are worth having. Measured on @RickAstleyYT: UULF 139 videos plus
// UUSH 294 shorts plus UULV 2 streams is UU's 435 uploads, in four requests with
// no paging.
func TestUploadKindsDeriveFromTheChannelID(t *testing.T) {
	ids, ok := ytid.PlaylistsFor("UCuAXFkgsw1L7xaCfnd5JJOw")
	if !ok {
		t.Fatal("a UC channel id should derive playlists")
	}
	want := map[string]string{
		"all":     "UUuAXFkgsw1L7xaCfnd5JJOw",
		"videos":  "UULFuAXFkgsw1L7xaCfnd5JJOw",
		"shorts":  "UUSHuAXFkgsw1L7xaCfnd5JJOw",
		"streams": "UULVuAXFkgsw1L7xaCfnd5JJOw",
	}
	for kind, id := range want {
		if got := uploadKinds[kind].playlist(ids); got != id {
			t.Errorf("--kind %s derives %q, want %q", kind, got, id)
		}
	}
	if uploadKinds["popular"].playlist(ids) == "" {
		t.Error("--kind popular has no playlist behind it")
	}
}

// There is no tab that lists a channel's uploads. The Videos tab is long form
// only and equals UULF, so --kind all --via tab has no answer and has to say so
// rather than quietly returning the long form videos as if they were everything.
func TestUploadKindsWithoutATab(t *testing.T) {
	for _, kind := range []string{"all", "popular"} {
		if uploadKinds[kind].tab != "" {
			t.Errorf("--kind %s claims a tab and there is none", kind)
		}
	}
	for _, kind := range []string{"videos", "shorts", "streams"} {
		if uploadKinds[kind].tab != kind {
			t.Errorf("--kind %s maps to tab %q", kind, uploadKinds[kind].tab)
		}
	}
}

// The three ways of asking for something that does not exist, each of which has
// to fail before a request goes out rather than after one comes back wrong.
func TestStreamUploadsRejectsBadCombinations(t *testing.T) {
	c := NewClient(DefaultConfig())
	nothing := func(Video) error { return nil }
	cases := []struct {
		label string
		opt   UploadsOptions
		want  string
	}{
		{"unknown kind", UploadsOptions{Kind: "reels"}, "unknown kind"},
		{"unknown via", UploadsOptions{Kind: "videos", Via: "rss"}, "unknown --via"},
		{"all has no tab", UploadsOptions{Kind: "all", Via: "tab"}, "no tab behind --kind all"},
		{"popular has no tab", UploadsOptions{Kind: "popular", Via: "tab"}, "no tab behind --kind popular"},
	}
	for _, tc := range cases {
		err := c.StreamUploads(context.Background(), "@RickAstleyYT", tc.opt, nothing)
		if err == nil {
			t.Errorf("%s: expected an error", tc.label)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error = %q, want it to mention %q", tc.label, err, tc.want)
		}
		// Usage, so the exit code says the command was wrong rather than that
		// YouTube had nothing.
		if errs.KindOf(err) != errs.KindUsage {
			t.Errorf("%s: kind = %v, want usage so the exit code is 2", tc.label, errs.KindOf(err))
		}
	}
}
