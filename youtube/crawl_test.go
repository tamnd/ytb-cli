package youtube

import (
	"slices"
	"testing"

	"github.com/tamnd/ytb-cli/pkg/graph"
)

// A budget spent on listings buys an order of magnitude more graph than the same
// budget spent on watch pages, so listings go first.
func TestOrderFrontierPutsListingsFirst(t *testing.T) {
	refs := []string{
		"dQw4w9WgXcQ",
		"UCuAXFkgsw1L7xaCfnd5JJOw",
		"PLlaN88a7y2_plecYoJxvRFTLHVbIVAOoc",
		"UULFuAXFkgsw1L7xaCfnd5JJOw",
	}
	orderFrontier(refs)
	if frontierRank(refs[0]) != 0 || frontierRank(refs[1]) != 0 {
		t.Fatalf("frontier = %v, want the two playlists first", refs)
	}
	if refs[len(refs)-1] != "dQw4w9WgXcQ" {
		t.Errorf("frontier = %v, want the watch page last", refs)
	}
}

// A channel's uploads family is arithmetic on its id, so it costs no request to
// work out and one request each to read.
func TestDerivedPlaylists(t *testing.T) {
	got := derivedPlaylists(graph.ChannelURI("UCuAXFkgsw1L7xaCfnd5JJOw"))
	want := []graph.URI{
		graph.PlaylistURI("UUuAXFkgsw1L7xaCfnd5JJOw"),
		graph.PlaylistURI("UULFuAXFkgsw1L7xaCfnd5JJOw"),
		graph.PlaylistURI("UUSHuAXFkgsw1L7xaCfnd5JJOw"),
		graph.PlaylistURI("UULVuAXFkgsw1L7xaCfnd5JJOw"),
	}
	if !slices.Equal(got, want) {
		t.Errorf("derived = %v, want %v", got, want)
	}
	for _, u := range got {
		if u == graph.PlaylistURI("UULPuAXFkgsw1L7xaCfnd5JJOw") {
			t.Error("popular is on the frontier: it is UULF's videos in another order")
		}
	}
	if n := derivedPlaylists(graph.VideoURI("dQw4w9WgXcQ")); n != nil {
		t.Errorf("a video has an uploads family: %v", n)
	}
}

// Two things a crawl will not follow to, and neither is refused when somebody
// asks for it by name.
func TestSkipOnFrontier(t *testing.T) {
	for _, ref := range []string{"UULPuAXFkgsw1L7xaCfnd5JJOw", "RDdQw4w9WgXcQ"} {
		if !skipOnFrontier(ref) {
			t.Errorf("%s is on the frontier", ref)
		}
	}
	for _, ref := range []string{"UULFuAXFkgsw1L7xaCfnd5JJOw", "dQw4w9WgXcQ", "UCuAXFkgsw1L7xaCfnd5JJOw"} {
		if skipOnFrontier(ref) {
			t.Errorf("%s was kept off the frontier", ref)
		}
	}
}

// A comment has no address of its own, a hashtag page is a feed rather than an
// entity, and an external URL is somebody else's site.
func TestReadableRef(t *testing.T) {
	readable := map[graph.URI]string{
		graph.VideoURI("dQw4w9WgXcQ"):                      "dQw4w9WgXcQ",
		graph.ChannelURI("UCuAXFkgsw1L7xaCfnd5JJOw"):       "UCuAXFkgsw1L7xaCfnd5JJOw",
		graph.PlaylistURI("UULFuAXFkgsw1L7xaCfnd5JJOw"):    "UULFuAXFkgsw1L7xaCfnd5JJOw",
		graph.CommentURI("UgxKREWxIgDrGmefsGV4AaABAg"):     "",
		graph.HashtagURI("music"):                          "",
		graph.ExternalURI("https://example.com/somewhere"): "",
	}
	for uri, want := range readable {
		if got := ReadableRef(uri); got != want {
			t.Errorf("ReadableRef(%s) = %q, want %q", uri, got, want)
		}
	}
}

// The manifest says what the crawl did not do before it does anything, so a
// store with no comment claims in it can be explained without guessing.
func TestCrawlPolicyNamesWhatIsOff(t *testing.T) {
	policy := crawlPolicy(CrawlOptions{})
	seen := map[string]string{}
	for _, n := range policy {
		if n.Reason == "" {
			t.Errorf("%s is off with no reason given", n.Ref)
		}
		seen[n.Ref] = n.Reason
	}
	for _, ref := range []string{"mixes", "popular", "formats", "comments", "posts", "captions", "music", "uploads"} {
		if _, ok := seen[ref]; !ok {
			t.Errorf("the policy does not mention %s", ref)
		}
	}

	on := crawlPolicy(CrawlOptions{Uploads: true, Claims: ClaimOptions{Comments: 20, Captions: true}})
	for _, n := range on {
		if n.Ref == "comments" || n.Ref == "captions" || n.Ref == "uploads" {
			t.Errorf("%s is listed as off after being turned on", n.Ref)
		}
	}
}

// The store has the last word on what counts as unread, so a node this crawl
// only heard of and a previous crawl fetched is not fetched twice.
func TestNextFrontierSkipsWhatTheStoreRead(t *testing.T) {
	s := testStore(t)

	fetched := graph.VideoURI("dQw4w9WgXcQ")
	if _, err := s.PutRecord(Video{VideoID: "dQw4w9WgXcQ", Title: "Never Gonna Give You Up"}); err != nil {
		t.Fatalf("put record: %v", err)
	}
	named := map[graph.URI]bool{
		fetched:                          true,
		graph.VideoURI("9bZkp7q19f0"):    true,
		graph.CommentURI("UgxKREWxIgDr"): true,
		graph.PlaylistURI("UULPuAXFkgsw1L7xaCfnd5JJOw"): true,
	}
	got := nextFrontier(s, named, map[string]bool{"9bZkp7q19f0": false})

	if !slices.Equal(got, []string{"9bZkp7q19f0"}) {
		t.Errorf("frontier = %v, want just the video nothing has read", got)
	}
}
