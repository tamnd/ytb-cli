// Package rdf writes claims and records as RDF. Spec 3005 doc 04 section 5.
//
// The vocabulary did not have to be chosen. YouTube publishes schema.org about
// its own pages: a watch page is marked up as a schema.org/VideoObject with
// twenty itemprop values, an author as a Person, a BreadcrumbList for the
// channel and both counts as InteractionCounter blocks. So the mapping here is
// read off the site rather than invented, and where the site says nothing the
// terms are the ones x-cli and facebook-cli already export into, so a store
// from all three tools joins.
//
// A predicate this package has not been taught to translate is written under
// its own name in the yt: namespace rather than dropped, because losing a claim
// in translation loses it silently.
package rdf

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tamnd/ytb-cli/pkg/graph"
)

// The namespaces. yt: is the docs site so a term nobody has seen can be looked
// up rather than guessed at.
const (
	NSSchema = "https://schema.org/"
	NSYT     = "https://tamnd.github.io/ytb-cli/ns#"
	NSRDF    = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
	NSRDFS   = "http://www.w3.org/2000/01/rdf-schema#"
	NSXSD    = "http://www.w3.org/2001/XMLSchema#"
	NSPROV   = "http://www.w3.org/ns/prov#"
)

// Prefixes is the prefix table, in the order Turtle declares it and JSON-LD
// contexts list it. A slice rather than a map so the header is byte-stable.
var Prefixes = []struct{ Prefix, IRI string }{
	{"schema", NSSchema},
	{"yt", NSYT},
	{"rdf", NSRDF},
	{"rdfs", NSRDFS},
	{"xsd", NSXSD},
	{"prov", NSPROV},
}

// Object is the third position of a statement: either an IRI or a literal.
type Object struct {
	IRI     string
	Literal string
	// Datatype is an xsd IRI, empty for a plain string. A count written as a
	// string and a count written as an integer are different to a consumer that
	// sorts them, and every count on this site is a number.
	Datatype string
	// Lang is a BCP 47 tag on a plain literal, for a title that has one.
	Lang string
}

func (o Object) IsIRI() bool { return o.IRI != "" }

// Source is one read that asserted a statement. Client is on it because "the
// ANDROID app was told this" is a materially different statement from "a
// browser was told this", which is the whole reason doc 04 puts client in a
// claim's key.
type Source struct {
	URL    string
	Client string
}

// Statement is one assertion with every read that made it.
//
// A store holds one row per source and client, so a fact four reads agree on
// arrives here four times and RDF cannot say the same thing twice. The
// assertion is written once and the four sources are annotated onto it, which
// keeps the agreement and drops the padding.
type Statement struct {
	Subject   string
	Predicate string
	Object    Object
	Sources   []Source
}

// Key is the assertion without its provenance, which is what deduplicates.
func (s Statement) Key() string {
	return strings.Join([]string{s.Subject, s.Predicate, s.Object.IRI, s.Object.Literal, s.Object.Datatype, s.Object.Lang}, "\x00")
}

// IRIObject and the rest are the constructors, so no caller builds an Object
// with both halves filled.
func IRIObject(iri string) Object { return Object{IRI: iri} }

func StringObject(s string) Object { return Object{Literal: s} }

func TypedObject(s, datatype string) Object { return Object{Literal: s, Datatype: datatype} }

func IntObject(n int64) Object { return TypedObject(fmt.Sprint(n), NSXSD+"integer") }

func BoolObject(b bool) Object { return TypedObject(fmt.Sprint(b), NSXSD+"boolean") }

func DateTimeObject(s string) Object { return TypedObject(s, NSXSD+"dateTime") }

// Class is the rdf:type of a node kind, from the table in doc 04 section 5.
//
// The rows YouTube's own markup covers are the first two: a watch page says
// VideoObject and a channel page's ld+json says Person. The rest are inferred
// and marked so in the spec, because attributing an inference to the page would
// be putting words in its mouth.
func Class(kind graph.Kind) string {
	switch kind {
	case graph.Video:
		return NSSchema + "VideoObject"
	case graph.Channel:
		return NSSchema + "Person"
	case graph.Playlist:
		return NSSchema + "ItemList"
	case graph.Comment:
		return NSSchema + "Comment"
	case graph.Post:
		return NSSchema + "SocialMediaPosting"
	case graph.Album:
		return NSSchema + "MusicAlbum"
	case graph.Artist:
		return NSSchema + "MusicGroup"
	case graph.Hashtag:
		return NSYT + "Hashtag"
	default:
		// External is a URL and its class is whatever it is on the other end of the
		// link, which this tool has not looked at and will not guess.
		return ""
	}
}

// FragmentClass is the rdf:type of a part of a node: a caption track, a chapter,
// a thumbnail rendition, a format.
func FragmentClass(part string) string {
	switch part {
	case graph.FragCaption:
		return NSSchema + "MediaObject"
	case graph.FragChapter:
		return NSSchema + "Clip"
	case graph.FragThumb:
		return NSSchema + "ImageObject"
	case graph.FragFormat:
		// A format is a video track or an audio track and the itag alone does not
		// say which, so the class is the one that covers both. The record carries
		// the mime type where a caller wants the difference.
		return NSSchema + "MediaObject"
	default:
		return ""
	}
}

// Term is the RDF predicate one of doc 04's predicates maps to, and whether the
// triple turns round on the way out.
//
// ytb writes "channel published video" because that is the direction a page
// reads in, and schema:author runs from the work to its author. Getting that
// backwards produces a file that loads without complaint and says a video wrote
// Rick Astley.
func Term(p graph.Predicate) (iri string, inverse bool) {
	info, ok := graph.Predicates[p]
	if !ok {
		// Not dropped. A claim this table has not been taught about is still a
		// claim, and it goes out under its own name in the yt namespace.
		return NSYT + camel(string(p)), false
	}
	return expand(info.RDF), info.Inverse
}

// expand turns a prefixed name from the predicate table into a full IRI.
func expand(curie string) string {
	prefix, local, ok := strings.Cut(curie, ":")
	if !ok {
		return NSYT + curie
	}
	for _, p := range Prefixes {
		if p.Prefix == prefix {
			return p.IRI + local
		}
	}
	return curie
}

// Shorten is expand backwards, for Turtle and for a table a person reads.
func Shorten(iri string) string {
	for _, p := range Prefixes {
		if rest, ok := strings.CutPrefix(iri, p.IRI); ok {
			return p.Prefix + ":" + rest
		}
	}
	return iri
}

// camel turns in_playlist_after into inPlaylistAfter, which is how the yt
// namespace spells the terms doc 04 declares in it.
func camel(s string) string {
	parts := strings.Split(s, "_")
	for i := 1; i < len(parts); i++ {
		if parts[i] == "" {
			continue
		}
		parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
	}
	return strings.Join(parts, "")
}

// FromSet turns claims into statements: one rdf:type per node, one assertion
// per distinct triple, with every source that asserted it.
func FromSet(s *graph.Set) []Statement {
	byKey := map[string]*Statement{}
	var order []string
	add := func(st Statement) {
		k := st.Key()
		cur, ok := byKey[k]
		if !ok {
			cp := st
			byKey[k] = &cp
			order = append(order, k)
			return
		}
		cur.Sources = append(cur.Sources, st.Sources...)
	}

	for _, u := range s.Nodes() {
		p, ok := graph.Parse(u)
		if !ok {
			continue
		}
		class := Class(p.Kind)
		if p.IsFragment() {
			class = FragmentClass(p.Part)
		}
		if class == "" {
			continue
		}
		// No provenance on a type claim. Where the type came off the page's own
		// markup the page said it; where it came off the URI kind this tool
		// inferred it, and citing the page for an inference puts words in its
		// mouth. Doc 04 section 5.
		add(Statement{Subject: string(u), Predicate: NSRDF + "type", Object: IRIObject(class)})
	}

	for _, e := range s.Edges() {
		term, inverse := Term(e.Predicate)
		from, to := string(e.From), string(e.To)
		if inverse {
			from, to = to, from
		}
		add(Statement{
			Subject:   from,
			Predicate: term,
			Object:    IRIObject(to),
			Sources:   []Source{{URL: e.Source, Client: e.Client}},
		})
	}

	out := make([]Statement, 0, len(order))
	for _, k := range order {
		st := *byKey[k]
		st.Sources = dedupeSources(st.Sources)
		out = append(out, st)
	}
	Sort(out)
	return out
}

// dedupeSources drops repeats and sorts, so two runs over the same claims write
// the same bytes.
func dedupeSources(in []Source) []Source {
	if len(in) < 2 {
		return in
	}
	seen := map[Source]bool{}
	out := in[:0:0]
	for _, s := range in {
		if s.URL == "" && s.Client == "" {
			continue
		}
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].URL != out[j].URL {
			return out[i].URL < out[j].URL
		}
		return out[i].Client < out[j].Client
	})
	return out
}

// Sort puts statements in the one order every writer emits them in. A dump that
// reorders itself between two runs over the same input cannot be diffed, and a
// diff is how somebody notices that YouTube started saying something different.
func Sort(in []Statement) {
	sort.SliceStable(in, func(i, j int) bool {
		a, b := in[i], in[j]
		switch {
		case a.Subject != b.Subject:
			return a.Subject < b.Subject
		case a.Predicate != b.Predicate:
			return a.Predicate < b.Predicate
		case a.Object.IRI != b.Object.IRI:
			return a.Object.IRI < b.Object.IRI
		default:
			return a.Object.Literal < b.Object.Literal
		}
	})
}

// Merge folds two statement lists into one, combining the sources of any
// assertion both of them make. This is how a record's literals and a set's
// claims end up in one document.
func Merge(lists ...[]Statement) []Statement {
	byKey := map[string]*Statement{}
	var order []string
	for _, list := range lists {
		for _, st := range list {
			k := st.Key()
			if cur, ok := byKey[k]; ok {
				cur.Sources = dedupeSources(append(cur.Sources, st.Sources...))
				continue
			}
			cp := st
			cp.Sources = dedupeSources(cp.Sources)
			byKey[k] = &cp
			order = append(order, k)
		}
	}
	out := make([]Statement, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	Sort(out)
	return out
}

// ISODuration formats seconds the way the page does, PT3M34S, so our duration
// and the page's compare as strings rather than as two spellings of a number.
func ISODuration(seconds int) string {
	if seconds <= 0 {
		return ""
	}
	h, m, s := seconds/3600, (seconds%3600)/60, seconds%60
	out := "PT"
	if h > 0 {
		out += fmt.Sprintf("%dH", h)
	}
	if m > 0 {
		out += fmt.Sprintf("%dM", m)
	}
	if s > 0 || (h == 0 && m == 0) {
		out += fmt.Sprintf("%dS", s)
	}
	return out
}
