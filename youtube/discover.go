package youtube

import (
	"context"
	"fmt"
	"strings"
)

// discover.go is the breadth-first graph walker. Every read in this package
// answers one question about one object (a video's metadata, a channel's
// uploads, a playlist's items); the walker chains them. From a seed video,
// channel, or playlist it follows the object's links (a video's uploader and
// related videos, a channel's uploads, playlists, and community posts, a
// playlist's items and owner, a comment's author) and from each neighbor it
// follows theirs, hop by hop, until it runs out of depth or budget.
//
// It is engine-agnostic on purpose: Walk talks to the small grapher interface,
// not to *Client directly, so the traversal is hermetically testable with a
// fake graph and *Client is just the production grapher.

// NodeKind is the type of a node the walk visits.
type NodeKind string

const (
	KindVideo    NodeKind = "video"
	KindChannel  NodeKind = "channel"
	KindPlaylist NodeKind = "playlist"
	KindComment  NodeKind = "comment"
	KindPost     NodeKind = "post"
)

// Edge names a link the walk can follow. The string is the public vocabulary:
// it is what the user types in --follow, what lands in the store's edges.kind
// column, and what a discovered node reports as the edge it arrived by.
type Edge string

const (
	EdgeChannel   Edge = "channel"   // video -> the channel that uploaded it
	EdgeRelated   Edge = "related"   // video -> a related/recommended video
	EdgeComments  Edge = "comments"  // video -> a comment on it
	EdgeUploads   Edge = "uploads"   // channel -> a video it uploaded
	EdgePlaylists Edge = "playlists" // channel -> a playlist it owns
	EdgeCommunity Edge = "community" // channel -> a community post
	EdgeItems     Edge = "items"     // playlist -> a video in it
	EdgeOwner     Edge = "owner"     // playlist -> the channel that owns it
	EdgeCommenter Edge = "commenter" // comment -> the channel that wrote it
)

// allEdges is the full vocabulary, in a stable display order.
var allEdges = []Edge{
	EdgeChannel, EdgeRelated, EdgeComments,
	EdgeUploads, EdgePlaylists, EdgeCommunity,
	EdgeItems, EdgeOwner, EdgeCommenter,
}

// knownEdges indexes allEdges for validation.
var knownEdges = func() map[Edge]bool {
	m := make(map[Edge]bool, len(allEdges))
	for _, e := range allEdges {
		m[e] = true
	}
	return m
}()

// Target reports the kind of node an edge leads to.
func (e Edge) Target() NodeKind {
	switch e {
	case EdgeChannel, EdgeOwner, EdgeCommenter:
		return KindChannel
	case EdgePlaylists:
		return KindPlaylist
	case EdgeComments:
		return KindComment
	case EdgeCommunity:
		return KindPost
	default:
		return KindVideo
	}
}

// gated reports whether an edge is the one YouTube hides behind its per-IP
// anti-bot wall. Comments (and a commenter reached through them) are served only
// when the request is not in Restricted Mode; every other edge is on the open
// surface. The walker does not pre-drop a gated edge: it tries it and turns a
// refusal into a note, so a walk always produces what it can.
func (e Edge) gated() bool {
	switch e {
	case EdgeComments, EdgeCommenter:
		return true
	default:
		return false
	}
}

// EdgeSet is a chosen set of edges to follow.
type EdgeSet map[Edge]bool

// Has reports whether the set contains e (a nil set contains nothing).
func (s EdgeSet) Has(e Edge) bool { return s[e] }

// List returns the set's edges in stable display order.
func (s EdgeSet) List() []Edge {
	var out []Edge
	for _, e := range allEdges {
		if s[e] {
			out = append(out, e)
		}
	}
	return out
}

// String renders the set as a comma-separated, ordered list.
func (s EdgeSet) String() string { return joinEdges(s.List()) }

// edgePresets are the named bundles --follow accepts in place of listing edges.
// They are the everyday intents: read the obvious neighbors of whatever you gave
// (content), map what a channel publishes (feed), study who engaged (comments),
// or take it all. Preset names are kept disjoint from edge names so no token is
// ambiguous: to chase recommendations alone, name the edge (`--follow related`).
//
// content is the default and deliberately spans every seed kind: a video yields
// its uploader and related videos, a channel yields its uploads, a playlist
// yields its items and owner, so `ytb discover <anything>` does the obvious
// thing with no flags. It stays entirely on the open surface (no comments).
var edgePresets = map[string]EdgeSet{
	"content":  newEdgeSet(EdgeChannel, EdgeRelated, EdgeUploads, EdgeItems, EdgeOwner),
	"feed":     newEdgeSet(EdgeUploads, EdgePlaylists, EdgeCommunity),
	"comments": newEdgeSet(EdgeComments, EdgeCommenter),
	"all":      newEdgeSet(allEdges...),
}

// presetNames lists the presets in a friendly order for help text.
var presetNames = []string{"content", "feed", "comments", "all"}

func newEdgeSet(edges ...Edge) EdgeSet {
	s := make(EdgeSet, len(edges))
	for _, e := range edges {
		s[e] = true
	}
	return s
}

// DefaultEdges is what a walk follows when --follow is unset: the obvious
// neighbors of the seed, all on the open surface, so `ytb discover <video>`
// works with no tokens and no anti-bot exposure.
func DefaultEdges() EdgeSet { return edgePresets["content"].clone() }

func (s EdgeSet) clone() EdgeSet {
	out := make(EdgeSet, len(s))
	for e := range s {
		out[e] = true
	}
	return out
}

// EdgeHelp is the one-line catalogue of presets and edges for flag help and
// usage errors, so the names a user can type live in exactly one place.
func EdgeHelp() string {
	return "presets: " + strings.Join(presetNames, ",") + "; edges: " + joinEdges(allEdges)
}

// ParseEdges turns a --follow spec into an EdgeSet. The spec is a comma list of
// preset names and/or edge names ("content", "channel,related", "uploads,items").
// An empty spec yields DefaultEdges. An unknown token is a usage error naming the
// catalogue, so a typo points the user at the real vocabulary.
func ParseEdges(spec string) (EdgeSet, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return DefaultEdges(), nil
	}
	set := EdgeSet{}
	for _, part := range strings.Split(spec, ",") {
		p := strings.ToLower(strings.TrimSpace(part))
		if p == "" {
			continue
		}
		if preset, ok := edgePresets[p]; ok {
			for e := range preset {
				set[e] = true
			}
			continue
		}
		e := Edge(p)
		if !knownEdges[e] {
			return nil, fmt.Errorf("unknown edge or preset %q (%s)", p, EdgeHelp())
		}
		set[e] = true
	}
	if len(set) == 0 {
		return nil, fmt.Errorf("no edges selected (%s)", EdgeHelp())
	}
	return set, nil
}

func joinEdges(edges []Edge) string {
	ss := make([]string, len(edges))
	for i, e := range edges {
		ss[i] = string(e)
	}
	return strings.Join(ss, ",")
}

// Node is one object the walk reached, tagged with how it got there: the BFS
// depth, the edge it arrived by, and the endpoint of the node it came from.
// Exactly one of the entity pointers is set, matching Kind. Node is what Walk
// hands to its callback and what the CLI renders.
type Node struct {
	Kind     NodeKind       `json:"kind"`
	Depth    int            `json:"depth"`
	Via      Edge           `json:"via,omitempty"`
	Parent   string         `json:"parent,omitempty"`
	Video    *Video         `json:"video,omitempty"`
	Channel  *Channel       `json:"channel,omitempty"`
	Playlist *Playlist      `json:"playlist,omitempty"`
	Comment  *Comment       `json:"comment,omitempty"`
	Post     *CommunityPost `json:"post,omitempty"`
}

// Endpoint is the node's stable identifier inside a walk: a video/channel/
// playlist/comment/post id. It is what edges record as src/dst.
func (n *Node) Endpoint() string {
	switch n.Kind {
	case KindVideo:
		if n.Video != nil {
			return n.Video.VideoID
		}
	case KindChannel:
		if n.Channel != nil {
			return n.Channel.ChannelID
		}
	case KindPlaylist:
		if n.Playlist != nil {
			return n.Playlist.PlaylistID
		}
	case KindComment:
		if n.Comment != nil {
			return n.Comment.ID
		}
	case KindPost:
		if n.Post != nil {
			return n.Post.PostID
		}
	}
	return ""
}

// key is the dedup key for a hydrated node. Channels key on a lowercased id so
// the same channel reached as an uploader and as a commenter collapses to one
// node; the rest key on their id with a per-kind prefix so a video and a
// playlist that happened to share an id never collide.
func (n *Node) key() string {
	switch n.Kind {
	case KindVideo:
		return "v:" + nodeID(n.Video != nil, func() string { return n.Video.VideoID })
	case KindChannel:
		return "c:" + strings.ToLower(nodeID(n.Channel != nil, func() string { return n.Channel.ChannelID }))
	case KindPlaylist:
		return "p:" + nodeID(n.Playlist != nil, func() string { return n.Playlist.PlaylistID })
	case KindComment:
		return "m:" + nodeID(n.Comment != nil, func() string { return n.Comment.ID })
	case KindPost:
		return "o:" + nodeID(n.Post != nil, func() string { return n.Post.PostID })
	}
	return ""
}

func nodeID(ok bool, get func() string) string {
	if ok {
		return get()
	}
	return ""
}

// Seed is a parsed starting point for a walk.
type Seed struct {
	Kind NodeKind
	Ref  string // canonical id / @handle to fetch
}

// ParseSeed classifies a raw reference into a Seed, reusing the domain's own
// Classify so a seed is read exactly like any other youtube reference: a watch
// link or bare video id is a video, a playlist link or PL-style id is a
// playlist, a @handle or UC id or channel URL is a channel.
func ParseSeed(ref string) (Seed, error) {
	t, id, err := Domain{}.Classify(ref)
	if err != nil {
		return Seed{}, err
	}
	switch t {
	case "video":
		return Seed{Kind: KindVideo, Ref: id}, nil
	case "channel":
		return Seed{Kind: KindChannel, Ref: id}, nil
	case "playlist":
		return Seed{Kind: KindPlaylist, Ref: id}, nil
	default:
		return Seed{}, fmt.Errorf("cannot start a walk from a %s reference: %q", t, ref)
	}
}

// WalkOptions tunes a traversal.
type WalkOptions struct {
	Depth  int     // hops to follow from each seed (0 = seeds only)
	Max    int     // stop after emitting this many nodes (0 = unlimited)
	Fanout int     // per-edge neighbor cap (0 = unlimited)
	Edges  EdgeSet // edges to follow (nil = DefaultEdges)

	// OnEdge, if set, is called for every edge the walk traverses, before the
	// neighbor is visited, with the two endpoints and the edge. The store sink
	// uses it to record the graph; it fires even for an already-visited neighbor
	// so the edge list stays complete.
	OnEdge func(src, dst string, edge Edge)

	// Note, if set, surfaces a one-line advisory (a comment edge refused by the
	// anti-bot wall, a neighbor that could not be fetched). It never carries a
	// fatal error.
	Note func(string)
}

// grapher is the slice of the client the walker needs. *Client satisfies it; a
// test supplies a fake. Every method matches *Client exactly.
type grapher interface {
	FetchVideo(ctx context.Context, ref string, opt VideoOptions) (*VideoResult, error)
	FetchChannel(ctx context.Context, ref string) (*Channel, error)
	FetchPlaylist(ctx context.Context, ref string) (*Playlist, error)
	StreamChannelTab(ctx context.Context, ref, tab string, opt PageOptions, emit func(Video) error) error
	StreamChannelPlaylists(ctx context.Context, ref string, opt PageOptions, emit func(Playlist) error) error
	StreamPlaylistItems(ctx context.Context, ref string, opt PageOptions, emit func(PlaylistVideo, Video) error) error
	StreamComments(ctx context.Context, ref string, opt CommentOptions, emit func(Comment) error) error
	StreamCommunity(ctx context.Context, ref string, opt PageOptions, emit func(CommunityPost) error) error
}

var _ grapher = (*Client)(nil)

// Walker performs the breadth-first traversal over a grapher.
type Walker struct{ g grapher }

// NewWalker builds a Walker over any grapher (the client in production, a fake
// in tests).
func NewWalker(g grapher) *Walker { return &Walker{g: g} }

// Walk runs the client's traversal. It is the production entry point: it builds
// a Walker over the client and walks the seeds. See Walker.Walk.
func (c *Client) Walk(ctx context.Context, seeds []Seed, opts WalkOptions, emit func(*Node) error) error {
	return NewWalker(c).Walk(ctx, seeds, opts, emit)
}

// frontier is a queued, possibly-not-yet-hydrated node. Stream reads (a
// channel's uploads, a playlist's items) hand back fully built entities, so
// those rides carry the entity already and skip the per-pop fetch; a reference
// reached as an id (an uploader channel, a related video) carries only the id
// and is fetched when it is popped.
type frontier struct {
	kind     NodeKind
	ref      string
	depth    int
	via      Edge
	parent   string
	video    *Video
	channel  *Channel
	playlist *Playlist
	comment  *Comment
	post     *CommunityPost
}

func (f frontier) key() string {
	switch f.kind {
	case KindVideo:
		id := f.ref
		if f.video != nil {
			id = f.video.VideoID
		}
		return "v:" + id
	case KindChannel:
		id := f.ref
		if f.channel != nil {
			id = f.channel.ChannelID
		}
		return "c:" + strings.ToLower(id)
	case KindPlaylist:
		id := f.ref
		if f.playlist != nil {
			id = f.playlist.PlaylistID
		}
		return "p:" + id
	case KindComment:
		if f.comment != nil {
			return "m:" + f.comment.ID
		}
		return "m:" + f.ref
	case KindPost:
		if f.post != nil {
			return "o:" + f.post.PostID
		}
		return "o:" + f.ref
	}
	return ""
}

// Walk visits the seeds and their links in breadth-first order, calling emit for
// each node as it is reached. It returns when the queue drains, the node budget
// (opts.Max) is hit, emit returns an error, or a seed cannot be fetched. A
// gated edge (comments, and the channel reached through one) is attempted, not
// pre-dropped: when YouTube refuses it the failure becomes a Note and the walk
// keeps going on the rest of the graph.
func (w *Walker) Walk(ctx context.Context, seeds []Seed, opts WalkOptions, emit func(*Node) error) error {
	edges := opts.Edges
	if edges == nil {
		edges = DefaultEdges()
	}

	visited := map[string]bool{}
	queue := make([]frontier, 0, len(seeds))
	for _, s := range seeds {
		queue = append(queue, frontier{kind: s.Kind, ref: s.Ref})
	}

	emitted := 0
	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		f := queue[0]
		queue = queue[1:]
		if visited[f.key()] {
			continue
		}
		visited[f.key()] = true

		node, vres, err := w.hydrate(ctx, f, f.depth < opts.Depth, edges)
		if err != nil {
			if f.depth == 0 {
				return err // a seed we cannot fetch is fatal, like a single read
			}
			if opts.Note != nil {
				opts.Note(fmt.Sprintf("skip %s %s: %v", f.kind, f.ref, err))
			}
			continue
		}
		if node == nil {
			continue
		}
		visited[node.key()] = true // collapse handle/id aliases of the same node

		if err := emit(node); err != nil {
			return err
		}
		emitted++
		if opts.Max > 0 && emitted >= opts.Max {
			return nil
		}
		if f.depth >= opts.Depth {
			continue
		}
		for _, nb := range w.neighbors(ctx, node, vres, edges, opts) {
			if !visited[nb.key()] {
				queue = append(queue, nb)
			}
		}
	}
	return nil
}

// hydrate turns a frontier item into a Node, fetching the object when the item
// did not already carry it. It returns the VideoResult alongside a video node so
// neighbors can read the related list and comment availability without a second
// fetch. expand says whether this node will be expanded (its depth is below the
// limit); a video carried in from a stream is only refetched (for its related
// list) when it will actually be expanded along the related edge.
func (w *Walker) hydrate(ctx context.Context, f frontier, expand bool, edges EdgeSet) (*Node, *VideoResult, error) {
	n := &Node{Kind: f.kind, Depth: f.depth, Via: f.via, Parent: f.parent}
	switch f.kind {
	case KindVideo:
		wantRelated := expand && edges.Has(EdgeRelated)
		if f.video == nil {
			res, err := w.g.FetchVideo(ctx, f.ref, VideoOptions{Next: wantRelated})
			if err != nil {
				return nil, nil, err
			}
			if res == nil {
				return nil, nil, fmt.Errorf("video not found: %s", f.ref)
			}
			n.Video = &res.Video
			return n, res, nil
		}
		n.Video = f.video
		if wantRelated {
			// The streamed video has no related list; fetch it once so the related
			// edge has something to expand. A failure here is not fatal: keep the
			// node we already have and expand whatever else applies.
			if res, err := w.g.FetchVideo(ctx, f.video.VideoID, VideoOptions{Next: true}); err == nil && res != nil {
				n.Video = &res.Video
				return n, res, nil
			}
		}
		return n, nil, nil
	case KindChannel:
		if f.channel == nil {
			ch, err := w.g.FetchChannel(ctx, f.ref)
			if err != nil {
				return nil, nil, err
			}
			if ch == nil {
				return nil, nil, fmt.Errorf("channel not found: %s", f.ref)
			}
			n.Channel = ch
		} else {
			n.Channel = f.channel
		}
		return n, nil, nil
	case KindPlaylist:
		if f.playlist == nil {
			pl, err := w.g.FetchPlaylist(ctx, f.ref)
			if err != nil {
				return nil, nil, err
			}
			if pl == nil {
				return nil, nil, fmt.Errorf("playlist not found: %s", f.ref)
			}
			n.Playlist = pl
		} else {
			n.Playlist = f.playlist
		}
		return n, nil, nil
	case KindComment:
		n.Comment = f.comment
		return n, nil, nil
	case KindPost:
		n.Post = f.post
		return n, nil, nil
	}
	return nil, nil, nil
}

// neighbors expands a node into its outbound frontier under the chosen edges,
// recording each edge via opts.OnEdge. The per-edge fanout caps every stream
// read and the inline related loop, so one hop can never page a channel's whole
// upload history unless the caller asked for it (Fanout 0).
func (w *Walker) neighbors(ctx context.Context, n *Node, vres *VideoResult, edges EdgeSet, opts WalkOptions) []frontier {
	var out []frontier
	src := n.Endpoint()
	streamMax := opts.Fanout
	if streamMax <= 0 {
		streamMax = opts.Max // bound an uncapped stream by the node budget
	}

	addEdge := func(dst string, via Edge) {
		if opts.OnEdge != nil {
			opts.OnEdge(src, dst, via)
		}
	}

	switch n.Kind {
	case KindVideo:
		v := n.Video
		if edges.Has(EdgeChannel) && v.ChannelID != "" {
			addEdge(v.ChannelID, EdgeChannel)
			out = append(out, frontier{kind: KindChannel, ref: v.ChannelID, depth: n.Depth + 1, via: EdgeChannel, parent: src})
		}
		if edges.Has(EdgeRelated) && vres != nil {
			i := 0
			for _, r := range vres.Related {
				if r.RelatedVideoID == "" {
					continue
				}
				if streamMax > 0 && i >= streamMax {
					break
				}
				addEdge(r.RelatedVideoID, EdgeRelated)
				out = append(out, frontier{kind: KindVideo, ref: r.RelatedVideoID, depth: n.Depth + 1, via: EdgeRelated, parent: src})
				i++
			}
		}
		if edges.Has(EdgeComments) {
			i := 0
			err := w.g.StreamComments(ctx, v.VideoID, CommentOptions{Max: streamMax}, func(cm Comment) error {
				c := cm
				addEdge(c.ID, EdgeComments)
				out = append(out, frontier{kind: KindComment, depth: n.Depth + 1, via: EdgeComments, parent: src, comment: &c})
				i++
				if streamMax > 0 && i >= streamMax {
					return ErrStop
				}
				return nil
			})
			w.note(opts, err)
		}
	case KindChannel:
		ch := n.Channel
		ref := ch.ChannelID
		if edges.Has(EdgeUploads) {
			i := 0
			err := w.g.StreamChannelTab(ctx, ref, "videos", PageOptions{Max: streamMax}, func(v Video) error {
				vv := v
				addEdge(vv.VideoID, EdgeUploads)
				out = append(out, frontier{kind: KindVideo, depth: n.Depth + 1, via: EdgeUploads, parent: src, video: &vv})
				i++
				if streamMax > 0 && i >= streamMax {
					return ErrStop
				}
				return nil
			})
			w.note(opts, err)
		}
		if edges.Has(EdgePlaylists) {
			i := 0
			err := w.g.StreamChannelPlaylists(ctx, ref, PageOptions{Max: streamMax}, func(p Playlist) error {
				pp := p
				addEdge(pp.PlaylistID, EdgePlaylists)
				out = append(out, frontier{kind: KindPlaylist, depth: n.Depth + 1, via: EdgePlaylists, parent: src, playlist: &pp})
				i++
				if streamMax > 0 && i >= streamMax {
					return ErrStop
				}
				return nil
			})
			w.note(opts, err)
		}
		if edges.Has(EdgeCommunity) {
			i := 0
			err := w.g.StreamCommunity(ctx, ref, PageOptions{Max: streamMax}, func(p CommunityPost) error {
				pp := p
				addEdge(pp.PostID, EdgeCommunity)
				out = append(out, frontier{kind: KindPost, depth: n.Depth + 1, via: EdgeCommunity, parent: src, post: &pp})
				i++
				if streamMax > 0 && i >= streamMax {
					return ErrStop
				}
				return nil
			})
			w.note(opts, err)
		}
	case KindPlaylist:
		pl := n.Playlist
		ref := pl.PlaylistID
		if edges.Has(EdgeItems) {
			i := 0
			err := w.g.StreamPlaylistItems(ctx, ref, PageOptions{Max: streamMax}, func(pv PlaylistVideo, v Video) error {
				vv := v
				if vv.VideoID == "" {
					vv.VideoID = pv.VideoID
				}
				if vv.URL == "" {
					vv.URL = "https://www.youtube.com/watch?v=" + vv.VideoID
				}
				addEdge(vv.VideoID, EdgeItems)
				out = append(out, frontier{kind: KindVideo, depth: n.Depth + 1, via: EdgeItems, parent: src, video: &vv})
				i++
				if streamMax > 0 && i >= streamMax {
					return ErrStop
				}
				return nil
			})
			w.note(opts, err)
		}
		if edges.Has(EdgeOwner) && pl.ChannelID != "" {
			addEdge(pl.ChannelID, EdgeOwner)
			out = append(out, frontier{kind: KindChannel, ref: pl.ChannelID, depth: n.Depth + 1, via: EdgeOwner, parent: src})
		}
	case KindComment:
		cm := n.Comment
		if edges.Has(EdgeCommenter) && cm.AuthorChannelID != "" {
			addEdge(cm.AuthorChannelID, EdgeCommenter)
			out = append(out, frontier{kind: KindChannel, ref: cm.AuthorChannelID, depth: n.Depth + 1, via: EdgeCommenter, parent: src})
		}
	case KindPost:
		// A community post is a leaf in this graph: its author is the channel we
		// arrived from, so it has no outbound edge of its own.
	}
	return out
}

// note surfaces a non-fatal stream failure (a comment edge refused by the
// anti-bot wall, a channel with no community tab, a transient rate limit) as an
// advisory and keeps the walk going on the rest of the graph. The clean stop
// sentinel ErrStop is not a failure.
func (w *Walker) note(opts WalkOptions, err error) {
	if err == nil || err == ErrStop {
		return
	}
	if opts.Note != nil {
		opts.Note(err.Error())
	}
}
