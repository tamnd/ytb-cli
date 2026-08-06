package graph

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// edge.go is the claim. Doc 04 section 3.
//
// An edge is not a fact, it is an observation: subject, predicate, object, and
// who said so. The source is part of the claim's identity rather than metadata
// hanging off it, so two surfaces asserting the same edge stay two rows and a
// disagreement between them is queryable rather than lost to whichever read
// happened to run last. That rule came from x-cli, earned its keep in
// facebook-cli, and stays.
//
// Client is new in this tool. The same URL answers differently depending on
// which app ytb said it was: /player as WEB returns UNPLAYABLE with no caption
// tracks and as ANDROID returns six of them, on the same video in the same
// minute. So an edge read as ANDROID and the same edge read as WEB are two
// observations, and collapsing them would lose the fact that only one of the
// two carries anything.

// Edge is one claim.
type Edge struct {
	From      URI       `json:"from" table:"from,truncate"`
	Predicate Predicate `json:"predicate" table:"predicate"`
	To        URI       `json:"to" table:"to,truncate"`
	// Source is the URL that asserted it. An InnerTube POST carries the operation
	// as a fragment, .../youtubei/v1/browse#UUuAXFkgsw1L7..., because thirty
	// browses of one URL in a log that cannot tell them apart is a log nobody can
	// check a record against. A fragment is never sent to a server, so writing one
	// down invents nothing about the request.
	Source string `json:"source" table:"-"`
	// Surface is s1..s11 from doc 01, or "derived" on the handful of claims the id
	// space asserts on its own. A UU playlist id names its channel and no request
	// was made to learn that, so writing s2 there would credit a read that never
	// happened.
	Surface string `json:"surface" table:"-"`
	// Client is WEB, ANDROID, WEB_REMIX, or empty for a plain HTML read.
	Client string `json:"client" table:"-"`
	// Tier is 0 for anonymous and 1 for a session, from doc 00.
	Tier int `json:"tier" table:"-"`
	// Note is a human label for the object end, and it is why the table output is
	// readable. Three yt:// URIs on a row tell nobody anything; the same row with
	// a name on the end tells them what they came for. It is never part of the
	// claim's identity, because a title changes and the claim does not.
	Note string `json:"note,omitempty" table:"note,truncate"`
	// Position orders the claims a single ordered read produced, so a playlist's
	// contains edges keep the playlist's order. Zero on unordered claims.
	Position int `json:"position,omitempty" table:"-"`
}

// SurfaceDerived is the surface of a claim no request produced: the id space
// asserted it. Doc 01 numbers the surfaces s1 to s11 and this is deliberately
// not one of them.
const SurfaceDerived = "derived"

// ErrUnknownPredicate is refused rather than written, because a claim under a
// misspelled predicate is never queried and never noticed.
var ErrUnknownPredicate = errors.New("not a predicate in doc 04 section 3.1")

// NewEdge builds a claim and checks it against the table.
//
// It refuses an unknown predicate, an empty end and an end of the wrong kind.
// The empty end matters most in practice: ChannelURI returns "" for a handle,
// and without this check a channel that was only ever named by its @handle
// would produce a claim pointing at yt://channel/, which is a node that exists,
// collects edges from every unresolved handle in the store, and means nothing.
func NewEdge(from URI, p Predicate, to URI, prov Provenance) (Edge, error) {
	info, ok := Predicates[p]
	if !ok {
		return Edge{}, fmt.Errorf("%q: %w", p, ErrUnknownPredicate)
	}
	if from == "" || to == "" {
		return Edge{}, fmt.Errorf("%s: an end is empty, which usually means a handle was not resolved", p)
	}
	if !info.Allows(from, info.Domain) {
		return Edge{}, fmt.Errorf("%s does not run from %s, its domain is %s", p, from, strings.Join(info.Domain, " or "))
	}
	if !info.Allows(to, info.Range) {
		return Edge{}, fmt.Errorf("%s does not run to %s, its range is %s", p, to, strings.Join(info.Range, " or "))
	}
	return Edge{
		From: from, Predicate: p, To: to,
		Source: prov.Source, Surface: prov.Surface, Client: prov.Client, Tier: prov.Tier,
	}, nil
}

// Provenance is the observation half of a claim, carried separately because one
// read produces many edges and they all share it.
type Provenance struct {
	Source  string
	Surface string
	Client  string
	Tier    int
}

// Key is a claim's identity: the triple plus the source and the client. Two
// rows differing only by client are two observations, so client is in the key
// and not beside it.
func (e Edge) Key() string {
	return strings.Join([]string{string(e.From), string(e.Predicate), string(e.To), e.Source, e.Client}, "\x00")
}

// Triple is the claim without the observation, which is what deduplicates on
// the way into RDF: a fact four reads agree on is one assertion with four
// sources on it, not four assertions.
func (e Edge) Triple() string {
	return strings.Join([]string{string(e.From), string(e.Predicate), string(e.To)}, "\x00")
}

// Set collects edges, dropping exact repeats and keeping the first note seen.
//
// A set rather than a slice because one watch page asserts the same channel
// published the same video from videoDetails, from the owner renderer and from
// the microdata, and three identical rows in ytb edges is noise. Two different
// sources for the same triple are not repeats and both are kept.
type Set struct {
	seen  map[string]int
	edges []Edge
}

func NewSet() *Set { return &Set{seen: map[string]int{}} }

// Add appends an edge, or merges it into the one already there.
func (s *Set) Add(e Edge) {
	if s.seen == nil {
		s.seen = map[string]int{}
	}
	if i, ok := s.seen[e.Key()]; ok {
		// A later sighting with a note is better than an earlier one without: the
		// claim is the same, and the note is the only thing on the row a person
		// reads.
		if s.edges[i].Note == "" && e.Note != "" {
			s.edges[i].Note = e.Note
		}
		return
	}
	s.seen[e.Key()] = len(s.edges)
	s.edges = append(s.edges, e)
}

// Claim builds an edge and adds it, returning whether it was accepted.
//
// The error is swallowed on purpose and this is the only place that does it.
// Parsers run over payloads full of half-populated renderers, and a lockup with
// no channel id on it is a normal lockup rather than a bug, so a parser that
// stopped at the first unbuildable edge would return nothing for a page that
// mostly parsed. What the caller gets instead is a count: how many claims came
// off this read.
func (s *Set) Claim(from URI, p Predicate, to URI, prov Provenance, note string) bool {
	return s.ClaimAt(from, p, to, prov, note, 0)
}

// ClaimAt is Claim with the position an ordered read gave the claim, so a
// playlist's contains edges keep the playlist's order without a second
// predicate to hold it.
func (s *Set) ClaimAt(from URI, p Predicate, to URI, prov Provenance, note string, position int) bool {
	e, err := NewEdge(from, p, to, prov)
	if err != nil {
		return false
	}
	e.Note = note
	e.Position = position
	s.Add(e)
	return true
}

// Edges returns the claims in a stable order: by subject, then predicate, then
// position, then object. Stable because a dump that reorders itself between two
// runs over the same input cannot be diffed, and a diff is how somebody notices
// that YouTube started saying something different.
func (s *Set) Edges() []Edge {
	out := make([]Edge, len(s.edges))
	copy(out, s.edges)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		switch {
		case a.From != b.From:
			return a.From < b.From
		case a.Predicate != b.Predicate:
			return a.Predicate < b.Predicate
		case a.Position != b.Position:
			return a.Position < b.Position
		case a.To != b.To:
			return a.To < b.To
		case a.Source != b.Source:
			return a.Source < b.Source
		default:
			return a.Client < b.Client
		}
	})
	return out
}

func (s *Set) Len() int { return len(s.edges) }

// Nodes returns every URI any claim named, in a stable order.
//
// This is the frontier. Most of what comes back was never fetched: one watch
// page names a channel, twenty related videos and a handful of links, and a
// store that holds only what was read cannot say what it has not looked at.
func (s *Set) Nodes() []URI {
	seen := map[URI]bool{}
	var out []URI
	for _, e := range s.Edges() {
		for _, u := range []URI{e.From, e.To} {
			if !seen[u] {
				seen[u] = true
				out = append(out, u)
			}
		}
	}
	slices.Sort(out)
	return out
}

// CountByPredicate is what ytb graph summarises and what the tests hold to a
// floor, so a parser that quietly stops emitting one kind of claim is caught by
// the number rather than by somebody noticing months later.
func (s *Set) CountByPredicate() map[Predicate]int {
	out := map[Predicate]int{}
	for _, e := range s.edges {
		out[e.Predicate]++
	}
	return out
}
