package rdf

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tamnd/ytb-cli/pkg/graph"
)

const (
	testVideo   = "yt://video/dQw4w9WgXcQ"
	testChannel = "yt://channel/UCuAXFkgsw1L7xaCfnd5JJOw"
	testSource  = "https://www.youtube.com/watch?v=dQw4w9WgXcQ"
)

// sample is a small set with the two cases that matter for the writers: an
// inverted predicate and one triple asserted by two different reads.
func sample() *graph.Set {
	set := graph.NewSet()
	page := graph.Provenance{Source: testSource, Surface: "s1", Client: "WEB"}
	feed := graph.Provenance{Source: "https://www.youtube.com/feeds/videos.xml?channel_id=UCuAXFkgsw1L7xaCfnd5JJOw", Surface: "s6"}
	video := graph.VideoURI("dQw4w9WgXcQ")
	channel := graph.ChannelURI("UCuAXFkgsw1L7xaCfnd5JJOw")
	set.Claim(channel, graph.Published, video, page, "Never Gonna Give You Up")
	set.Claim(channel, graph.Published, video, feed, "Never Gonna Give You Up")
	set.Claim(video, graph.Tagged, graph.HashtagURI("rickroll"), page, "")
	set.ClaimAt(video, graph.RelatedTo, graph.VideoURI("fcnDmrtj6Sk"), page, "Dai Dai", 1)
	return set
}

func TestTermTurnsAuthorshipRound(t *testing.T) {
	// The direction is the point. A file that loads without complaint and says a
	// video wrote Rick Astley is worse than one that fails to load.
	for _, p := range []graph.Predicate{graph.Published, graph.Owns, graph.Commented, graph.Posted, graph.ChapterOf} {
		if _, inverse := Term(p); !inverse {
			t.Errorf("%s does not invert, so it would be written from the work's author to the work", p)
		}
	}
	if iri, inverse := Term(graph.Published); iri != NSSchema+"author" || inverse != true {
		t.Errorf("Term(published) = %q, %v", iri, inverse)
	}
	if iri, _ := Term(graph.Contains); iri != NSSchema+"itemListElement" {
		t.Errorf("Term(contains) = %q", iri)
	}
	// A predicate the table has not been taught about is written rather than
	// dropped, because a claim lost in translation is lost silently.
	if iri, inverse := Term(graph.Predicate("not_a_predicate")); iri != NSYT+"notAPredicate" || inverse {
		t.Errorf("Term of an unknown predicate = %q, %v", iri, inverse)
	}
}

func TestEveryPredicateHasATermInANamespaceWeDeclare(t *testing.T) {
	for _, info := range graph.All() {
		iri, _ := Term(info.Name)
		if !strings.HasPrefix(iri, NSSchema) && !strings.HasPrefix(iri, NSYT) {
			t.Errorf("%s maps to %q, which is in no namespace the output declares", info.Name, iri)
		}
		if short := Shorten(iri); short == iri {
			t.Errorf("%s maps to %q, which no prefix shortens, so Turtle would write it long", info.Name, iri)
		}
	}
}

func TestFromSetInvertsAndMergesSources(t *testing.T) {
	got := FromSet(sample())
	var author *Statement
	for i, st := range got {
		if st.Predicate == NSSchema+"author" {
			author = &got[i]
		}
	}
	if author == nil {
		t.Fatal("no schema:author statement came out of a published claim")
	}
	if author.Subject != testVideo || author.Object.IRI != testChannel {
		t.Fatalf("schema:author runs %s -> %s, want video -> channel", author.Subject, author.Object.IRI)
	}
	// One assertion, both reads. Two rows would be RDF saying the same thing
	// twice; one row with one source would throw away the agreement.
	if len(author.Sources) != 2 {
		t.Fatalf("schema:author carries %d sources, want the two reads that asserted it", len(author.Sources))
	}

	// A node gets its class with no provenance on it: the kind was inferred from
	// the URI and citing the page for an inference puts words in its mouth.
	types := 0
	for _, st := range got {
		if st.Predicate == NSRDF+"type" {
			types++
			if len(st.Sources) != 0 {
				t.Errorf("the type of %s cites %d sources", st.Subject, len(st.Sources))
			}
		}
	}
	if types != 4 {
		t.Errorf("got %d type statements, want one per node the claims named", types)
	}
}

func TestWritersAreByteStable(t *testing.T) {
	for _, format := range []Format{NTriples, Turtle, JSONLD} {
		var first, second bytes.Buffer
		for _, buf := range []*bytes.Buffer{&first, &second} {
			// Built from scratch each time, so map iteration inside FromSet gets a
			// chance to order things differently between the two.
			if err := Write(buf, Merge(FromSet(sample())), format, Options{Provenance: true}); err != nil {
				t.Fatalf("%s: %v", format, err)
			}
		}
		if first.String() != second.String() {
			t.Errorf("%s is not byte stable over the same input, so its output cannot be diffed", format)
		}
		if first.Len() == 0 {
			t.Errorf("%s wrote nothing", format)
		}
	}
}

func TestNTriplesCarriesProvenanceAsQuotedTriples(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, FromSet(sample()), NTriples, Options{Provenance: true}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	want := "<< <" + testVideo + "> <" + NSSchema + "author> <" + testChannel + "> >> <" + NSPROV + "wasDerivedFrom> <" + testSource + ">"
	if !strings.Contains(out, want) {
		t.Errorf("no quoted triple for the author claim:\n%s", out)
	}
	if !strings.Contains(out, "<"+NSYT+"client> \"WEB\"") {
		t.Error("the client was dropped, so a WEB read and an ANDROID read look alike")
	}

	var plain bytes.Buffer
	if err := Write(&plain, FromSet(sample()), NTriples, Options{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plain.String(), "<<") {
		t.Error("--no-provenance still wrote quoted triples")
	}
	if plain.Len() >= buf.Len() {
		t.Error("dropping provenance did not make the document smaller")
	}
}

func TestTurtleDeclaresItsPrefixes(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, FromSet(sample()), Turtle, Options{Provenance: true}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, p := range Prefixes {
		if !strings.Contains(out, "@prefix "+p.Prefix+": <"+p.IRI+">") {
			t.Errorf("the %s prefix is used and not declared", p.Prefix)
		}
	}
	if !strings.Contains(out, "schema:author <"+testChannel+">") {
		t.Errorf("the author triple was not shortened:\n%s", out)
	}
}

func TestJSONLDPutsEachSourceInItsOwnGraph(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, FromSet(sample()), JSONLD, Options{Provenance: true}); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Context map[string]string `json:"@context"`
		Graph   []struct {
			ID    string           `json:"@id"`
			Graph []map[string]any `json:"@graph"`
		} `json:"@graph"`
	}
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("the writer produced something that is not JSON: %v\n%s", err, buf.String())
	}
	if doc.Context["schema"] != NSSchema {
		t.Errorf("the context does not declare schema: %+v", doc.Context)
	}
	// JSON-LD has had named graphs since 1.0 and has no quoted triples, so a
	// graph per source is how provenance is spelled here.
	found := false
	for _, g := range doc.Graph {
		if g.ID == testSource+"#claims" {
			found = true
		}
	}
	if !found {
		t.Errorf("no named graph for the watch page, got %d top level entries", len(doc.Graph))
	}
}

func TestCompareReportsEachKindOfAgreement(t *testing.T) {
	src := []Source{{URL: testSource}}
	ours := []Statement{
		{Subject: testVideo, Predicate: NSSchema + "name", Object: StringObject("Never Gonna Give You Up"), Sources: src},
		{Subject: testVideo, Predicate: NSSchema + "author", Object: IRIObject(testChannel), Sources: src},
		{Subject: testVideo, Predicate: NSSchema + "duration", Object: StringObject("PT3M34S"), Sources: src},
		{Subject: testVideo, Predicate: NSSchema + "description", Object: StringObject("The official video\nfor Never Gonna Give You Up by Rick Astley."), Sources: src},
		{Subject: testVideo, Predicate: NSYT + "tagged", Object: IRIObject("yt://hashtag/rickroll"), Sources: src},
		// A related video is a subject the page never spoke about, so nothing about
		// it belongs in the comparison at all.
		{Subject: "yt://video/fcnDmrtj6Sk", Predicate: NSSchema + "author", Object: IRIObject("yt://channel/UCGnjeahCJW1AF34HBmQTJ-Q"), Sources: src},
	}
	page := []Statement{
		{Subject: testVideo, Predicate: NSSchema + "name", Object: StringObject("Never Gonna Give You Up")},
		{Subject: testVideo, Predicate: NSSchema + "author", Object: IRIObject("http://www.youtube.com/@RickAstleyYT")},
		{Subject: testVideo, Predicate: NSSchema + "duration", Object: StringObject("PT4M")},
		{Subject: testVideo, Predicate: NSSchema + "description", Object: StringObject("The official video for Never Gonna Give You Up by Rick ...")},
		{Subject: testVideo, Predicate: NSSchema + "genre", Object: StringObject("Music")},
	}
	aliases := map[string]string{"https://www.youtube.com/@RickAstleyYT": testChannel}

	got := map[string]Agreement{}
	for _, c := range Compare(ours, page, aliases) {
		got[c.Predicate] = c.Agree
	}
	want := map[string]Agreement{
		"schema:name":        Agree,
		"schema:author":      AgreeNormalised,
		"schema:duration":    Disagree,
		"schema:description": AgreeTruncated,
		"schema:genre":       OnlyPage,
		"yt:tagged":          OnlyOurs,
	}
	for pred, w := range want {
		if got[pred] != w {
			t.Errorf("%s compared as %q, want %q", pred, got[pred], w)
		}
	}
	if len(got) != len(want) {
		t.Errorf("compared %d predicates, want %d: a subject the page never mentioned is not a disagreement", len(got), len(want))
	}
}

func TestCompareWithoutAnAliasIsADisagreement(t *testing.T) {
	// An alias has to come from the read. Without one the tool has no evidence
	// that the handle and the channel id are the same thing, and saying so anyway
	// would be a guess dressed up as agreement.
	ours := []Statement{{Subject: testVideo, Predicate: NSSchema + "author", Object: IRIObject(testChannel)}}
	page := []Statement{{Subject: testVideo, Predicate: NSSchema + "author", Object: IRIObject("http://www.youtube.com/@RickAstleyYT")}}
	got := Compare(ours, page, nil)
	if len(got) != 1 || got[0].Agree != Disagree {
		t.Fatalf("got %+v, want one disagreement", got)
	}
}

func TestCompareTreatsRegionsAsASet(t *testing.T) {
	// regionsAllowed comes back in the page's order on a watch page and shuffled
	// on a channel page, and 249 country codes in another order is one answer.
	var ours, page []Statement
	for _, c := range []string{"US", "GB", "VN", "DE"} {
		ours = append(ours, Statement{Subject: testVideo, Predicate: NSSchema + "regionsAllowed", Object: StringObject(c)})
	}
	for _, c := range []string{"VN", "US", "DE", "GB"} {
		page = append(page, Statement{Subject: testVideo, Predicate: NSSchema + "regionsAllowed", Object: StringObject(c)})
	}
	got := Compare(ours, page, nil)
	if len(got) != 1 || got[0].Agree != Agree {
		t.Fatalf("got %+v, want agreement", got)
	}
	// Past three the cell is a count, because 249 country codes in a column
	// nobody can read is worse than the number that answers the question.
	if got[0].Ours != "4 values" {
		t.Errorf("a long list was written out in full: %q", got[0].Ours)
	}
}

func TestCompareNormalisesTheSpellingsTheSiteActuallyUses(t *testing.T) {
	// Both rows are off real pages. jNQXAC9IVRw is marked up as PT0M19S where
	// dQw4w9WgXcQ is marked up as PT3M34S, and jNQXAC9IVRw's description meta
	// joins the lines with nothing where dQw4w9WgXcQ's joins them with a space.
	ours := []Statement{
		{Subject: testVideo, Predicate: NSSchema + "duration", Object: StringObject("PT19S")},
		{Subject: testVideo, Predicate: NSSchema + "description", Object: StringObject("alarming rate\nhttps://www.youtube.com/watch?v=0PT5c1z3LL8\nNanoplastics")},
	}
	page := []Statement{
		{Subject: testVideo, Predicate: NSSchema + "duration", Object: StringObject("PT0M19S")},
		{Subject: testVideo, Predicate: NSSchema + "description", Object: StringObject("alarming ratehttps://www.youtube.com/watch?v=0PT5c1z3LL8Nanoplastics")},
	}
	for _, c := range Compare(ours, page, nil) {
		if c.Agree != AgreeNormalised {
			t.Errorf("%s compared as %q, want agreement after normalising", c.Predicate, c.Agree)
		}
	}
	// A duration that really differs still has to come back as a disagreement.
	longer := []Statement{{Subject: testVideo, Predicate: NSSchema + "duration", Object: StringObject("PT20S")}}
	if got := Compare(longer, page[:1], nil); got[0].Agree != Disagree {
		t.Errorf("PT20S against PT0M19S compared as %q", got[0].Agree)
	}
}

func TestISODurationMatchesThePage(t *testing.T) {
	cases := map[int]string{
		214:  "PT3M34S",
		3600: "PT1H",
		3661: "PT1H1M1S",
		59:   "PT59S",
		0:    "",
		-1:   "",
	}
	for secs, want := range cases {
		if got := ISODuration(secs); got != want {
			t.Errorf("ISODuration(%d) = %q, want %q", secs, got, want)
		}
	}
}

func TestShortenIsExpandBackwards(t *testing.T) {
	for _, p := range Prefixes {
		iri := p.IRI + "thing"
		if got := Shorten(iri); got != p.Prefix+":thing" {
			t.Errorf("Shorten(%q) = %q", iri, got)
		}
		if got := expand(p.Prefix + ":thing"); got != iri {
			t.Errorf("expand(%q) = %q", p.Prefix+":thing", got)
		}
	}
	// A term in no declared namespace comes back untouched rather than mangled.
	if got := Shorten("https://example.com/term"); got != "https://example.com/term" {
		t.Errorf("Shorten of a foreign IRI = %q", got)
	}
}
