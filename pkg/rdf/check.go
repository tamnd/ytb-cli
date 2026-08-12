package rdf

import (
	"sort"
	"strings"
)

// check.go is the one thing this tool can do that x-cli and facebook-cli could
// not: check its own output against the site's. Doc 04 section 5.1.
//
// A watch page publishes a schema.org VideoObject about itself. Converting both
// that and our own record to triples and comparing them per predicate turns
// "the parser looks right" into a number, and the two rows that made it worth
// building are the ones below: the author URL on the page is plain http on a
// page served over TLS and names a handle rather than a channel id, and
// regionsAllowed matches code for code on a watch page and arrives shuffled on
// a channel page.

// Agreement is how a predicate compared.
type Agreement string

const (
	// Agree means the two say the same thing.
	Agree Agreement = "yes"
	// AgreeNormalised means they say the same thing after the scheme, the
	// trailing slash or the order was taken out of the question.
	AgreeNormalised Agreement = "yes, after normalising"
	// AgreeTruncated means the page said the beginning of what we said and marked
	// where it stopped. The description meta tag cuts off around 160 characters
	// and ends in an ellipsis, so the two texts differ by the site's own cut
	// rather than by their content.
	AgreeTruncated Agreement = "yes, page truncated it"
	// Disagree means both said something and the somethings differ. This is the
	// row worth reading.
	Disagree Agreement = "no"
	// OnlyOurs and OnlyPage mean one side is silent, which is not a disagreement.
	// The page rounds where the payload does not and says nothing at all about
	// most of what this tool reads.
	OnlyOurs Agreement = "page did not say"
	OnlyPage Agreement = "ytb did not say"
)

// Comparison is one predicate compared.
type Comparison struct {
	Predicate string    `json:"predicate"`
	Ours      string    `json:"ytb"`
	Page      string    `json:"page"`
	Agree     Agreement `json:"agree"`
}

// Compare lines two statement lists up by predicate and reports each one.
//
// It compares by predicate rather than by whole triple because that is the
// question being asked: not "are these two documents identical", which they
// never are, but "where the page and this tool both spoke, did they say the
// same thing".
//
// aliases maps an address the site used to the URI this tool uses for the same
// thing, and every entry has to have come from the read itself. Without it the
// author row is a disagreement between yt://channel/UCuAXFkgsw1L7xaCfnd5JJOw
// and http://www.youtube.com/@RickAstleyYT, which is not a disagreement.
func Compare(ours, page []Statement, aliases map[string]string) []Comparison {
	pageBy := groupByPredicate(page)
	// Only the subjects the page spoke about. A watch page read brings back the
	// video, its channel and thirty related videos, and every one of those has a
	// type and an author, so comparing the whole read against a page that
	// describes one video reports thirty one authors against one and calls it a
	// disagreement. It is not one: the page was never asked about the others.
	oursBy := groupByPredicate(about(ours, subjects(page)))

	seen := map[string]bool{}
	var names []string
	for _, m := range []map[string][]string{oursBy, pageBy} {
		for p := range m {
			if !seen[p] {
				seen[p] = true
				names = append(names, p)
			}
		}
	}
	sort.Strings(names)

	out := make([]Comparison, 0, len(names))
	for _, p := range names {
		a, b := oursBy[p], pageBy[p]
		c := Comparison{Predicate: Shorten(p), Ours: join(a), Page: join(b)}
		switch {
		case len(a) == 0:
			c.Agree = OnlyPage
		case len(b) == 0:
			c.Agree = OnlyOurs
		case equalSets(a, b, nil):
			c.Agree = Agree
		case equalSets(a, b, normaliser(aliases)):
			c.Agree = AgreeNormalised
		case truncatedMatch(a, b):
			c.Agree = AgreeTruncated
		default:
			c.Agree = Disagree
		}
		out = append(out, c)
	}
	return out
}

// subjects is the set of things a statement list talks about.
func subjects(in []Statement) map[string]bool {
	out := map[string]bool{}
	for _, st := range in {
		out[st.Subject] = true
	}
	return out
}

// about keeps the statements whose subject is one of the given ones.
func about(in []Statement, keep map[string]bool) []Statement {
	out := in[:0:0]
	for _, st := range in {
		if keep[st.Subject] {
			out = append(out, st)
		}
	}
	return out
}

// truncatedMatch reports whether every page value is a cut off form of one of
// ours: the same text up to an ellipsis, with runs of whitespace collapsed
// because the meta tag writes a description's newlines as spaces.
func truncatedMatch(ours, page []string) bool {
	if len(page) == 0 || len(page) > len(ours) {
		return false
	}
	flat := make([]string, len(ours))
	for i, s := range ours {
		flat[i] = squash(s)
	}
	for _, p := range page {
		head, cut := cutEllipsis(squash(p))
		if !cut || head == "" {
			return false
		}
		found := false
		for _, o := range flat {
			if strings.HasPrefix(o, head) && len(o) > len(head) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// cutEllipsis strips a trailing ellipsis, in either of the two spellings the
// site uses, and says whether there was one.
func cutEllipsis(s string) (string, bool) {
	for _, suffix := range []string{"…", "..."} {
		if head, ok := strings.CutSuffix(s, suffix); ok {
			return strings.TrimSpace(head), true
		}
	}
	return s, false
}

func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

func groupByPredicate(in []Statement) map[string][]string {
	out := map[string][]string{}
	for _, st := range in {
		v := st.Object.Literal
		if st.Object.IsIRI() {
			v = st.Object.IRI
		}
		out[st.Predicate] = append(out[st.Predicate], v)
	}
	for k := range out {
		sort.Strings(out[k])
	}
	return out
}

// equalSets compares two value lists as sets, through norm where one is given.
//
// As sets rather than as lists because regionsAllowed arrives in the page's
// order on a watch page and shuffled on a channel page, and 249 country codes
// in a different order is the same answer.
func equalSets(a, b []string, norm func(string) string) bool {
	if len(a) != len(b) {
		return false
	}
	x, y := append([]string(nil), a...), append([]string(nil), b...)
	if norm != nil {
		for i := range x {
			x[i] = norm(x[i])
		}
		for i := range y {
			y[i] = norm(y[i])
		}
	}
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

// normaliser is normaliseValue with the caller's aliases applied after it, so
// an address the read itself tied to a URI compares equal to that URI.
func normaliser(aliases map[string]string) func(string) string {
	// The keys go through normaliseValue too. The page writes the author as
	// http://www.youtube.com/@RickAstleyYT and the alias was recorded off the
	// payload as https://www.youtube.com/@RickAstleyYT, so a lookup on the raw
	// key misses on a difference this function exists to ignore.
	table := make(map[string]string, len(aliases))
	for k, v := range aliases {
		table[normaliseValue(k)] = v
	}
	return func(s string) string {
		s = normaliseValue(s)
		if uri, ok := table[s]; ok {
			return uri
		}
		return s
	}
}

// normaliseValue takes the differences that are spelling out of a comparison.
//
// Three of them, each measured rather than imagined. The page writes http on a
// page served over TLS and sometimes a www. It writes a duration as PT0M19S on
// one video and PT3M34S on another, so the same nineteen seconds is two strings
// depending on which video you ask about. And it writes a description's
// newlines as a space on one page and as nothing at all on another, which is
// why whitespace is dropped rather than collapsed: comparing "alarming rate" to
// "alarming ratehttps://..." any other way reports a disagreement about line
// endings. The cost is that a title differing only in spacing compares equal,
// which is a trade worth making for a check whose job is to find the rows where
// the two sides really say different things.
func normaliseValue(s string) string {
	s = strings.TrimSpace(s)
	if d, ok := canonicalDuration(s); ok {
		return d
	}
	for _, prefix := range []string{"http://", "https://"} {
		if rest, ok := strings.CutPrefix(s, prefix); ok {
			s = "https://" + strings.TrimPrefix(rest, "www.")
			break
		}
	}
	return squash(strings.TrimSuffix(s, "/"))
}

// canonicalDuration turns an ISO 8601 duration into its seconds, so PT0M19S and
// PT19S are one answer. Anything that is not one of those comes back untouched.
func canonicalDuration(s string) (string, bool) {
	rest, ok := strings.CutPrefix(s, "PT")
	if !ok || rest == "" {
		return "", false
	}
	total, n := 0, 0
	for _, r := range rest {
		switch {
		case r >= '0' && r <= '9':
			n = n*10 + int(r-'0')
		case r == 'H':
			total, n = total+n*3600, 0
		case r == 'M':
			total, n = total+n*60, 0
		case r == 'S':
			total, n = total+n, 0
		default:
			return "", false
		}
	}
	if n != 0 {
		return "", false
	}
	return itoa(total) + "s", true
}

// squash drops whitespace entirely. See normaliseValue for why collapsing it is
// not enough.
func squash(s string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, s)
}

func join(vals []string) string {
	if len(vals) == 0 {
		return ""
	}
	if len(vals) == 1 {
		return clip(vals[0])
	}
	// A list of 249 country codes in a cell nobody can read is worse than a
	// count, and the count is what tells you whether the two agree at a glance.
	if len(vals) > 3 {
		return itoa(len(vals)) + " values"
	}
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = clip(v)
	}
	return strings.Join(parts, ", ")
}

// clip keeps a cell readable. A description is two thousand characters with
// newlines in it and the comparison already happened, so the cell only has to
// show enough to recognise which value it is.
func clip(s string) string {
	s = collapse(s)
	const max = 120
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
