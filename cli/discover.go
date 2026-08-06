package cli

import (
	"context"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/ytb-cli/pkg/graph"
	"github.com/tamnd/ytb-cli/youtube"
)

// defaultDiscoverBudget caps a streaming walk when the user did not pass -n, so
// `ytb discover <video>` always terminates instead of spidering YouTube forever.
const defaultDiscoverBudget = 500

// newDiscoverCmd is the breadth-first graph walk. Where the record reads each
// answer one question about one object, discover chains them: from a seed video,
// channel, or playlist it follows the object's links and from each neighbor it
// follows theirs, hop by hop, emitting one row per node as it is reached.
//
// It shares the read group with the per-object commands because it is a read; it
// only touches the store when --store is set, where it writes each node it
// reached as a record.
func newDiscoverCmd() kit.Command {
	var (
		depth  int
		fanout int
		follow string
		store  bool
	)
	return kit.Command{
		Use:     "discover <seed>...",
		Aliases: []string{"walk"},
		Group:   "read",
		Short:   "Breadth-first walk of the graph linked from a video, channel, or playlist",
		Long: `Walk the graph of linked YouTube objects, breadth first, starting from one or
more seeds. A seed is anything ytb can resolve: a video id or watch URL, a
channel id, @handle, or URL, or a playlist id or URL.

--follow chooses which links to traverse. It takes a preset or a comma-separated
edge list:

  content   a video's channel and related videos, a channel's uploads, a
            playlist's items and owner (the default; the obvious neighbors)
  feed      a channel's uploads, playlists, and community posts
  comments  a video's comments and the channels that wrote them
  all       every edge

Edges: channel, related, comments, uploads, playlists, community, items, owner,
commenter. Name an edge directly to follow just that link, e.g. --follow related
to chase the recommendation graph.

--depth is how many hops to follow (default 1; 0 emits only the seeds). --fanout
caps neighbors per edge (default 25). The walk streams nodes and stops after -n
nodes (default 500). Comments are served only when YouTube is not applying its
per-IP Restricted Mode to this network; when it is, the comment edges are noted
and skipped and the rest of the walk continues.

Add --store to write every node it reached into the store. A node the walk
fetched is stored as a record; a node it only saw in a shelf is stored as a
sighting with no record, so a later crawl still knows to go and read it. It
writes nodes and not claims: the edge names above are walk instructions rather
than the predicates a claim is made of, so ytb crawl is what writes the graph.
Query either afterwards with ytb query.`,
		Args: kit.MinimumNArgs(1),
		Flags: func(f *kit.FlagSet) {
			f.IntVar(&depth, "depth", 1, "hops to follow from each seed (0 = seeds only)")
			f.IntVar(&fanout, "fanout", 25, "max neighbors to follow per edge (0 = unlimited)")
			f.StringVar(&follow, "follow", "content", "edges to follow ("+youtube.EdgeHelp()+")")
			f.BoolVar(&store, "store", false, "write every node reached into the local store")
		},
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)

			edges, err := youtube.ParseEdges(follow)
			if err != nil {
				return usageErr(err.Error())
			}
			seeds, err := parseSeeds(args)
			if err != nil {
				return err
			}

			var st *youtube.Store
			if store {
				st, err = app.RequireStore()
				if err != nil {
					return err
				}
			}

			budget := app.Limit
			if budget <= 0 {
				budget = defaultDiscoverBudget
			}

			opts := youtube.WalkOptions{
				Depth:  depth,
				Max:    budget,
				Fanout: fanout,
				Edges:  edges,
				Note:   func(s string) { app.logf("note: %s", s) },
			}

			n := 0
			err = app.Client.Walk(ctx, seeds, opts, func(nd *youtube.Node) error {
				if st != nil {
					storeWalkNode(st, nd)
				}
				if e := app.Out.Emit(nodeRow(nd)); e != nil {
					return e
				}
				n++
				return nil
			})
			if flushErr := app.Out.Flush(); flushErr != nil && err == nil {
				err = flushErr
			}
			if err != nil {
				// The only fatal walk error is a seed that could not be fetched;
				// it surfaces like a failed single read. Deeper failures (a gated
				// comment edge, a missing community tab) are notes, not errors.
				return err
			}
			if n == 0 {
				return noResults("nothing discovered")
			}
			return nil
		},
	}
}

// storeWalkNode writes a walked node into the store.
//
// Records only, and only for the nodes the walk actually asked about. The walk's
// edge names are how a person describes a hop rather than the predicates the
// claims table is built on: "uploads" is a walk instruction and published is a
// claim about the world, so writing the first as the second would put rows in
// the store that no crawl would produce and no query over doc 04's vocabulary
// would find. Use ytb crawl for claims.
//
// A node the walk only saw in somebody else's shelf is written down as a
// sighting with no record, which is what leaves it on the next crawl's frontier
// instead of marking it read on the strength of a title.
func storeWalkNode(st *youtube.Store, nd *youtube.Node) {
	var rec any
	var uri graph.URI
	switch nd.Kind {
	case youtube.KindVideo:
		if nd.Video != nil {
			rec, uri = *nd.Video, graph.VideoURI(nd.Video.VideoID)
		}
	case youtube.KindChannel:
		if nd.Channel != nil {
			rec, uri = *nd.Channel, graph.ChannelURI(nd.Channel.ChannelID)
		}
	case youtube.KindPlaylist:
		if nd.Playlist != nil {
			rec, uri = *nd.Playlist, graph.PlaylistURI(nd.Playlist.PlaylistID)
		}
	case youtube.KindComment:
		if nd.Comment != nil {
			rec, uri = *nd.Comment, graph.CommentURI(nd.Comment.ID)
		}
	case youtube.KindPost:
		if nd.Post != nil {
			rec, uri = *nd.Post, graph.PostURI(nd.Post.PostID)
		}
	}
	if rec == nil {
		return
	}
	if !nd.Fetched {
		_ = st.Sight(uri)
		return
	}
	_, _ = st.PutRecord(rec)
}

// parseSeeds turns the positional arguments into walk seeds, reporting an
// unrecognized reference as a usage error rather than a plain failure.
func parseSeeds(args []string) ([]youtube.Seed, error) {
	seeds := make([]youtube.Seed, 0, len(args))
	for _, a := range args {
		s, err := youtube.ParseSeed(a)
		if err != nil {
			return nil, usageErr(err.Error())
		}
		seeds = append(seeds, s)
	}
	return seeds, nil
}
