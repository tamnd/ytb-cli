package cli

import (
	"context"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/ytb-cli/pkg/graph"
	"github.com/tamnd/ytb-cli/youtube"
)

// edges.go is the graph plane's three commands. Spec 3005 doc 04.
//
// edges is one read and what it claims. graph walks the frontier those claims
// name, on a budget counted in requests. predicates prints the vocabulary and
// makes no request at all.

func newEdgesCmd() kit.Command {
	var o claimFlags
	return kit.Command{
		Use:   "edges <ref>...",
		Group: "read",
		Short: "The claims one read makes: subject, predicate, object, and who said so",
		Long: `Read anything YouTube has an id for and print what that read claims.

A claim is an edge with its provenance attached: which URL asserted it, which
surface answered, which client ytb said it was, and at which tier. The source is
part of the claim's identity, so two surfaces asserting the same edge stay two
rows and a disagreement between them is something you can query rather than
something the last read overwrote.

The point of the plane is what one request already knows. A watch page names a
channel, twenty related videos, a handful of external links and a hashtag, and
every one of those is a node ytb has heard of and never fetched.

One request per reference by default. Each flag below is another read and says
what it costs. A handle costs one more, to resolve it, because a handle is not
an identity and can never be a node.

ytb predicates prints the twenty one predicates with their domains and ranges.`,
		Args:  kit.MinimumNArgs(1),
		Flags: o.bind,
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)
			set := graph.NewSet()
			var failed error
			for _, ref := range args {
				if err := app.Client.Claims(ctx, ref, o.options(), set); err != nil {
					// One bad reference in a list should not throw away the claims the
					// others produced, so it is noted and the run continues.
					app.logf("note: %s", err)
					failed = err
					continue
				}
			}
			if set.Len() == 0 {
				if failed != nil {
					return failed
				}
				return noResults("no claims")
			}
			for _, e := range set.Edges() {
				if err := app.Out.Emit(edgeRow(e)); err != nil {
					return err
				}
			}
			return app.Out.Flush()
		},
	}
}

func newGraphCmd() kit.Command {
	var (
		o       claimFlags
		depth   int
		budget  int
		listing bool
	)
	return kit.Command{
		Use:   "graph <seed>...",
		Group: "read",
		Short: "Walk the frontier the claims name, on a budget counted in requests",
		Long: `Collect claims from the seeds, then from the nodes those claims named, and so
on, and print how many claims of each predicate came back.

--depth is how many hops to follow. Depth 0 is the seeds and nothing else, which
is exactly what ytb edges does.

--budget is in requests and it is counted, not estimated. Every request that
goes out passes a hook the walk counts on, and the walk stops when the count
reaches the budget, mid-level if that is where it lands. A cache hit never
reaches the hook, so it is not a request and does not count, which is why a
second walk over the same seeds gets further on the same budget.

The frontier is every node the claims so far have named and nothing has read.
Most of it is videos: one watch page names twenty related ones. Only nodes ytb
can read are followed, so an external URL and a hashtag are named and never
fetched.

--edges prints the claims themselves instead of the summary.`,
		Args: kit.MinimumNArgs(1),
		Flags: func(f *kit.FlagSet) {
			f.IntVar(&depth, "depth", 1, "hops to follow from each seed (0 = the seeds only)")
			f.IntVar(&budget, "budget", 25, "how many requests the walk may spend")
			f.BoolVar(&listing, "edges", false, "print the claims rather than the per-predicate counts")
			o.bind(f)
		},
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)

			var spent atomic.Int64
			app.Client.SetOnRequest(func(string, string) { spent.Add(1) })
			defer app.Client.SetOnRequest(nil)

			set := graph.NewSet()
			// read is by reference rather than by URI because a seed can be a handle,
			// which has no URI until something resolves it, and reading the same
			// channel twice under two spellings is still two requests.
			read := map[string]bool{}
			seen := map[graph.URI]bool{}
			frontier := args
			opt := o.options()

			for hop := 0; hop <= depth; hop++ {
				stopped := false
				for _, ref := range frontier {
					if read[ref] {
						continue
					}
					if budget > 0 && spent.Load() >= int64(budget) {
						app.logf("note: budget of %d requests reached at hop %d", budget, hop)
						stopped = true
						break
					}
					read[ref] = true
					if err := app.Client.Claims(ctx, ref, opt, set); err != nil {
						app.logf("note: %s", err)
					}
				}
				if stopped || hop == depth {
					break
				}
				// The frontier is what the claims named and nothing has read.
				var next []string
				for _, u := range set.Nodes() {
					if seen[u] {
						continue
					}
					seen[u] = true
					if ref := readableRef(u); ref != "" && !read[ref] {
						next = append(next, ref)
					}
				}
				frontier = next
				if len(frontier) == 0 {
					break
				}
			}

			if set.Len() == 0 {
				return noResults("no claims")
			}
			if listing {
				for _, e := range set.Edges() {
					if err := app.Out.Emit(edgeRow(e)); err != nil {
						return err
					}
				}
				return app.Out.Flush()
			}

			counts := set.CountByPredicate()
			names := make([]string, 0, len(counts))
			for p := range counts {
				names = append(names, string(p))
			}
			sort.Strings(names)
			for _, name := range names {
				if err := app.Out.Emit(Row{
					Cols:  []string{"predicate", "claims"},
					Vals:  []string{name, itoa(counts[graph.Predicate(name)])},
					Value: map[string]any{"predicate": name, "claims": counts[graph.Predicate(name)]},
				}); err != nil {
					return err
				}
			}
			app.logf("%d claims over %d nodes, %d requests spent", set.Len(), len(set.Nodes()), spent.Load())
			return app.Out.Flush()
		},
	}
}

func newPredicatesCmd() kit.Command {
	return kit.Command{
		Use:   "predicates",
		Group: "read",
		Short: "The closed vocabulary: every predicate with its domain and its range",
		Long: `Print the twenty one predicates the graph plane may write.

The table is closed. A predicate not in it cannot be written, which is what
stops a typo becoming a claim that looks fine, is never queried because nobody
knows to ask for it, and is discovered a year later by somebody counting.

The rdf column is the term each one maps to on the way out, and most of them
come from the schema.org markup YouTube publishes on its own pages rather than
from anything invented here. Where the arrow turns round on the way to RDF, the
inverse column says so: ytb writes "channel published video" because that is the
direction a page reads in, and schema:author runs from the work to its author.

This command makes no request.`,
		Args: kit.NoArgs,
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)
			for _, info := range graph.All() {
				rdf := info.RDF
				if info.Inverse {
					rdf += " (inverse)"
				}
				if err := app.Out.Emit(Row{
					Cols: []string{"predicate", "from", "to", "rdf", "origin"},
					Vals: []string{
						string(info.Name),
						strings.Join(info.Domain, ", "),
						strings.Join(info.Range, ", "),
						rdf,
						info.Origin,
					},
					Value: info,
				}); err != nil {
					return err
				}
			}
			return app.Out.Flush()
		},
	}
}

// claimFlags are the extra reads a claim collection may make, shared by edges
// and graph so the two ask for the same things by the same names.
type claimFlags struct {
	captions bool
	comments int
	items    int
	featured bool
	posts    int
	music    bool
}

func (o *claimFlags) bind(f *kit.FlagSet) {
	f.BoolVar(&o.captions, "captions", false, "read the ANDROID player for the caption list (1 request)")
	f.IntVar(&o.comments, "comments", 0, "read up to n comments (1 request per page of about 20)")
	f.IntVar(&o.items, "items", 0, "read up to n playlist items (1 request per page)")
	f.BoolVar(&o.featured, "featured", false, "read a channel's home tab for its featured channels (1 request)")
	f.IntVar(&o.posts, "posts", 0, "read up to n community posts (the tab refuses tier 0 today)")
	f.BoolVar(&o.music, "music", false, "read the same id through YouTube Music (1 request)")
}

func (o *claimFlags) options() youtube.ClaimOptions {
	return youtube.ClaimOptions{
		Captions: o.captions,
		Comments: o.comments,
		Items:    o.items,
		Featured: o.featured,
		Posts:    o.posts,
		Music:    o.music,
	}
}

// edgeRow is one claim as a row. The note is on the end because it is the only
// column a person reads: three yt:// URIs tell nobody anything.
func edgeRow(e graph.Edge) Row {
	return Row{
		Cols: []string{"from", "predicate", "to", "note", "surface"},
		Vals: []string{
			string(e.From), string(e.Predicate), string(e.To), e.Note, surfaceCell(e),
		},
		Value: e,
	}
}

// surfaceCell is the surface with the claimed client after it, because s2 on its
// own does not say whether it was WEB or ANDROID that answered, and on this site
// that is the difference between six caption tracks and none.
func surfaceCell(e graph.Edge) string {
	if e.Client == "" {
		return e.Surface
	}
	return e.Surface + " " + e.Client
}

// readableRef turns a frontier node back into something Claims can read. A
// comment, a hashtag and an external URL are named by claims and are not reads,
// so they stay on the frontier and are never fetched.
func readableRef(u graph.URI) string {
	p, ok := graph.Parse(u)
	if !ok || p.IsFragment() {
		return ""
	}
	switch p.Kind {
	case graph.Video, graph.Channel, graph.Playlist, graph.Album, graph.Artist:
		return p.ID
	default:
		return ""
	}
}
