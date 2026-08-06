package graph

import (
	"slices"
	"sort"
)

// predicate.go is the closed vocabulary. Doc 04 section 3.1.
//
// Twenty-one predicates, each with a domain and a range, and a predicate not in
// this table cannot be written. That is the point of a table rather than a
// convention: a typo in a predicate name produces a claim that looks fine, is
// never queried because nobody knows to ask for it, and is discovered a year
// later when somebody counts. NewEdge refuses one, and TestEveryPredicateIsUsed
// refuses the opposite, a predicate declared here that no parser ever emits.

// Predicate is an edge label.
type Predicate string

const (
	// Published is the backbone. Nearly every listing read produces one, and a
	// lockup byline naming a channel is a claim about a video nobody fetched.
	Published Predicate = "published"
	// UploadsOf is derived from the UU prefix and costs no request at all.
	UploadsOf  Predicate = "uploads_of"
	Owns       Predicate = "owns"
	Contains   Predicate = "contains"
	Mentions   Predicate = "mentions"
	LinksTo    Predicate = "links_to"
	Tagged     Predicate = "tagged"
	RelatedTo  Predicate = "related_to"
	CommentsOn Predicate = "comments_on"
	Commented  Predicate = "commented"
	// RepliesTo comes from the dot in a reply id, so a reply names its parent
	// with no request.
	RepliesTo   Predicate = "replies_to"
	Posted      Predicate = "posted"
	Attaches    Predicate = "attaches"
	Shares      Predicate = "shares"
	Features    Predicate = "features"
	HasCaptions Predicate = "has_captions"
	ChapterOf   Predicate = "chapter_of"
	ByArtist    Predicate = "by_artist"
	InAlbum     Predicate = "in_album"
	SeenAs      Predicate = "seen_as"
	// InPlaylistAfter is the only predicate whose two ends are the same kind and
	// whose direction carries the meaning on its own.
	InPlaylistAfter Predicate = "in_playlist_after"
)

// PredicateInfo is one row of the table: what may sit on each end, where the
// claim comes from, and how it is written in RDF.
type PredicateInfo struct {
	Name Predicate `json:"name" kit:"id" table:"predicate"`
	// Domain is the kinds allowed on the subject end, Range on the object end.
	// A fragment kind is written as its part name, so "chapter" here means
	// yt://video/<id>#chapter/<n> and not a node kind.
	Domain []string `json:"domain" table:"from"`
	Range  []string `json:"range" table:"to"`
	// Origin is the surface or the field that asserts it, in the words doc 04
	// uses, because "where it comes from" is the column somebody reads this table
	// for.
	Origin string `json:"origin" table:"origin,truncate"`
	// RDF is the term this maps to on the way out, and Inverse says the triple
	// turns round. Doc 04 section 5: ytb writes "channel published video" because
	// that is the direction a page reads in, and schema:author runs from the work
	// to its author. Getting that backwards produces a file that loads without
	// complaint and says a video wrote Rick Astley.
	RDF     string `json:"rdf" table:"rdf"`
	Inverse bool   `json:"inverse" table:"-"`
}

// Predicates is the table. It is a map so a lookup is a lookup, and All()
// returns it in a fixed order for display.
var Predicates = map[Predicate]PredicateInfo{
	Published: {
		Name: Published, Domain: []string{"channel"}, Range: []string{"video"},
		Origin: "videoDetails.channelId, a lockup byline, an Atom author",
		RDF:    "schema:author", Inverse: true,
	},
	UploadsOf: {
		Name: UploadsOf, Domain: []string{"playlist"}, Range: []string{"channel"},
		Origin: "the UU prefix, derived, no request",
		RDF:    "yt:uploadsOf",
	},
	Owns: {
		Name: Owns, Domain: []string{"channel"}, Range: []string{"playlist"},
		Origin: "a playlists tab entry, a playlist byline",
		RDF:    "schema:author", Inverse: true,
	},
	Contains: {
		Name: Contains, Domain: []string{"playlist"}, Range: []string{"video"},
		Origin: "a playlist item",
		RDF:    "schema:itemListElement",
	},
	Mentions: {
		Name: Mentions, Domain: []string{"video"}, Range: []string{"channel"},
		Origin: "a browseEndpoint run in the description",
		RDF:    "schema:mentions",
	},
	LinksTo: {
		Name: LinksTo, Domain: []string{"video", "post", "channel"}, Range: []string{"external"},
		Origin: "a description run, sameAs in the ld+json",
		RDF:    "schema:citation",
	},
	Tagged: {
		Name: Tagged, Domain: []string{"video"}, Range: []string{"hashtag"},
		Origin: "a hashtag run in the description",
		RDF:    "yt:tagged",
	},
	RelatedTo: {
		Name: RelatedTo, Domain: []string{"video"}, Range: []string{"video"},
		Origin: "the secondaryResults shelf",
		RDF:    "schema:relatedLink",
	},
	CommentsOn: {
		Name: CommentsOn, Domain: []string{"comment"}, Range: []string{"video"},
		Origin: "the containing comment section",
		RDF:    "schema:parentItem",
	},
	Commented: {
		Name: Commented, Domain: []string{"channel"}, Range: []string{"comment"},
		Origin: "the comment author",
		RDF:    "schema:author", Inverse: true,
	},
	RepliesTo: {
		Name: RepliesTo, Domain: []string{"comment"}, Range: []string{"comment"},
		Origin: "the dot in a reply id, no request",
		RDF:    "schema:parentItem",
	},
	Posted: {
		Name: Posted, Domain: []string{"channel"}, Range: []string{"post"},
		Origin: "a community tab entry",
		RDF:    "schema:author", Inverse: true,
	},
	Attaches: {
		Name: Attaches, Domain: []string{"post"}, Range: []string{"video", "playlist", "external"},
		// An image attachment is a URL, so it is an external node. There is no image
		// kind, because a kind that is only ever an address is an address.
		Origin: "the post attachment",
		RDF:    "schema:associatedMedia",
	},
	Shares: {
		Name: Shares, Domain: []string{"post"}, Range: []string{"post"},
		Origin: "a shared post attachment",
		RDF:    "schema:sharedContent",
	},
	Features: {
		Name: Features, Domain: []string{"channel"}, Range: []string{"channel"},
		Origin: "the featured channels shelf",
		RDF:    "yt:features",
	},
	HasCaptions: {
		Name: HasCaptions, Domain: []string{"video"}, Range: []string{"captions"},
		// A caption track has no id of its own, so the object end is the fragment
		// yt://video/<id>#captions/en and "captions" here is the fragment's part
		// name rather than a node kind.
		Origin: "the ANDROID player's caption list",
		RDF:    "schema:caption",
	},
	ChapterOf: {
		Name: ChapterOf, Domain: []string{"chapter"}, Range: []string{"video"},
		Origin: "a macro marker or a description timestamp",
		RDF:    "schema:hasPart", Inverse: true,
	},
	ByArtist: {
		Name: ByArtist, Domain: []string{"album", "video"}, Range: []string{"artist", "channel"},
		// An artist with a UC id is the channel, so either kind is allowed on the
		// object end and the URI says which one this claim got.
		Origin: "the music header byline",
		RDF:    "schema:byArtist",
	},
	InAlbum: {
		Name: InAlbum, Domain: []string{"video"}, Range: []string{"album"},
		Origin: "the album's track list",
		RDF:    "schema:inAlbum",
	},
	SeenAs: {
		Name: SeenAs, Domain: []string{"video"}, Range: []string{"video"},
		// A track is a video, so both ends are videos and there is no track kind.
		// Nothing is written when the music app answers with the id it was asked
		// for: a claim from a node to itself is not a claim. It is written when the
		// answer carries a different id, which is a song that has both an official
		// video and an art track.
		Origin: "the same track read through YouTube Music",
		RDF:    "yt:seenAs",
	},
	InPlaylistAfter: {
		Name: InPlaylistAfter, Domain: []string{"video"}, Range: []string{"video"},
		Origin: "adjacent items in an ordered playlist",
		RDF:    "yt:inPlaylistAfter",
	},
}

// All returns the table sorted by name, which is what ytb predicates prints and
// what the tests iterate, so both are stable between runs.
func All() []PredicateInfo {
	out := make([]PredicateInfo, 0, len(Predicates))
	for _, info := range Predicates {
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Known reports whether a predicate is in the table.
func Known(p Predicate) bool {
	_, ok := Predicates[p]
	return ok
}

// Allows reports whether a URI may sit on the given end of this predicate.
//
// A fragment matches on its part name, so a chapter URI satisfies a "chapter"
// domain and a video URI does not, which is what stops chapter_of being written
// from the video to itself.
func (i PredicateInfo) Allows(u URI, end []string) bool {
	p, ok := Parse(u)
	if !ok {
		return false
	}
	name := string(p.Kind)
	if p.IsFragment() {
		name = p.Part
	}
	return slices.Contains(end, name)
}
