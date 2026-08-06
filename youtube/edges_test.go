package youtube

import (
	"strings"
	"testing"
	"time"

	"github.com/tamnd/ytb-cli/pkg/graph"
)

// edges_test.go holds the other half of the closed vocabulary. graph.NewEdge
// refuses a predicate nobody declared; this file refuses a predicate nobody
// emits, which is the failure that hides: the table says the tool reads
// something, ytb predicates prints the row, and no parser has ever written one.

const (
	testVideoID   = "dQw4w9WgXcQ"
	testChannelID = "UCuAXFkgsw1L7xaCfnd5JJOw"
	otherChannel  = "UC-lHJZR3Gqxm24_Vd_AJ5Yw"
)

func testProv() graph.Provenance {
	return graph.Provenance{Source: NormalizeVideoURL(testVideoID), Surface: SurfaceInnerTube, Client: "WEB"}
}

// emitEveryPredicate runs every claim parser over a record built to exercise it.
// The records are hand built rather than fetched because three of the twenty one
// predicates have no live producer at tier 0 on an ordinary network: comments
// are hidden by Restricted Mode, the community tab refuses an anonymous read,
// and the WEB_REMIX player refuses every id. Those parsers still have to be
// held to the table.
func emitEveryPredicate(set *graph.Set) {
	prov := testProv()

	yes := true
	video := Video{
		VideoID:      testVideoID,
		Title:        "Never Gonna Give You Up",
		ChannelID:    testChannelID,
		ChannelTitle: "Rick Astley",
		IsFamilySafe: &yes,
		PublishedAt:  time.Date(2009, 10, 25, 6, 57, 33, 0, time.UTC),
		Mentions:     []string{otherChannel},
		Links:        []Link{{Text: "the store", URL: "https://rickastley.co.uk/"}},
		Hashtags:     []string{"#RickRoll"},
		CaptionTracks: []CaptionTrack{
			{LanguageCode: "en", VssID: ".en", Name: "English"},
		},
		Chapters: []Chapter{{Title: "Intro", StartSeconds: 0, Position: 1}},
	}
	video.Sources = []string{NormalizeVideoURL(testVideoID)}
	video.Surfaces = []string{SurfaceWatchHTML}
	VideoClaims(set, video)

	ChannelClaims(set, Channel{
		Envelope:  Envelope{Sources: []string{NormalizeChannelURL(testChannelID)}, Surfaces: []string{SurfaceBrowseHTML}},
		ChannelID: testChannelID,
		Title:     "Rick Astley",
		Links:     []ChannelLink{{Title: "Website", URL: "https://rickastley.co.uk"}},
	})

	PlaylistClaims(set, Playlist{
		Envelope:   Envelope{Sources: []string{NormalizePlaylistURL("PLlaN88a7y2_plecYoJxvRFTLHVbIVAOoc")}, Surfaces: []string{SurfaceBrowseHTML}},
		PlaylistID: "PLlaN88a7y2_plecYoJxvRFTLHVbIVAOoc",
		Title:      "The playlist",
		ChannelID:  testChannelID,
	})

	PlaylistItemClaims(set, "PLlaN88a7y2_plecYoJxvRFTLHVbIVAOoc", []Video{
		{VideoID: "fcnDmrtj6Sk", Title: "One", ChannelID: testChannelID, Position: 1},
		{VideoID: "JNEK3G9Mkfg", Title: "Two", ChannelID: otherChannel, Position: 2},
	}, prov)

	RelatedClaims(set, testVideoID, []Video{
		{VideoID: "fcnDmrtj6Sk", Title: "Dai Dai", ChannelID: otherChannel},
	}, prov)

	CommentClaims(set, []Comment{
		{ID: "UgxKREWxIgDrw8w2e_Z4AaABAg", VideoID: testVideoID, AuthorChannelID: otherChannel, AuthorDisplayName: "Somebody", TextDisplay: "never gonna give you up"},
		{ID: "UgxKREWxIgDrw8w2e_Z4AaABAg.9fMPPjZbFUu9fMSRxvxbHV", VideoID: testVideoID, AuthorChannelID: testChannelID, TextDisplay: "a reply"},
	}, prov)

	PostClaims(set, []CommunityPost{{
		PostID:      "UgkxKREWxIgDrw8w2e_Z4AaABCQ",
		ChannelID:   testChannelID,
		AuthorName:  "Rick Astley",
		ContentText: "New video out now",
		Attachments: `[{"type":"video","id":"fcnDmrtj6Sk"},{"type":"playlist","id":"PLlaN88a7y2_plecYoJxvRFTLHVbIVAOoc"},{"type":"image","url":"https://yt3.ggpht.com/x.jpg"},{"type":"post","id":"UgkxAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},{"type":"poll"}]`,
	}}, prov)

	FeaturedClaims(set, testChannelID, []Channel{{ChannelID: otherChannel, Title: "Another channel"}}, prov)

	musicProv := graph.Provenance{Source: "https://music.youtube.com/browse/MPREb_test", Surface: SurfaceMusic, Client: "WEB_REMIX"}
	AlbumClaims(set, Album{AlbumID: "MPREb_test", Title: "Whenever You Need Somebody", ArtistID: testChannelID, ArtistName: "Rick Astley"},
		[]Song{{VideoID: "fcnDmrtj6Sk", Title: "One", ArtistID: testChannelID, ArtistName: "Rick Astley", AlbumID: "MPREb_test", AlbumName: "Whenever You Need Somebody"}}, musicProv)

	SeenAsClaims(set, testVideoID, "JNEK3G9Mkfg", musicProv, "the art track")
}

func TestEveryPredicateIsUsed(t *testing.T) {
	set := graph.NewSet()
	emitEveryPredicate(set)
	counts := set.CountByPredicate()

	for _, info := range graph.All() {
		if counts[info.Name] == 0 {
			t.Errorf("%s is in the table and no parser emitted one, so ytb predicates advertises a claim the tool never writes", info.Name)
		}
	}
	// And the other way round, which a typo in a note or a copied line would
	// otherwise sneak past.
	for p := range counts {
		if !graph.Known(p) {
			t.Errorf("a parser emitted %s, which is not in the table", p)
		}
	}
}

func TestClaimsPointTheRightWay(t *testing.T) {
	set := graph.NewSet()
	emitEveryPredicate(set)

	video := graph.VideoURI(testVideoID)
	channel := graph.ChannelURI(testChannelID)
	want := []struct {
		from graph.URI
		p    graph.Predicate
		to   graph.URI
	}{
		{channel, graph.Published, video},
		// Derived from the UU prefix with no request, which is the cheapest claim
		// in the store and the one that seeds a crawl of a channel's whole output.
		{graph.PlaylistURI("UUuAXFkgsw1L7xaCfnd5JJOw"), graph.UploadsOf, channel},
		{video, graph.Tagged, graph.HashtagURI("rickroll")},
		{video, graph.Mentions, graph.ChannelURI(otherChannel)},
		{graph.ChapterURI(testVideoID, 0), graph.ChapterOf, video},
		{video, graph.HasCaptions, graph.CaptionURI(testVideoID, "en")},
		{graph.VideoURI("JNEK3G9Mkfg"), graph.InPlaylistAfter, graph.VideoURI("fcnDmrtj6Sk")},
		{video, graph.SeenAs, graph.VideoURI("JNEK3G9Mkfg")},
	}
	have := map[string]bool{}
	for _, e := range set.Edges() {
		have[string(e.From)+" "+string(e.Predicate)+" "+string(e.To)] = true
	}
	for _, w := range want {
		if !have[string(w.from)+" "+string(w.p)+" "+string(w.to)] {
			t.Errorf("missing claim: %s %s %s", w.from, w.p, w.to)
		}
	}
}

func TestRepliesToComesOffTheIDWithNoRequest(t *testing.T) {
	set := graph.NewSet()
	const parent = "UgxKREWxIgDrw8w2e_Z4AaABAg"
	const reply = parent + ".9fMPPjZbFUu9fMSRxvxbHV"
	// No ParentID on the record: the dot in the id is the only thing that says so,
	// which is what lets a page of replies state its own tree.
	CommentClaims(set, []Comment{{ID: reply, VideoID: testVideoID, TextDisplay: "a reply"}}, testProv())

	var found *graph.Edge
	for i, e := range set.Edges() {
		if e.Predicate == graph.RepliesTo {
			found = &set.Edges()[i]
		}
	}
	if found == nil {
		t.Fatal("no replies_to claim came off a reply id")
	}
	if found.To != graph.CommentURI(parent) {
		t.Errorf("replies_to points at %s, want the parent comment", found.To)
	}
	if found.Surface != graph.SurfaceDerived {
		t.Errorf("replies_to is credited to surface %q, and no request was made to learn it", found.Surface)
	}
}

func TestDerivedClaimsCreditNoRequest(t *testing.T) {
	set := graph.NewSet()
	emitEveryPredicate(set)
	for _, e := range set.Edges() {
		if e.Predicate != graph.UploadsOf {
			continue
		}
		if e.Surface != graph.SurfaceDerived {
			t.Errorf("uploads_of is credited to %q, but the UU prefix is arithmetic and not a read", e.Surface)
		}
		if e.Client != "" {
			t.Errorf("uploads_of names client %q, and no client was told anything", e.Client)
		}
	}
}

func TestUnresolvedHandleWritesNothing(t *testing.T) {
	// The failure this prevents is quiet: without it every video whose byline was
	// only ever a handle hangs a claim on yt://channel/, which is one node
	// collecting edges from every unresolved channel in the store.
	set := graph.NewSet()
	VideoClaims(set, Video{VideoID: testVideoID, Title: "A video", ChannelID: "@RickAstleyYT"})
	for _, e := range set.Edges() {
		if strings.Contains(string(e.From), "channel/@") || strings.Contains(string(e.To), "channel/@") {
			t.Errorf("a handle was used as a node: %+v", e)
		}
		if e.Predicate == graph.Published {
			t.Errorf("published was written from an unresolved handle: %+v", e)
		}
	}
}

func TestNoteNamesTheEndThatIsNew(t *testing.T) {
	// A related lockup gives the video a title and the channel nothing, so
	// labelling the published claim with the channel leaves the commonest claim
	// in the store with an empty note.
	set := graph.NewSet()
	RelatedClaims(set, testVideoID, []Video{{VideoID: "fcnDmrtj6Sk", Title: "Dai Dai", ChannelID: otherChannel}}, testProv())
	for _, e := range set.Edges() {
		if e.Note == "" {
			t.Errorf("claim with no note: %+v", e)
		}
	}

	if got := excerpt("a comment\nwith a newline in it"); got != "a comment with a newline in it" {
		t.Errorf("excerpt kept the newline: %q", got)
	}
	long := excerpt(strings.Repeat("x", 200))
	if len([]rune(long)) != 60 {
		t.Errorf("excerpt returned %d runes, want 60", len([]rune(long)))
	}
	if !strings.HasSuffix(long, "…") {
		t.Errorf("a cut excerpt does not say it was cut: %q", long)
	}
}

func TestPollAttachmentProducesNothing(t *testing.T) {
	// It is a real attachment with no node behind it: the options are text on the
	// post, and a node per poll option is a node nothing else ever names.
	set := graph.NewSet()
	PostClaims(set, []CommunityPost{{
		PostID:      "UgkxKREWxIgDrw8w2e_Z4AaABCQ",
		ChannelID:   testChannelID,
		ContentText: "Which one?",
		Attachments: `[{"type":"poll"}]`,
	}}, testProv())
	if n := set.CountByPredicate()[graph.Attaches]; n != 0 {
		t.Errorf("a poll produced %d attaches claims", n)
	}
	if set.Len() != 1 {
		t.Errorf("got %d claims, want just the posted one", set.Len())
	}
}
