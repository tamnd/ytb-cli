package ytb

import (
	"strings"
	"testing"

	"github.com/tamnd/any-cli/kit/errs"
)

// The fixture is the real tab strip from @RickAstleyYT, trimmed to the fields that
// name a tab. The titles are in it on purpose: they are what a title match would
// have keyed on, and Home/Live show why that fails even in English.
func TestChannelTabsFromRealStrip(t *testing.T) {
	resp := loadFixture(t, "channel_tabs.json")
	tabs := ChannelTabs(resp)
	want := map[string]string{
		"featured":  "Home",
		"videos":    "Videos",
		"shorts":    "Shorts",
		"streams":   "Live",
		"releases":  "Releases",
		"playlists": "Playlists",
		"posts":     "Posts",
		"search":    "Search",
	}
	if len(tabs) != len(want) {
		t.Fatalf("read %d tabs, want %d: %v", len(tabs), len(want), slugs(tabs))
	}
	for _, tab := range tabs {
		title, ok := want[tab.Slug]
		if !ok {
			t.Errorf("unexpected tab %q", tab.Slug)
			continue
		}
		if tab.Title != title {
			t.Errorf("tab %s: title = %q, want %q", tab.Slug, tab.Title, title)
		}
		if tab.Params == "" {
			t.Errorf("tab %s: no params", tab.Slug)
		}
		if tab.BrowseID != "UCuAXFkgsw1L7xaCfnd5JJOw" {
			t.Errorf("tab %s: browseId = %q", tab.Slug, tab.BrowseID)
		}
	}
}

// The response states each tab's name twice, in the params blob and in the web
// url. They have to agree, because tabSlug prefers the blob and a silent
// disagreement would mean the two are not the same fact after all.
func TestTabSlugAgreesWithURL(t *testing.T) {
	resp := loadFixture(t, "channel_tabs.json")
	var checked int
	walkJSON(resp, func(m map[string]any) {
		for _, key := range tabRendererKeys {
			tr, ok := m[key].(map[string]any)
			if !ok {
				continue
			}
			ep := mapValue(tr, "endpoint")
			fromParams := paramsSlug(stringValue(mapValue(ep, "browseEndpoint")["params"]))
			fromURL := urlSlug(stringValue(mapValue(mapValue(ep, "commandMetadata"), "webCommandMetadata")["url"]))
			if fromParams != fromURL {
				t.Errorf("tab %q: params says %q, url says %q", extractText(tr["title"]), fromParams, fromURL)
			}
			checked++
		}
	})
	if checked != 8 {
		t.Fatalf("checked %d tabs, want 8", checked)
	}
}

// The posts blob this package used to carry, next to the one the channel actually
// answers to. They are different, which is the whole argument for reading the blob
// off the response.
func TestPostsBlobThatWentStaleStillDecodes(t *testing.T) {
	const shipped = "EgVwb3N0c_IGBAoCEgA"   // what this package hardcoded
	const served = "EgVwb3N0c_IGBAoCSgA%3D" // what @RickAstleyYT serves
	if shipped == served {
		t.Fatal("the two blobs are equal, so the stale-blob test proves nothing")
	}
	for _, blob := range []string{shipped, served} {
		if got := paramsSlug(blob); got != "posts" {
			t.Errorf("paramsSlug(%q) = %q, want posts", blob, got)
		}
	}
}

func TestParamsSlugRejectsGarbage(t *testing.T) {
	for _, in := range []string{"", "!!!!", "AAAA", "EgZ", "MTIzNDU2Nzg5"} {
		if got := paramsSlug(in); got != "" {
			t.Errorf("paramsSlug(%q) = %q, want empty", in, got)
		}
	}
}

func TestAssertTabAcceptsTheSelectedTab(t *testing.T) {
	resp := loadFixture(t, "channel_tabs.json")
	if err := AssertTab(resp, "featured"); err != nil {
		t.Fatalf("featured is the selected tab, got %v", err)
	}
}

// A tab the channel does have, but not the one the response selected. This is the
// case YouTube answers with a 200 and the home page.
func TestAssertTabRejectsTheWrongTab(t *testing.T) {
	resp := loadFixture(t, "channel_tabs.json")
	err := AssertTab(resp, "videos")
	if err == nil {
		t.Fatal("asked for videos and got the featured tab, want an error")
	}
	if errs.ExitCode(err) != 3 {
		t.Fatalf("exit code = %d, want 3", errs.ExitCode(err))
	}
	if !strings.Contains(err.Error(), "featured") {
		t.Errorf("error should name the tab YouTube selected: %v", err)
	}
}

// A tab the channel does not have at all. The error has to carry the real list,
// because "no members tab" is not useful without "here is what there is".
func TestAssertTabListsTheRealTabs(t *testing.T) {
	resp := loadFixture(t, "channel_tabs.json")
	err := AssertTab(resp, "membership")
	if err == nil {
		t.Fatal("want an error for a tab this channel has not got")
	}
	if errs.ExitCode(err) != 3 {
		t.Fatalf("exit code = %d, want 3", errs.ExitCode(err))
	}
	for _, slug := range []string{"videos", "shorts", "streams", "playlists", "posts"} {
		if !strings.Contains(err.Error(), slug) {
			t.Errorf("error should list %s: %v", slug, err)
		}
	}
}

func TestAssertTabOnAContinuationWithNoStrip(t *testing.T) {
	if err := AssertTab(map[string]any{}, "videos"); err == nil {
		t.Fatal("a response with no tab strip should not pass as the videos tab")
	}
}

// A continuation response carries the strip with nothing selected. The tab being
// present is all that can be checked, and it is enough.
func TestAssertTabAcceptsAnUnselectedStrip(t *testing.T) {
	raw := `{"tabs":[{"tabRenderer":{"title":"Videos","endpoint":{"browseEndpoint":{
		"browseId":"UC1","params":"EgZ2aWRlb3PyBgQKAjoA"}}}}]}`
	if err := AssertTab(decode(t, raw), "videos"); err != nil {
		t.Fatalf("unselected strip with a videos tab: %v", err)
	}
}

func TestTabSlugForTheWordsPeopleType(t *testing.T) {
	cases := map[string]string{
		"videos": "videos", "shorts": "shorts", "live": "streams",
		"streams": "streams", "home": "featured", "community": "posts",
		"Videos": "videos", "posts": "posts",
	}
	for in, want := range cases {
		if got := tabSlugFor(in); got != want {
			t.Errorf("tabSlugFor(%q) = %q, want %q", in, got, want)
		}
	}
}
