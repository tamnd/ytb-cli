package rdf

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// write.go is the three output formats. Doc 04 section 5.
//
// How provenance is spelled differs by format, because a statement about a
// statement is the one thing RDF has never had one obvious spelling for.
// N-Triples and Turtle use the quoted-triple form, << s p o >>
// prov:wasDerivedFrom <url>, at one line per claim. JSON-LD uses named graphs,
// one per source, because JSON-LD has had those since 1.0 and has no quoted
// triples at all.

// Options controls the writers.
type Options struct {
	// Provenance off drops the source annotations, which is most of the bytes.
	// With it off there is nothing left to tell four agreeing reads apart, so
	// they collapse to one assertion.
	Provenance bool
}

// Format is an output syntax.
type Format string

const (
	NTriples Format = "nt"
	Turtle   Format = "turtle"
	JSONLD   Format = "jsonld"
)

// Formats is what --format accepts, for the flag's help and for validation.
var Formats = []Format{NTriples, Turtle, JSONLD}

// Write dispatches on the format.
func Write(w io.Writer, statements []Statement, f Format, opt Options) error {
	switch f {
	case NTriples, "":
		return WriteNT(w, statements, opt)
	case Turtle:
		return WriteTurtle(w, statements, opt)
	case JSONLD:
		return WriteJSONLD(w, statements, opt)
	default:
		return fmt.Errorf("unknown rdf format %q, one of nt, turtle or jsonld", f)
	}
}

// WriteNT writes N-Triples, which streams: one statement per line, no header,
// nothing held in memory that the caller did not already hold.
func WriteNT(w io.Writer, statements []Statement, opt Options) error {
	for _, st := range statements {
		line := fmt.Sprintf("%s %s %s .\n", iri(st.Subject), iri(st.Predicate), object(st.Object))
		if _, err := io.WriteString(w, line); err != nil {
			return err
		}
		if !opt.Provenance {
			continue
		}
		quoted := fmt.Sprintf("<< %s %s %s >>", iri(st.Subject), iri(st.Predicate), object(st.Object))
		for _, s := range st.Sources {
			if s.URL != "" {
				if _, err := fmt.Fprintf(w, "%s %s %s .\n", quoted, iri(NSPROV+"wasDerivedFrom"), iri(s.URL)); err != nil {
					return err
				}
			}
			if s.Client != "" {
				if _, err := fmt.Fprintf(w, "%s %s %s .\n", quoted, iri(NSYT+"client"), literal(s.Client)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// WriteTurtle writes Turtle: the same statements with the prefixes declared and
// each subject written once.
func WriteTurtle(w io.Writer, statements []Statement, opt Options) error {
	for _, p := range Prefixes {
		if _, err := fmt.Fprintf(w, "@prefix %s: <%s> .\n", p.Prefix, p.IRI); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, "\n"); err != nil {
		return err
	}

	// Statements arrive sorted by subject, so a subject's block is a run.
	for i := 0; i < len(statements); {
		j := i
		for j < len(statements) && statements[j].Subject == statements[i].Subject {
			j++
		}
		if _, err := fmt.Fprintf(w, "%s\n", term(statements[i].Subject)); err != nil {
			return err
		}
		for k := i; k < j; k++ {
			sep := " ;"
			if k == j-1 {
				sep = " ."
			}
			if _, err := fmt.Fprintf(w, "    %s %s%s\n", term(statements[k].Predicate), objectTerm(statements[k].Object), sep); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(w, "\n"); err != nil {
			return err
		}
		i = j
	}

	if !opt.Provenance {
		return nil
	}
	// The provenance goes after the data rather than inside it: a quoted triple
	// cannot be a subject inside a predicate list, and a reader who does not care
	// about provenance can stop at the blank line.
	for _, st := range statements {
		quoted := fmt.Sprintf("<< %s %s %s >>", term(st.Subject), term(st.Predicate), objectTerm(st.Object))
		for _, s := range st.Sources {
			if s.URL != "" {
				if _, err := fmt.Fprintf(w, "%s prov:wasDerivedFrom %s .\n", quoted, term(s.URL)); err != nil {
					return err
				}
			}
			if s.Client != "" {
				if _, err := fmt.Fprintf(w, "%s yt:client %s .\n", quoted, literal(s.Client)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// WriteJSONLD writes JSON-LD with the context inline, so a consumer needs
// nothing from the network to read it.
//
// With provenance on, every source is its own named graph and an assertion four
// reads agree on appears in four of them, because there the agreement is the
// point: dropping three would drop it. With provenance off there is one default
// graph and the copies collapse.
func WriteJSONLD(w io.Writer, statements []Statement, opt Options) error {
	ctx := map[string]any{}
	for _, p := range Prefixes {
		ctx[p.Prefix] = p.IRI
	}

	doc := map[string]any{"@context": ctx}

	if !opt.Provenance {
		doc["@graph"] = jsonNodes(statements)
		return encodeJSON(w, doc)
	}

	// Group by source URL. A claim with no source at all, which is what an
	// inferred rdf:type is, goes in the default graph.
	byGraph := map[string][]Statement{}
	var names []string
	for _, st := range statements {
		if len(st.Sources) == 0 {
			if _, ok := byGraph[""]; !ok {
				names = append(names, "")
			}
			byGraph[""] = append(byGraph[""], st)
			continue
		}
		for _, s := range st.Sources {
			// The graph gets its own IRI rather than borrowing the page's: a page and
			// the claims read off it are two things, and naming them both <source>
			// turns the provenance into "source was derived from source", which is
			// true and useless.
			name := s.URL + "#claims"
			if _, ok := byGraph[name]; !ok {
				names = append(names, name)
			}
			cp := st
			cp.Sources = []Source{s}
			byGraph[name] = append(byGraph[name], cp)
		}
	}
	sort.Strings(names)

	graphs := make([]any, 0, len(names))
	for _, name := range names {
		nodes := jsonNodes(byGraph[name])
		if name == "" {
			graphs = append(graphs, map[string]any{"@graph": nodes})
			continue
		}
		entry := map[string]any{
			"@id":                 name,
			"prov:wasDerivedFrom": map[string]any{"@id": strings.TrimSuffix(name, "#claims")},
			"@graph":              nodes,
		}
		if client := byGraph[name][0].Sources[0].Client; client != "" {
			entry["yt:client"] = client
		}
		graphs = append(graphs, entry)
	}
	doc["@graph"] = graphs
	return encodeJSON(w, doc)
}

// jsonNodes folds statements sharing a subject into one JSON-LD node object,
// which is what makes the output readable rather than a list of triples in
// JSON's clothing.
func jsonNodes(statements []Statement) []any {
	var order []string
	nodes := map[string]map[string]any{}
	for _, st := range statements {
		node, ok := nodes[st.Subject]
		if !ok {
			node = map[string]any{"@id": st.Subject}
			nodes[st.Subject] = node
			order = append(order, st.Subject)
		}
		key := Shorten(st.Predicate)
		if st.Predicate == NSRDF+"type" {
			key = "@type"
		}
		var val any
		switch {
		case st.Predicate == NSRDF+"type":
			val = Shorten(st.Object.IRI)
		case st.Object.IsIRI():
			val = map[string]any{"@id": st.Object.IRI}
		case st.Object.Datatype != "":
			val = map[string]any{"@value": st.Object.Literal, "@type": Shorten(st.Object.Datatype)}
		case st.Object.Lang != "":
			val = map[string]any{"@value": st.Object.Literal, "@language": st.Object.Lang}
		default:
			val = st.Object.Literal
		}
		switch cur := node[key].(type) {
		case nil:
			node[key] = val
		case []any:
			node[key] = append(cur, val)
		default:
			node[key] = []any{cur, val}
		}
	}
	out := make([]any, 0, len(order))
	for _, id := range order {
		out = append(out, nodes[id])
	}
	return out
}

func encodeJSON(w io.Writer, doc any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	// Turned off because a yt:// URI and a description both contain characters
	// Go would otherwise escape into <, which is valid JSON nobody can read.
	enc.SetEscapeHTML(false)
	return enc.Encode(doc)
}

// iri writes an absolute IRI in N-Triples form.
func iri(s string) string { return "<" + escapeIRI(s) + ">" }

// term writes an IRI in Turtle form, shortened to a prefixed name where one of
// the declared prefixes covers it.
func term(s string) string {
	if short := Shorten(s); short != s && !strings.ContainsAny(short, " \t()[]{},;") {
		return short
	}
	return iri(s)
}

func object(o Object) string {
	if o.IsIRI() {
		return iri(o.IRI)
	}
	return literalWith(o, iri)
}

func objectTerm(o Object) string {
	if o.IsIRI() {
		return term(o.IRI)
	}
	return literalWith(o, term)
}

func literal(s string) string { return `"` + escapeLiteral(s) + `"` }

func literalWith(o Object, wrap func(string) string) string {
	out := literal(o.Literal)
	switch {
	case o.Datatype != "":
		out += "^^" + wrap(o.Datatype)
	case o.Lang != "":
		out += "@" + o.Lang
	}
	return out
}

// escapeIRI removes the characters an IRI may not contain. A YouTube URL with a
// space in a query parameter is rare and does happen, and an unescaped one
// produces a file no parser will load.
func escapeIRI(s string) string {
	r := strings.NewReplacer(
		"<", "%3C", ">", "%3E", `"`, "%22", " ", "%20",
		"{", "%7B", "}", "%7D", "|", "%7C", "\\", "%5C", "^", "%5E", "`", "%60",
		"\n", "", "\r", "",
	)
	return r.Replace(s)
}

func escapeLiteral(s string) string {
	r := strings.NewReplacer(
		"\\", `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`,
	)
	return r.Replace(s)
}
