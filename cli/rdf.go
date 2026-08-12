package cli

import (
	"context"
	"fmt"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/ytb-cli/pkg/rdf"
	"github.com/tamnd/ytb-cli/ytb"
)

// rdf.go writes the graph plane in a vocabulary something else can read, and
// checks it against the site's own. Spec 3005 doc 04 section 5.

func newRDFCmd() kit.Command {
	var (
		o        claimFlags
		format   string
		noProv   bool
		check    bool
		contexts bool
	)
	return kit.Command{
		Use:   "rdf <ref>...",
		Group: "read",
		Short: "The same claims as n-triples, turtle or json-ld, with provenance",
		Long: `Read anything YouTube has an id for and write it as RDF.

The vocabulary was not invented here. YouTube publishes schema.org about its own
pages: a watch page is marked up as a schema.org/VideoObject with twenty itemprop
values, an author as a Person and both counts as InteractionCounter blocks, and
a channel page carries a ProfilePage in ld+json. So the mapping is read off the
site. Where the site says nothing the terms are the ones x-cli and facebook-cli
already export into, so a store from all three tools joins, and a predicate with
no schema.org equivalent goes in the yt namespace declared in the output rather
than assumed.

Provenance is not dropped in translation. Every claim carries prov:wasDerivedFrom
naming the URL, and where it depended on a claimed client it carries yt:client
too, because "the ANDROID app was told this" is a different statement from "a
browser was told this". That is most of the bytes and --no-provenance turns it
off.

How that is spelled differs by format, because a statement about a statement is
the one thing RDF has never had one obvious spelling for. nt and turtle use the
quoted triple form, << s p o >> prov:wasDerivedFrom <url>. json-ld uses a named
graph per source, since it has had those since 1.0 and has no quoted triples.

--check is the one thing this tool can do that the others could not: it parses
the page's own microdata and compares it with what ytb read, predicate by
predicate, and reports where the two disagree.

Output is byte stable between two runs over the same input in all three formats.
A dump that reorders itself cannot be diffed, and a diff is how somebody notices
that YouTube started saying something different.`,
		Args: kit.MinimumNArgs(1),
		Flags: func(f *kit.FlagSet) {
			f.StringVar(&format, "format", string(rdf.NTriples), "nt, turtle or jsonld")
			f.BoolVar(&noProv, "no-provenance", false, "drop the source and client annotations")
			f.BoolVar(&check, "check", false, "compare our triples with the page's own microdata")
			f.BoolVar(&contexts, "types-only", false, "write nothing but the rdf:type of each node")
			o.bind(f)
		},
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)
			opt := o.options()
			opt.Microdata = check

			col := ytb.NewCollector()
			var failed error
			for _, ref := range args {
				if err := app.Client.Collect(ctx, ref, opt, col); err != nil {
					app.logf("note: %s", err)
					failed = err
				}
			}
			if col.Set.Len() == 0 && len(col.Statements) == 0 {
				if failed != nil {
					return failed
				}
				return noResults("no claims")
			}

			if check {
				return runRDFCheck(app, col)
			}

			statements := rdf.Merge(rdf.FromSet(col.Set), col.Statements)
			if contexts {
				statements = onlyTypes(statements)
			}
			// RDF is a document rather than a stream of records, so it goes to the
			// writer whole and does not pass through Emit. --output json on this
			// command would be json-ld, which is what --format jsonld already is.
			return rdf.Write(cmdOut, statements, rdf.Format(format), rdf.Options{Provenance: !noProv})
		},
	}
}

// runRDFCheck prints the per-predicate comparison rather than the triples.
func runRDFCheck(app *App, col *ytb.Collector) error {
	if len(col.Page) == 0 {
		return fmt.Errorf("no schema.org markup on the page, so there is nothing to check against: an embed page and a consent interstitial both do this")
	}
	ours := rdf.Merge(rdf.FromSet(col.Set), col.Statements)
	for _, c := range rdf.Compare(ours, col.Page, col.Aliases) {
		if err := app.Out.Emit(Row{
			Cols:  []string{"predicate", "ytb", "page", "agree"},
			Vals:  []string{c.Predicate, c.Ours, c.Page, string(c.Agree)},
			Value: c,
		}); err != nil {
			return err
		}
	}
	return app.Out.Flush()
}

// onlyTypes keeps the rdf:type statements, which is the smallest useful export:
// what nodes exist and what kind each one is, with none of the edges.
func onlyTypes(in []rdf.Statement) []rdf.Statement {
	out := in[:0:0]
	for _, st := range in {
		if st.Predicate == rdf.NSRDF+"type" {
			out = append(out, st)
		}
	}
	return out
}
