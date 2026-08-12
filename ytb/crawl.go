package ytb

import (
	"context"
	"fmt"
	"sort"
	"sync/atomic"
	"time"

	"github.com/tamnd/ytb-cli/pkg/graph"
	"github.com/tamnd/ytb-cli/pkg/ytid"
)

// crawl.go walks the frontier and writes what it finds. Spec 3005 doc 04
// section 3.3.
//
// The budget is in requests, because requests are the unit rate limits are
// written in and the unit a person can reason about. It is a count rather than
// an estimate: every request that goes out passes a hook the walk counts on, and
// a cache hit never reaches the hook, so it is not a request, does not count and
// is not logged. That is why a second crawl over the same seeds gets further on
// the same budget.

// CrawlOptions is how far, how much, and what to read at each node.
type CrawlOptions struct {
	// Depth is hops from the seeds. Depth 0 reads the seeds and nothing else.
	Depth int
	// Budget is how many requests the whole walk may spend, counted as they go
	// out. Zero means no ceiling, which is not a default anything should choose.
	//
	// It is checked before each reference rather than before each request, because
	// stopping halfway through reading one thing stores half of it. So a crawl can
	// finish a few requests over its budget, and the manifest reports what it
	// actually spent rather than what it was allowed.
	Budget int
	// Claims are the extra reads made at each node, and every one of them is a
	// request per node rather than a request per crawl.
	Claims ClaimOptions
	// Uploads puts a channel's derived playlists on the frontier. On by default
	// through the CLI: each is one request for a page of about thirty videos and
	// needs no channel read to construct, so it is the cheapest thing in the walk.
	Uploads bool
	// Resume starts from what the store has heard of and not read, as well as from
	// the seeds. Without it a second crawl walks the same first hop again.
	Resume bool
}

// Note is one thing the crawl did not do, with the reason in the words whoever
// refused it used.
type Note struct {
	Ref    string `json:"ref"`
	Reason string `json:"reason"`
}

// Manifest is the record of one crawl.
//
// A crawl that hit a wall and a crawl that ran out of budget look identical in
// the store and different in here, which is the only reason this type exists.
type Manifest struct {
	Seeds      []string       `json:"seeds"`
	Depth      int            `json:"depth"`
	Budget     int            `json:"budget"`
	Spent      int            `json:"spent"`
	StoppedBy  string         `json:"stopped_by"`
	StartedAt  time.Time      `json:"started_at"`
	FinishedAt time.Time      `json:"finished_at"`
	Reads      int            `json:"reads"`
	Bytes      int64          `json:"bytes"`
	Claims     int            `json:"claims"`
	Records    int            `json:"records"`
	Nodes      int            `json:"nodes"`
	Surfaces   map[string]int `json:"surfaces"`
	Clients    map[string]int `json:"clients"`
	Statuses   map[string]int `json:"statuses"`
	// Refusals are the reads that came back as a no, quoted. A gated comment
	// section and a 404 are both in here and they are different sentences.
	Refusals []Note `json:"refusals,omitempty"`
	// Policy is what the crawl left alone on purpose. It is filled from the
	// options rather than from what happened, because a store with no comment
	// claims in it and a crawl that never asked for comments are the same silence
	// otherwise.
	Policy []Note `json:"policy"`
}

// Crawl walks from the seeds and writes nodes, claims and reads into the store.
//
// It returns the manifest whatever happened, cancellation included, because a
// partial crawl with a manifest is useful and a partial crawl without one is a
// store nobody can account for.
func Crawl(ctx context.Context, c *Client, store *Store, seeds []string, opt CrawlOptions, logf func(string)) (*Manifest, error) {
	if logf == nil {
		logf = func(string) {}
	}
	m := &Manifest{
		Seeds:     seeds,
		Depth:     opt.Depth,
		Budget:    opt.Budget,
		StartedAt: time.Now(),
		Surfaces:  map[string]int{},
		Clients:   map[string]int{},
		Statuses:  map[string]int{},
		Policy:    crawlPolicy(opt),
	}

	var spent atomic.Int64
	c.SetOnRequest(func(string, string) { spent.Add(1) })
	c.SetOnRead(func(r Read) {
		m.Reads++
		m.Bytes += int64(r.Bytes)
		m.Surfaces[r.Surface]++
		if r.Client != "" {
			m.Clients[r.Client]++
		}
		m.Statuses[fmt.Sprint(r.Status)]++
		if err := store.PutRead(r); err != nil {
			logf(fmt.Sprintf("note: could not log the read of %s: %v", r.URL, err))
		}
	})
	defer func() {
		c.SetOnRequest(nil)
		c.SetOnRead(nil)
	}()

	// read is keyed by reference rather than by URI because a seed can be a
	// handle, which has no URI until something resolves it, and reading one
	// channel under two spellings is still two requests.
	read := map[string]bool{}
	nodes := map[graph.URI]bool{}
	frontier := append([]string(nil), seeds...)
	if opt.Resume {
		resumed, err := store.Frontier("", 500)
		if err != nil {
			return m, err
		}
		for _, u := range resumed {
			if ref := ReadableRef(u); ref != "" && !skipOnFrontier(ref) {
				frontier = append(frontier, ref)
			}
		}
		logf(fmt.Sprintf("resuming with %d nodes the store has heard of and not read", len(resumed)))
	}

	m.StoppedBy = "the frontier ran out"
	for hop := 0; hop <= opt.Depth; hop++ {
		orderFrontier(frontier)
		named := map[graph.URI]bool{}
		for _, ref := range frontier {
			if err := ctx.Err(); err != nil {
				m.StoppedBy = "cancelled"
				m.Spent = int(spent.Load())
				m.FinishedAt = time.Now()
				return m, err
			}
			if read[ref] {
				continue
			}
			if opt.Budget > 0 && int(spent.Load()) >= opt.Budget {
				m.StoppedBy = fmt.Sprintf("the budget of %d requests ran out at hop %d", opt.Budget, hop)
				logf("note: " + m.StoppedBy)
				m.Spent = int(spent.Load())
				m.FinishedAt = time.Now()
				return m, nil
			}
			read[ref] = true

			col := NewCollector()
			if err := c.Collect(ctx, ref, opt.Claims, col); err != nil {
				// A refusal is an answer about this reference rather than a failed
				// crawl, so it is written down and the walk goes on.
				m.Refusals = append(m.Refusals, Note{Ref: ref, Reason: err.Error()})
				logf(fmt.Sprintf("note: %s: %v", ref, err))
				continue
			}
			claimed, err := store.PutClaims(col.Set.Edges())
			if err != nil {
				return m, err
			}
			m.Claims += claimed
			for _, rec := range col.Records {
				uri, err := store.PutRecord(rec)
				if err != nil {
					logf(fmt.Sprintf("note: %s: %v", ref, err))
					continue
				}
				m.Records++
				// A record that is read is read, whichever spelling found it, so the
				// node never goes back on the frontier under its other name.
				read[refFor(uri)] = true
			}
			for _, u := range col.Set.Nodes() {
				if !nodes[u] {
					nodes[u] = true
					m.Nodes++
				}
				named[u] = true
				if !opt.Uploads {
					continue
				}
				for _, pl := range derivedPlaylists(u) {
					named[pl] = true
					if !nodes[pl] {
						nodes[pl] = true
						m.Nodes++
					}
					// A derived id is arithmetic on a channel id, so this is a node the
					// store now knows about with nothing having been asked. Writing it
					// down is what puts it on the frontier.
					if err := store.Sight(pl); err != nil {
						return m, err
					}
				}
			}
			logf(fmt.Sprintf("%s: %d claims, %d requests spent", ref, col.Set.Len(), spent.Load()))
		}
		if hop == opt.Depth {
			m.StoppedBy = fmt.Sprintf("depth %d, which is as far as it was asked to go", opt.Depth)
			break
		}
		frontier = nextFrontier(store, named, read)
		if len(frontier) == 0 {
			break
		}
	}

	m.Spent = int(spent.Load())
	m.FinishedAt = time.Now()
	return m, nil
}

// nextFrontier is what the last hop named, minus everything already read and
// everything nothing can read.
//
// The store has the last word on what counts as unread, so a node this crawl
// only heard of and a previous crawl fetched is not fetched twice.
func nextFrontier(store *Store, named map[graph.URI]bool, read map[string]bool) []string {
	var out []string
	seen := map[string]bool{}
	for u := range named {
		ref := ReadableRef(u)
		if ref == "" || read[ref] || seen[ref] {
			continue
		}
		if skipOnFrontier(ref) {
			continue
		}
		if n, err := store.Node(u); err == nil && n != nil && n.Read() {
			continue
		}
		seen[ref] = true
		out = append(out, ref)
	}
	sort.Strings(out)
	return out
}

// skipOnFrontier is the short list of things a crawl will not follow to.
//
// A channel record names its popular playlist, and reading it costs a request
// for the videos UULF already gave in another order. A mix is generated per
// viewer and browses to nothing. Neither is refused when it is a seed: somebody
// who asks for one by name is asking a different question.
func skipOnFrontier(ref string) bool {
	switch ytid.Classify(ref).Kind {
	case ytid.Popular, ytid.Mix:
		return true
	default:
		return false
	}
}

// orderFrontier puts the cheapest claims first. Doc 04 section 3.2 measured the
// order: a playlist browse came back with 98 items in one response and a channel
// tab with about thirty, every one of them a claim about a video nobody
// fetched, while a watch page is 1.3 MB for one video and its neighbours. So a
// budget spent on listings buys an order of magnitude more graph than the same
// budget spent on watch pages.
func orderFrontier(refs []string) {
	sort.SliceStable(refs, func(i, j int) bool {
		a, b := frontierRank(refs[i]), frontierRank(refs[j])
		if a != b {
			return a < b
		}
		return refs[i] < refs[j]
	})
}

func frontierRank(ref string) int {
	switch ytid.Classify(ref).Kind {
	case ytid.Uploads, ytid.Videos, ytid.Shorts, ytid.Streams, ytid.Popular, ytid.Playlist, ytid.Album:
		return 0
	case ytid.Channel, ytid.Handle, ytid.LegacyUser, ytid.LegacyCustom:
		return 1
	case ytid.MusicAlbum, ytid.MusicArtist:
		return 2
	default:
		return 3
	}
}

// derivedPlaylists is the uploads family of a channel node, and nothing at all
// for any other node. UU, UULF, UUSH and UULV: one request each for a page of
// videos, and no request at all to work the id out.
func derivedPlaylists(u graph.URI) []graph.URI {
	p, ok := graph.Parse(u)
	if !ok || p.IsFragment() || p.Kind != graph.Channel {
		return nil
	}
	pl, ok := ytid.PlaylistsFor(p.ID)
	if !ok {
		return nil
	}
	// Popular is left off on purpose: it is the same videos as UULF in a
	// different order, so it costs a request to learn nothing new.
	out := make([]graph.URI, 0, 4)
	for _, id := range []string{pl.Uploads, pl.Videos, pl.Shorts, pl.Streams} {
		if id != "" {
			out = append(out, graph.PlaylistURI(id))
		}
	}
	return out
}

// crawlPolicy writes down what this crawl will not do before it does anything,
// so the manifest can say why a store has no comment claims in it.
func crawlPolicy(opt CrawlOptions) []Note {
	out := []Note{
		{Ref: "mixes", Reason: "never queued: an RD id is generated per viewer and browses to nothing, so queuing one spends a request to be told so"},
		{Ref: "popular", Reason: "never queued: a channel's UULP is its UULF videos in view order, so following it spends a request on videos the crawl already has"},
		{Ref: "formats", Reason: "off: a player call per video costs a request and produces no claim beyond the caption tracks"},
	}
	if opt.Claims.Comments == 0 {
		out = append(out, Note{Ref: "comments", Reason: "off: Restricted Mode refuses the comment section at tier 0 on this address, and --comments turns it back on"})
	}
	if opt.Claims.Posts == 0 {
		out = append(out, Note{Ref: "posts", Reason: "off: the community tab refuses an anonymous read, and --posts turns it back on"})
	}
	if !opt.Uploads {
		out = append(out, Note{Ref: "uploads", Reason: "off: a channel's derived playlists were kept off the frontier by --no-uploads"})
	}
	if !opt.Claims.Captions {
		out = append(out, Note{Ref: "captions", Reason: "off: the caption list needs an ANDROID player call per video, and --captions turns it on"})
	}
	if !opt.Claims.Music {
		out = append(out, Note{Ref: "music", Reason: "off: reading a video through YouTube Music is a request per video, and --music turns it on"})
	}
	return out
}

// ReadableRef turns a node back into something Collect can read, or the empty
// string when nothing can read it.
//
// A comment, a hashtag and an external URL are named by claims and are never
// fetched: a comment has no address of its own, a hashtag page is a feed rather
// than an entity, and an external URL is somebody else's site.
func ReadableRef(u graph.URI) string {
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

// refFor is ReadableRef for a node just written, so a record fetched under one
// spelling marks the reference read under the other.
func refFor(u graph.URI) string {
	if ref := ReadableRef(u); ref != "" {
		return ref
	}
	return string(u)
}
