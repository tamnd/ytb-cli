package youtube

import (
	"context"
	"errors"
	"sort"
	"testing"
)

// fakeGraph is an in-memory grapher: a tiny YouTube whose edges are fixed maps,
// so the walker is tested without a network. It implements every grapher method
// the walker calls.
type fakeGraph struct {
	videos    map[string]*VideoResult    // video id -> result (with Related)
	channels  map[string]*Channel        // ref (id/handle) -> channel
	playlists map[string]*Playlist       // playlist id -> playlist
	uploads   map[string][]Video         // channel id -> uploaded videos
	plLists   map[string][]Playlist      // channel id -> playlists
	community map[string][]CommunityPost // channel id -> posts
	items     map[string][]Video         // playlist id -> videos
	comments  map[string][]Comment       // video id -> comments
	commErr   map[string]error           // video id -> error from StreamComments
}

func (g *fakeGraph) FetchVideo(_ context.Context, ref string, _ VideoOptions) (*VideoResult, error) {
	if v, ok := g.videos[ref]; ok {
		return v, nil
	}
	return nil, errors.New("video not found: " + ref)
}

func (g *fakeGraph) FetchChannel(_ context.Context, ref string) (*Channel, error) {
	if c, ok := g.channels[ref]; ok {
		return c, nil
	}
	return nil, errors.New("channel not found: " + ref)
}

func (g *fakeGraph) FetchPlaylist(_ context.Context, ref string) (*Playlist, error) {
	if p, ok := g.playlists[ref]; ok {
		return p, nil
	}
	return nil, errors.New("playlist not found: " + ref)
}

func streamSlice[T any](items []T, max int, emit func(T) error) error {
	n := 0
	for _, it := range items {
		if max > 0 && n >= max {
			return nil
		}
		if err := emit(it); err != nil {
			if err == ErrStop {
				return nil
			}
			return err
		}
		n++
	}
	return nil
}

func (g *fakeGraph) StreamChannelTab(_ context.Context, ref, _ string, opt PageOptions, emit func(Video) error) error {
	return streamSlice(g.uploads[ref], opt.Max, emit)
}

func (g *fakeGraph) StreamChannelPlaylists(_ context.Context, ref string, opt PageOptions, emit func(Playlist) error) error {
	return streamSlice(g.plLists[ref], opt.Max, emit)
}

func (g *fakeGraph) StreamPlaylistItems(_ context.Context, ref string, opt PageOptions, emit func(PlaylistVideo, Video) error) error {
	vids := g.items[ref]
	n := 0
	for i, v := range vids {
		if opt.Max > 0 && n >= opt.Max {
			return nil
		}
		if err := emit(PlaylistVideo{PlaylistID: ref, VideoID: v.VideoID, Position: i}, v); err != nil {
			if err == ErrStop {
				return nil
			}
			return err
		}
		n++
	}
	return nil
}

func (g *fakeGraph) StreamComments(_ context.Context, ref string, opt CommentOptions, emit func(Comment) error) error {
	if err, ok := g.commErr[ref]; ok {
		return err
	}
	return streamSlice(g.comments[ref], opt.Max, emit)
}

func (g *fakeGraph) StreamCommunity(_ context.Context, ref string, opt PageOptions, emit func(CommunityPost) error) error {
	return streamSlice(g.community[ref], opt.Max, emit)
}

// newFakeGraph builds a small connected corpus:
//
//	video vid1 (channel UCa, related [vid2])
//	channel UCa (uploads [vid1, vid3], playlists [PLaaaaaaaaaa], community [post1])
//	playlist PLaaaaaaaaaa (items [vid1, vid2], owner UCa)
//	comments on vid1: cmt1 by UCb
//	channel UCb (the commenter)
func newFakeGraph() *fakeGraph {
	vid1 := &VideoResult{
		Video:   Video{VideoID: "vid1", Title: "First", ChannelID: "UCa", ChannelName: "Alpha"},
		Related: []RelatedVideo{{VideoID: "vid1", RelatedVideoID: "vid2", Position: 0}},
	}
	vid2 := &VideoResult{Video: Video{VideoID: "vid2", Title: "Second", ChannelID: "UCa", ChannelName: "Alpha"}}
	vid3 := &VideoResult{Video: Video{VideoID: "vid3", Title: "Third", ChannelID: "UCa", ChannelName: "Alpha"}}
	return &fakeGraph{
		videos: map[string]*VideoResult{"vid1": vid1, "vid2": vid2, "vid3": vid3},
		channels: map[string]*Channel{
			"UCa": {ChannelID: "UCa", Handle: "@alpha", Title: "Alpha"},
			"UCb": {ChannelID: "UCb", Handle: "@beta", Title: "Beta"},
		},
		playlists: map[string]*Playlist{
			"PLaaaaaaaaaa": {PlaylistID: "PLaaaaaaaaaa", Title: "Mix", ChannelID: "UCa", ChannelName: "Alpha"},
		},
		uploads: map[string][]Video{
			"UCa": {{VideoID: "vid1", Title: "First", ChannelID: "UCa"}, {VideoID: "vid3", Title: "Third", ChannelID: "UCa"}},
		},
		plLists: map[string][]Playlist{
			"UCa": {{PlaylistID: "PLaaaaaaaaaa", Title: "Mix", ChannelID: "UCa"}},
		},
		community: map[string][]CommunityPost{
			"UCa": {{PostID: "post1", ChannelID: "UCa", AuthorName: "Alpha", ContentText: "hello"}},
		},
		items: map[string][]Video{
			"PLaaaaaaaaaa": {{VideoID: "vid1"}, {VideoID: "vid2"}},
		},
		comments: map[string][]Comment{
			"vid1": {{ID: "cmt1", VideoID: "vid1", AuthorChannelID: "UCb", AuthorDisplayName: "Beta", TextDisplay: "nice"}},
		},
	}
}

// collect walks and returns the emitted nodes plus the recorded edges.
func collect(t *testing.T, g grapher, seeds []Seed, opts WalkOptions) ([]*Node, []string) {
	t.Helper()
	var edgesLog []string
	opts.OnEdge = func(src, dst string, e Edge) {
		edgesLog = append(edgesLog, string(e)+":"+src+"->"+dst)
	}
	var nodes []*Node
	err := NewWalker(g).Walk(context.Background(), seeds, opts, func(n *Node) error {
		nodes = append(nodes, n)
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	return nodes, edgesLog
}

func TestParseEdges(t *testing.T) {
	t.Run("empty is the content default", func(t *testing.T) {
		set, err := ParseEdges("")
		if err != nil {
			t.Fatal(err)
		}
		want := edgePresets["content"]
		if set.String() != want.String() {
			t.Fatalf("got %q, want %q", set, want)
		}
	})
	t.Run("preset expands", func(t *testing.T) {
		set, err := ParseEdges("comments")
		if err != nil {
			t.Fatal(err)
		}
		if !set.Has(EdgeComments) || !set.Has(EdgeCommenter) {
			t.Fatalf("comments preset missing edges: %q", set)
		}
	})
	t.Run("list of edges and presets", func(t *testing.T) {
		set, err := ParseEdges("related,channel,uploads")
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range []Edge{EdgeRelated, EdgeChannel, EdgeUploads} {
			if !set.Has(e) {
				t.Fatalf("missing %s in %q", e, set)
			}
		}
	})
	t.Run("unknown token errors", func(t *testing.T) {
		if _, err := ParseEdges("nope"); err == nil {
			t.Fatal("expected error for unknown token")
		}
	})
}

func TestEdgeTargetAndGated(t *testing.T) {
	cases := map[Edge]NodeKind{
		EdgeChannel: KindChannel, EdgeOwner: KindChannel, EdgeCommenter: KindChannel,
		EdgePlaylists: KindPlaylist, EdgeComments: KindComment, EdgeCommunity: KindPost,
		EdgeRelated: KindVideo, EdgeUploads: KindVideo, EdgeItems: KindVideo,
	}
	for e, want := range cases {
		if got := e.Target(); got != want {
			t.Errorf("%s.Target() = %s, want %s", e, got, want)
		}
	}
	for _, e := range []Edge{EdgeComments, EdgeCommenter} {
		if !e.gated() {
			t.Errorf("%s should be gated", e)
		}
	}
	for _, e := range []Edge{EdgeChannel, EdgeRelated, EdgeUploads, EdgeItems, EdgeOwner, EdgePlaylists, EdgeCommunity} {
		if e.gated() {
			t.Errorf("%s should not be gated", e)
		}
	}
}

func TestParseSeed(t *testing.T) {
	cases := []struct {
		in   string
		kind NodeKind
		ref  string
	}{
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", KindVideo, "dQw4w9WgXcQ"},
		{"dQw4w9WgXcQ", KindVideo, "dQw4w9WgXcQ"},
		{"UCabcdefghijklmnopqrst12", KindChannel, "UCabcdefghijklmnopqrst12"},
		{"@mkbhd", KindChannel, "@mkbhd"},
		{"PLaaaaaaaaaaaa", KindPlaylist, "PLaaaaaaaaaaaa"},
		{"https://www.youtube.com/playlist?list=PLxyz", KindPlaylist, "PLxyz"},
	}
	for _, c := range cases {
		s, err := ParseSeed(c.in)
		if err != nil {
			t.Errorf("ParseSeed(%q): %v", c.in, err)
			continue
		}
		if s.Kind != c.kind || s.Ref != c.ref {
			t.Errorf("ParseSeed(%q) = %s/%s, want %s/%s", c.in, s.Kind, s.Ref, c.kind, c.ref)
		}
	}
	if _, err := ParseSeed(""); err == nil {
		t.Error("expected error for empty seed")
	}
}

func TestWalkContentFromVideo(t *testing.T) {
	g := newFakeGraph()
	nodes, edges := collect(t, g, []Seed{{Kind: KindVideo, Ref: "vid1"}}, WalkOptions{Depth: 1, Edges: DefaultEdges()})

	// Breadth-first: the seed first, then its channel and related video.
	if len(nodes) != 3 {
		t.Fatalf("got %d nodes, want 3: %+v", len(nodes), endpoints(nodes))
	}
	if nodes[0].Kind != KindVideo || nodes[0].Depth != 0 {
		t.Fatalf("seed node = %s depth %d", nodes[0].Kind, nodes[0].Depth)
	}
	got := endpoints(nodes[1:])
	sort.Strings(got)
	want := []string{"UCa", "vid2"}
	if !equal(got, want) {
		t.Fatalf("neighbors = %v, want %v", got, want)
	}
	if !contains(edges, "channel:vid1->UCa") || !contains(edges, "related:vid1->vid2") {
		t.Fatalf("edges = %v", edges)
	}
}

func TestWalkChannelPreset(t *testing.T) {
	g := newFakeGraph()
	nodes, _ := collect(t, g, []Seed{{Kind: KindChannel, Ref: "UCa"}}, WalkOptions{Depth: 1, Edges: edgePresets["feed"]})
	got := endpoints(nodes)
	sort.Strings(got)
	// seed UCa, uploads vid1+vid3, playlist PLaaaaaaaaaa, post1.
	want := []string{"PLaaaaaaaaaa", "UCa", "post1", "vid1", "vid3"}
	if !equal(got, want) {
		t.Fatalf("channel walk = %v, want %v", got, want)
	}
}

func TestWalkCommentsThenCommenter(t *testing.T) {
	g := newFakeGraph()
	// Depth 2 so the comment (depth 1) expands to its author channel (depth 2).
	nodes, edges := collect(t, g, []Seed{{Kind: KindVideo, Ref: "vid1"}},
		WalkOptions{Depth: 2, Edges: edgePresets["comments"]})
	kinds := map[NodeKind]int{}
	for _, n := range nodes {
		kinds[n.Kind]++
	}
	if kinds[KindComment] != 1 {
		t.Fatalf("want 1 comment node, got %d (%v)", kinds[KindComment], endpoints(nodes))
	}
	if kinds[KindChannel] != 1 {
		t.Fatalf("want 1 commenter channel, got %d (%v)", kinds[KindChannel], endpoints(nodes))
	}
	if !contains(edges, "comments:vid1->cmt1") || !contains(edges, "commenter:cmt1->UCb") {
		t.Fatalf("edges = %v", edges)
	}
}

func TestWalkCommentsRestrictedDegrades(t *testing.T) {
	g := newFakeGraph()
	g.commErr = map[string]error{"vid1": ErrCommentsRestricted}
	var notes []string
	opts := WalkOptions{Depth: 1, Edges: edgePresets["comments"], Note: func(s string) { notes = append(notes, s) }}
	var nodes []*Node
	err := NewWalker(g).Walk(context.Background(), []Seed{{Kind: KindVideo, Ref: "vid1"}}, opts, func(n *Node) error {
		nodes = append(nodes, n)
		return nil
	})
	if err != nil {
		t.Fatalf("a gated comment edge must not be fatal: %v", err)
	}
	if len(nodes) != 1 || nodes[0].Kind != KindVideo {
		t.Fatalf("want just the seed, got %v", endpoints(nodes))
	}
	if len(notes) == 0 {
		t.Fatal("expected a note about the refused comment edge")
	}
}

func TestWalkBudgetStops(t *testing.T) {
	g := newFakeGraph()
	nodes, _ := collect(t, g, []Seed{{Kind: KindVideo, Ref: "vid1"}}, WalkOptions{Depth: 1, Max: 2, Edges: DefaultEdges()})
	if len(nodes) != 2 {
		t.Fatalf("budget Max=2 should stop at 2, got %d", len(nodes))
	}
}

func TestWalkFanoutCaps(t *testing.T) {
	g := newFakeGraph()
	// Channel uploads has 2 videos; fanout 1 keeps only one of them.
	nodes, _ := collect(t, g, []Seed{{Kind: KindChannel, Ref: "UCa"}},
		WalkOptions{Depth: 1, Fanout: 1, Edges: newEdgeSet(EdgeUploads)})
	if len(nodes) != 2 { // seed + 1 upload
		t.Fatalf("fanout 1 should yield seed + 1 upload, got %d (%v)", len(nodes), endpoints(nodes))
	}
}

func TestWalkDedup(t *testing.T) {
	g := newFakeGraph()
	// From the playlist: items vid1, vid2 and owner UCa. UCa's uploads bring vid1
	// and vid3 back; vid1 must not be emitted twice.
	nodes, _ := collect(t, g, []Seed{{Kind: KindPlaylist, Ref: "PLaaaaaaaaaa"}},
		WalkOptions{Depth: 2, Edges: newEdgeSet(EdgeItems, EdgeOwner, EdgeUploads)})
	seen := map[string]int{}
	for _, n := range nodes {
		seen[n.Endpoint()]++
	}
	for id, c := range seen {
		if c != 1 {
			t.Fatalf("node %s emitted %d times, want 1 (%v)", id, c, endpoints(nodes))
		}
	}
}

func TestWalkSeedNotFoundFatal(t *testing.T) {
	g := newFakeGraph()
	err := NewWalker(g).Walk(context.Background(), []Seed{{Kind: KindVideo, Ref: "missing"}},
		WalkOptions{Depth: 1, Edges: DefaultEdges()}, func(*Node) error { return nil })
	if err == nil {
		t.Fatal("an unfetchable seed must be fatal")
	}
}

func TestWalkDepthZeroSeedsOnly(t *testing.T) {
	g := newFakeGraph()
	nodes, edges := collect(t, g, []Seed{{Kind: KindVideo, Ref: "vid1"}}, WalkOptions{Depth: 0, Edges: DefaultEdges()})
	if len(nodes) != 1 {
		t.Fatalf("depth 0 should emit only the seed, got %d", len(nodes))
	}
	if len(edges) != 0 {
		t.Fatalf("depth 0 should record no edges, got %v", edges)
	}
}

// --- small test helpers ---

func endpoints(nodes []*Node) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.Endpoint()
	}
	return out
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
