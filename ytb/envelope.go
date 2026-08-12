package ytb

import (
	"fmt"
	"strings"
	"time"
)

// envelope.go is the provenance block every record carries. Doc 03 section 1.
//
// A record without one is a claim with no evidence. Two reads of the same video
// carry different fields and sometimes different numbers for the same field: the
// watch page has the exact view count and no comment count, the Atom feed has an
// exact publication timestamp for the newest fifteen uploads and nothing else,
// and a search lockup has a rounded "36K views" and no description at all. With
// nothing on the record to say which surfaces answered, a listing row and a full
// read look the same, and a rounded number looks like an exact one.
//
// The eight fields are not omitempty, on purpose. An empty surfaces list is a bug
// and should be visible as one, and an empty missed list is a claim that the read
// got everything its surfaces carry, which is a different statement from having
// said nothing.

// Surface ids from doc 01. A read names its evidence with a constant so a typo is
// a build failure rather than an "s2" that quietly means nothing.
const (
	// SurfaceWatchHTML is the watch page: ytInitialPlayerResponse, ytInitialData,
	// ytcfg and the page's own schema.org microdata, in one 1.3 MB response.
	SurfaceWatchHTML = "s1"
	// SurfaceInnerTube is the web InnerTube API: browse, next, search, player.
	SurfaceInnerTube = "s2"
	// SurfaceMobilePlayer is /player as ANDROID or ANDROID_VR, the only source of
	// stream URLs and of caption baseUrl values that return bytes.
	SurfaceMobilePlayer = "s3"
	// SurfaceBrowseHTML is a channel or playlist page as HTML.
	SurfaceBrowseHTML = "s4"
	// SurfaceOEmbed is /oembed, which answers with a handle and nothing else.
	SurfaceOEmbed = "s5"
	// SurfaceFeed is the channel Atom feed: the fifteen newest uploads with exact
	// timestamps and exact view counts.
	SurfaceFeed = "s6"
	// SurfaceThumbCDN is i.ytimg.com.
	SurfaceThumbCDN = "s7"
	// SurfaceMediaCDN is googlevideo.com, ranged only.
	SurfaceMediaCDN = "s8"
	// SurfaceSuggest is the suggestqueries JSONP autocomplete.
	SurfaceSuggest = "s9"
	// SurfaceMusic is music.youtube.com InnerTube as WEB_REMIX.
	SurfaceMusic = "s10"
	// SurfaceSession is a read that carried the user's cookies. It is the only
	// tier 1 surface.
	SurfaceSession = "s11"
)

// SurfaceInfo is one row of doc 01's surface table: an id, the constant that
// spells it, what the read is, and whether it needs the user's cookies.
type SurfaceInfo struct {
	ID       string `json:"id"`
	Constant string `json:"constant"`
	Host     string `json:"host"`
	Tier     int    `json:"tier"`
	Summary  string `json:"summary"`
}

// SurfaceTable is every surface this tool reads, in id order.
//
// The table lived only in the invariant test, which meant the eleven ids a
// record can name were written down in a file the binary does not ship. A
// person holding a record with "s3" on it had to read the source to find out
// what answered.
func SurfaceTable() []SurfaceInfo {
	return []SurfaceInfo{
		{SurfaceWatchHTML, "SurfaceWatchHTML", "www.youtube.com", 0,
			"the watch page: ytInitialPlayerResponse, ytInitialData, ytcfg and the schema.org microdata, in one response"},
		{SurfaceInnerTube, "SurfaceInnerTube", "www.youtube.com", 0,
			"the web InnerTube API: browse, next, search and player"},
		{SurfaceMobilePlayer, "SurfaceMobilePlayer", "www.youtube.com", 0,
			"/player as ANDROID or ANDROID_VR, the only source of stream URLs and of caption URLs that return bytes"},
		{SurfaceBrowseHTML, "SurfaceBrowseHTML", "www.youtube.com", 0,
			"a channel or playlist page as HTML"},
		{SurfaceOEmbed, "SurfaceOEmbed", "www.youtube.com", 0,
			"/oembed, which answers with a handle and little else"},
		{SurfaceFeed, "SurfaceFeed", "www.youtube.com", 0,
			"the channel Atom feed: the fifteen newest uploads with exact timestamps"},
		{SurfaceThumbCDN, "SurfaceThumbCDN", "i.ytimg.com", 0,
			"the thumbnail CDN, which is asked whether a rendition exists rather than parsed"},
		{SurfaceMediaCDN, "SurfaceMediaCDN", "googlevideo.com", 0,
			"the media CDN, read in byte ranges and never whole"},
		{SurfaceSuggest, "SurfaceSuggest", "suggestqueries-clients6.youtube.com", 0,
			"the JSONP autocomplete endpoint"},
		{SurfaceMusic, "SurfaceMusic", "music.youtube.com", 0,
			"music.youtube.com InnerTube as WEB_REMIX"},
		{SurfaceSession, "SurfaceSession", "www.youtube.com", 1,
			"a read that carried the user's cookies, and the only surface that is not anonymous"},
	}
}

// Surfaces is the envelope's surface list, named so it can say how it prints.
//
// kit renders a slice column as its length, which is the right default: a table
// cell holding 27 keywords is a table with one column. For surfaces it is exactly
// wrong. "1" is not an answer to which surface answered, and a reader would take it
// for surface 1. A record read from the watch page has to print s1.
type Surfaces []string

// String joins the ids for the table, and is what kit calls in preference to the
// slice default.
func (s Surfaces) String() string {
	return strings.Join(s, ", ")
}

// Has reports whether the given surface answered.
func (s Surfaces) Has(id string) bool {
	for _, got := range s {
		if got == id {
			return true
		}
	}
	return false
}

// Envelope is embedded in a record rather than nested under a key, so its fields
// flatten into the record's own JSON the way encoding/json flattens an anonymous
// struct, and into the table's columns the same way.
type Envelope struct {
	// Kind is the record kind, one of the twelve in doc 03. It is not a column
	// because every row of one read has the same kind and the command already said
	// which.
	Kind string `json:"kind" table:"-"`
	// Tier is 0 for a signed out read and 1 when a session cookie contributed. It
	// is the highest tier of any read that fed this record, not the tier of the last
	// one, because one authenticated call is enough to make the record unreproducible
	// without cookies.
	Tier int `json:"tier" table:"tier"`
	// Surfaces are the surface ids that answered, in read order.
	Surfaces Surfaces `json:"surfaces" table:"surfaces"`
	// Sources are the URLs read, deduplicated, in read order.
	Sources []string `json:"sources" table:"-"`
	// Client is every InnerTube client context claimed, e.g. WEB then ANDROID.
	Client []string `json:"client" table:"-"`
	// Via names the surface behind a field that more than one surface could have
	// supplied, or that one surface spells two ways.
	//
	// A value is a surface id, and where the disagreement is between two blocks of
	// the same surface it is the id followed by the block. duration_seconds is
	// "s1 microformat.lengthSeconds", because videoDetails on the same page says 213
	// where microformat says 214, and "s1" on its own would not say which won.
	Via map[string]string `json:"via" table:"-"`
	// Missed is what this read did not see, in sentences rather than codes, because
	// it is read by a person deciding whether to fetch more. Empty means the read
	// believes it got everything its surfaces carry. It never means the thing has
	// nothing.
	Missed []string `json:"missed" table:"-"`
	// FetchedAt is when the first request of the read went out.
	FetchedAt time.Time `json:"fetched_at" table:"-"`
}

// newEnvelope stamps a record with its kind and the surfaces that answered.
//
// The slices and the map are allocated rather than left nil, so a read that saw
// no sources serialises "sources": [] instead of null. Those are different
// claims: [] says the read tracked its sources and there were none to add,
// null says nobody was keeping count.
func newEnvelope(kind string, surfaces ...string) Envelope {
	e := Envelope{
		Kind:      kind,
		Surfaces:  Surfaces{},
		Sources:   []string{},
		Client:    []string{},
		Via:       map[string]string{},
		Missed:    []string{},
		FetchedAt: time.Now(),
	}
	for _, s := range surfaces {
		e.addSurface(s)
	}
	return e
}

// addSurface records that a surface answered. Order is read order and a repeat is
// dropped, because a read that calls /player twice used one surface.
func (e *Envelope) addSurface(id string) {
	if id == "" {
		return
	}
	e.Surfaces = appendOnce(e.Surfaces, id)
}

// addSource records a URL that was read.
func (e *Envelope) addSource(url string) {
	if url == "" {
		return
	}
	e.Sources = appendOnce(e.Sources, url)
}

// addClient records an InnerTube client context that was claimed.
func (e *Envelope) addClient(name string) {
	if name == "" {
		return
	}
	e.Client = appendOnce(e.Client, name)
}

// setVia names where a field's value came from. See the Via field for why some
// values carry a block name after the surface id.
func (e *Envelope) setVia(field, source string) {
	if e.Via == nil {
		e.Via = map[string]string{}
	}
	e.Via[field] = source
}

// miss records something this read did not see, as a sentence.
func (e *Envelope) miss(format string, args ...any) {
	e.Missed = append(e.Missed, fmt.Sprintf(format, args...))
}

// raiseTier lifts the record to tier n if n is higher. A read never drops a tier
// it already claimed.
func (e *Envelope) raiseTier(n int) {
	if n > e.Tier {
		e.Tier = n
	}
}

// envelope returns the envelope itself, which is how a record hands its own
// out. Every record embeds Envelope, so every record's pointer has this method
// and the reads can stamp one without knowing which type it is holding.
func (e *Envelope) envelope() *Envelope { return e }

// appendOnce appends s to list unless it is already there, keeping order.
func appendOnce(list []string, s string) []string {
	for _, existing := range list {
		if existing == s {
			return list
		}
	}
	return append(list, s)
}
