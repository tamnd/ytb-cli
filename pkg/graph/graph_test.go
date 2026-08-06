package graph

import "testing"

// The rules under test here are the ones that cost something to get wrong. A
// handle used as a key produces a second node for a channel the day somebody
// renames it, a predicate written outside the table is never queried, and a set
// that reorders itself between runs produces a dump nobody can diff.

func TestChannelURIRefusesAnythingButAChannelID(t *testing.T) {
	const id = "UCuAXFkgsw1L7xaCfnd5JJOw"
	if got := ChannelURI(id); got != "yt://channel/"+id {
		t.Fatalf("ChannelURI(%q) = %q", id, got)
	}
	// A handle, a legacy user and a custom URL all reach a channel and none of
	// them is one, so each has to come back empty and be resolved by the caller.
	for _, in := range []string{"@RickAstleyYT", "RickAstleyYT", "c/RickAstley", "", "UC"} {
		if got := ChannelURI(in); got != "" {
			t.Errorf("ChannelURI(%q) = %q, want empty", in, got)
		}
	}
}

func TestHashtagURIIsCaseInsensitive(t *testing.T) {
	a, b := HashtagURI("#RickRoll"), HashtagURI("rickroll")
	if a != b {
		t.Fatalf("%q and %q are two nodes for one hashtag feed", a, b)
	}
	if a != "yt://hashtag/rickroll" {
		t.Fatalf("HashtagURI = %q", a)
	}
	if got := HashtagURI("#"); got != "" {
		t.Errorf("HashtagURI(%q) = %q, want empty", "#", got)
	}
}

func TestExternalURIConvergesOnOneNode(t *testing.T) {
	same := []string{
		"http://example.com/",
		"https://example.com",
		"https://www.example.com/",
		"https://EXAMPLE.com#top",
	}
	first := ExternalURI(same[0])
	if first == "" {
		t.Fatal("ExternalURI returned empty for a real URL")
	}
	for _, u := range same[1:] {
		if got := ExternalURI(u); got != first {
			t.Errorf("ExternalURI(%q) = %q, want %q", u, got, first)
		}
	}
	// A different path is a different link, and the query string stays because
	// two videos linking with different query strings link to different pages.
	if ExternalURI("https://example.com/a") == first {
		t.Error("a different path collapsed onto the same node")
	}
	if ExternalURI("https://example.com/?ref=yt") == ExternalURI("https://example.com/") {
		t.Error("a query string was dropped from the key")
	}
}

func TestParseIsTheInverseOfTheConstructors(t *testing.T) {
	cases := []struct {
		uri  URI
		kind Kind
		id   string
		part string
	}{
		{VideoURI("dQw4w9WgXcQ"), Video, "dQw4w9WgXcQ", ""},
		{PlaylistURI("UUuAXFkgsw1L7xaCfnd5JJOw"), Playlist, "UUuAXFkgsw1L7xaCfnd5JJOw", ""},
		{CaptionURI("dQw4w9WgXcQ", "en"), Video, "dQw4w9WgXcQ", FragCaption},
		{ChapterURI("dQw4w9WgXcQ", 42), Video, "dQw4w9WgXcQ", FragChapter},
		{FormatURI("dQw4w9WgXcQ", 137), Video, "dQw4w9WgXcQ", FragFormat},
	}
	for _, c := range cases {
		p, ok := Parse(c.uri)
		if !ok {
			t.Errorf("Parse(%q) failed", c.uri)
			continue
		}
		if p.Kind != c.kind || p.ID != c.id || p.Part != c.part {
			t.Errorf("Parse(%q) = %+v, want kind %s id %s part %q", c.uri, p, c.kind, c.id, c.part)
		}
		if p.IsFragment() != (c.part != "") {
			t.Errorf("Parse(%q).IsFragment() = %v", c.uri, p.IsFragment())
		}
	}
	for _, bad := range []string{"", "https://youtu.be/x", "yt://video", "yt://video/", "yt://video/x#chapter"} {
		if _, ok := Parse(URI(bad)); ok {
			t.Errorf("Parse(%q) accepted a URI that is not one", bad)
		}
	}
}

func TestNewEdgeChecksTheTable(t *testing.T) {
	video := VideoURI("dQw4w9WgXcQ")
	channel := ChannelURI("UCuAXFkgsw1L7xaCfnd5JJOw")
	prov := Provenance{Source: "https://www.youtube.com/watch?v=dQw4w9WgXcQ", Surface: "s1", Client: "WEB"}

	if _, err := NewEdge(channel, Published, video, prov); err != nil {
		t.Fatalf("a channel publishing a video was refused: %v", err)
	}
	// The wrong way round is the mistake this check exists for: it loads without
	// complaint and says a video published a channel.
	if _, err := NewEdge(video, Published, channel, prov); err == nil {
		t.Error("published was accepted running from a video to a channel")
	}
	if _, err := NewEdge(channel, Predicate("publishes"), video, prov); err == nil {
		t.Error("a predicate outside the table was accepted")
	}
	// An empty end is a handle that was never resolved, and writing it would make
	// yt://channel/ a node that collects edges from every unresolved handle.
	if _, err := NewEdge(ChannelURI("@RickAstleyYT"), Published, video, prov); err == nil {
		t.Error("an empty end was accepted")
	}
	// A chapter is a fragment, so chapter_of has to refuse the video itself.
	if _, err := NewEdge(ChapterURI("dQw4w9WgXcQ", 0), ChapterOf, video, prov); err != nil {
		t.Errorf("chapter_of from a chapter fragment was refused: %v", err)
	}
	if _, err := NewEdge(video, ChapterOf, video, prov); err == nil {
		t.Error("chapter_of was accepted from a video to itself")
	}
}

func TestSetKeepsTwoSourcesAndOneNote(t *testing.T) {
	set := NewSet()
	video := VideoURI("dQw4w9WgXcQ")
	channel := ChannelURI("UCuAXFkgsw1L7xaCfnd5JJOw")
	page := Provenance{Source: "https://www.youtube.com/watch?v=dQw4w9WgXcQ", Surface: "s1", Client: "WEB"}
	feed := Provenance{Source: "https://www.youtube.com/feeds/videos.xml?channel_id=UCuAXFkgsw1L7xaCfnd5JJOw", Surface: "s6"}

	set.Claim(channel, Published, video, page, "")
	set.Claim(channel, Published, video, page, "Never Gonna Give You Up")
	if set.Len() != 1 {
		t.Fatalf("the same claim from the same source became %d rows", set.Len())
	}
	if got := set.Edges()[0].Note; got != "Never Gonna Give You Up" {
		t.Errorf("the note from the second sighting was dropped, got %q", got)
	}

	// Two surfaces agreeing is two observations. Collapsing them would lose the
	// fact that both said so, which is the whole reason the source is in the key.
	set.Claim(channel, Published, video, feed, "")
	if set.Len() != 2 {
		t.Fatalf("a second source collapsed into the first, got %d rows", set.Len())
	}
}

func TestEdgesAndNodesAreOrdered(t *testing.T) {
	prov := Provenance{Source: "https://www.youtube.com/watch?v=dQw4w9WgXcQ", Surface: "s1"}
	build := func() *Set {
		set := NewSet()
		me := VideoURI("dQw4w9WgXcQ")
		for _, id := range []string{"zzz", "aaa", "mmm"} {
			set.ClaimAt(me, RelatedTo, VideoURI(id), prov, "", 0)
		}
		set.Claim(ChannelURI("UCuAXFkgsw1L7xaCfnd5JJOw"), Published, me, prov, "")
		return set
	}
	first, second := build().Edges(), build().Edges()
	if len(first) != 4 {
		t.Fatalf("built %d claims, want 4", len(first))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("two identical builds ordered claim %d differently: %+v and %+v", i, first[i], second[i])
		}
	}
	if first[0].From != ChannelURI("UCuAXFkgsw1L7xaCfnd5JJOw") {
		t.Errorf("claims are not sorted by subject, first is %+v", first[0])
	}
	if want := 5; len(build().Nodes()) != want {
		t.Errorf("Nodes() = %d, want %d: the frontier is mostly things nobody fetched", len(build().Nodes()), want)
	}
}

func TestPositionSurvivesOrdering(t *testing.T) {
	set := NewSet()
	pl := PlaylistURI("PLxyz")
	prov := Provenance{Source: "https://www.youtube.com/playlist?list=PLxyz", Surface: "s2"}
	// Added out of order, because a continuation can arrive before the page it
	// continues in a store that merged two reads.
	for _, item := range []struct {
		id  string
		pos int
	}{{"ccc", 3}, {"aaa", 1}, {"bbb", 2}} {
		set.ClaimAt(pl, Contains, VideoURI(item.id), prov, "", item.pos)
	}
	got := set.Edges()
	for i, want := range []int{1, 2, 3} {
		if got[i].Position != want {
			t.Fatalf("claim %d has position %d, want %d: a playlist's order has to come back off the claims alone", i, got[i].Position, want)
		}
	}
}

func TestAllIsTheWholeTableInOneOrder(t *testing.T) {
	all := All()
	if len(all) != len(Predicates) {
		t.Fatalf("All() returned %d of %d predicates", len(all), len(Predicates))
	}
	if len(all) != 21 {
		t.Fatalf("the table has %d predicates, doc 04 section 3.1 says 21", len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i-1].Name >= all[i].Name {
			t.Fatalf("All() is not sorted at %d: %s then %s", i, all[i-1].Name, all[i].Name)
		}
	}
	for _, info := range all {
		if !Known(info.Name) {
			t.Errorf("%s is in All() and not in the table", info.Name)
		}
		if len(info.Domain) == 0 || len(info.Range) == 0 || info.Origin == "" || info.RDF == "" {
			t.Errorf("%s has an incomplete row: %+v", info.Name, info)
		}
	}
}

func TestURLIsNotDerivedForNodesThatHaveNoAddress(t *testing.T) {
	if got := VideoURI("dQw4w9WgXcQ").URL(); got != "https://www.youtube.com/watch?v=dQw4w9WgXcQ" {
		t.Errorf("video URL = %q", got)
	}
	// A comment has no page of its own and an external node's address is a
	// property, not something recoverable from a hash.
	for _, u := range []URI{CommentURI("Ugx"), ExternalURI("https://example.com")} {
		if got := u.URL(); got != "" {
			t.Errorf("%q.URL() = %q, want empty", u, got)
		}
	}
}
