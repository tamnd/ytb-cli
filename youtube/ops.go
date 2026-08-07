package youtube

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
	"github.com/tamnd/ytb-cli/pkg/graph"
	"github.com/tamnd/ytb-cli/pkg/srv3"
)

// ops.go registers the reads whose command line is hand-written. Spec 3005 doc
// 06 section 4: one kit.Handle op per read, so each is an HTTP route under `ytb
// serve` and an MCP tool under `ytb mcp` from a single registration.
//
// Everything here carries NoCLI, because the command already exists in cli/ and
// a generated subcommand would shadow it: `ytb formats` prints a table somebody
// laid out, and the reflected version of the same op prints every field on the
// record. Without NoCLI a domain has to pick one surface or the other, which is
// how `ytb serve` ended up answering 404 to two thirds of what the binary reads.
//
// The pairing is checked by a test rather than by hand. cli.TestEveryReadIsServed
// walks the commands and fails when one has neither an op nor a written reason,
// so a new read cannot quietly land on the command line alone.

func registerReadOps(app *kit.App) {
	// The sidecars: one video id in, one list out, each its own request.
	kit.Handle(app, kit.OpMeta{Name: "formats", Group: "read", NoCLI: true,
		Summary: "List a video's streaming formats (metadata only)",
		URIType: "video",
		Args:    []kit.Arg{{Name: "ref", Help: "video id or URL"}}}, listFormats)
	kit.Handle(app, kit.OpMeta{Name: "captions", Group: "read", NoCLI: true,
		Summary: "List a video's caption tracks",
		URIType: "video",
		Args:    []kit.Arg{{Name: "ref", Help: "video id or URL"}}}, listCaptions)
	kit.Handle(app, kit.OpMeta{Name: "transcript", Group: "read", NoCLI: true,
		Summary: "Stream one caption track as timed lines",
		Long: `The command line renders the track as text, srt, vtt or json, because that is
what a person wants in a file. Here it is one record per line with its timings,
which is what a program wants, and both come off the same parse.`,
		URIType: "video",
		Args:    []kit.Arg{{Name: "ref", Help: "video id or URL"}}}, listTranscript)
	kit.Handle(app, kit.OpMeta{Name: "chapters", Group: "read", NoCLI: true,
		Summary: "List a video's chapters, with where each one came from",
		URIType: "video",
		Args:    []kit.Arg{{Name: "ref", Help: "video id or URL"}}}, listChapters)
	kit.Handle(app, kit.OpMeta{Name: "thumbnail", Group: "read", NoCLI: true,
		Summary: "List a video's thumbnail renditions, confirmed against the CDN",
		URIType: "video",
		Args:    []kit.Arg{{Name: "ref", Help: "video id or URL"}}}, listThumbnails)
	kit.Handle(app, kit.OpMeta{Name: "sponsorblock", Group: "read", NoCLI: true,
		Summary: "List the community SponsorBlock segments for a video",
		URIType: "video",
		Args:    []kit.Arg{{Name: "ref", Help: "video id or URL"}}}, listSponsorSegments)

	// Music. The parent is a route segment and a tool prefix here, not a command:
	// /v1/music/album, music_album. The command line keeps its own `ytb music`.
	kit.Handle(app, kit.OpMeta{Name: "search", Parent: "music", Group: "read", NoCLI: true,
		Summary: "Search YouTube Music: tracks, albums, artists and playlists",
		Args:    []kit.Arg{{Name: "query", Help: "search terms", Variadic: true}}}, musicSearch)
	kit.Handle(app, kit.OpMeta{Name: "artist", Parent: "music", Group: "read", NoCLI: true, Single: true,
		Summary: "Read a Music artist: discography, top songs, videos and related artists",
		URIType: "artist", Resolver: true,
		Args: []kit.Arg{{Name: "ref", Help: "artist browse id, channel id, or URL"}}}, getArtist)
	kit.Handle(app, kit.OpMeta{Name: "album", Parent: "music", Group: "read", NoCLI: true,
		Summary: "Read a Music album: the header, then its tracks",
		URIType: "album", Resolver: true,
		Args: []kit.Arg{{Name: "ref", Help: "album browse id or URL"}}}, getAlbum)
	kit.Handle(app, kit.OpMeta{Name: "playlist", Parent: "music", Group: "read", NoCLI: true,
		Summary: "Read a Music playlist: the header, then its tracks",
		URIType: "playlist",
		Args:    []kit.Arg{{Name: "ref", Help: "playlist id or URL"}}}, getMusicPlaylist)
	kit.Handle(app, kit.OpMeta{Name: "track", Parent: "music", Group: "read", NoCLI: true, Single: true,
		Summary: "Read one Music track, with its lyrics on request",
		URIType: "track", Resolver: true,
		Args: []kit.Arg{{Name: "ref", Help: "video id or URL"}}}, getTrack)

	// The graph plane. edges is one read and what it claims; graph walks the
	// frontier those claims name; predicates makes no request at all.
	kit.Handle(app, kit.OpMeta{Name: "edges", Group: "read", NoCLI: true,
		Summary: "The claims one read makes: subject, predicate, object, and who said so",
		Args:    []kit.Arg{{Name: "ref", Help: "any id, @handle, or URL", Variadic: true}}}, listEdges)
	kit.Handle(app, kit.OpMeta{Name: "graph", Group: "read", NoCLI: true,
		Summary: "Walk the frontier the claims name, on a budget counted in requests",
		Args:    []kit.Arg{{Name: "seed", Help: "any id, @handle, or URL", Variadic: true}}}, walkGraph)
	kit.Handle(app, kit.OpMeta{Name: "predicates", Group: "read", NoCLI: true,
		Summary: "The closed vocabulary: every predicate with its domain and its range"}, listPredicates)
	kit.Handle(app, kit.OpMeta{Name: "discover", Group: "read", NoCLI: true,
		Summary: "Breadth-first walk of the graph linked from a video, channel, or playlist",
		Args:    []kit.Arg{{Name: "seed", Help: "video, channel or playlist reference", Variadic: true}}}, listDiscovered)
}

// DefaultWalkBudget caps a streaming walk when the caller named no limit, so a
// walk always terminates instead of spidering YouTube forever.
const DefaultWalkBudget = 500

// --- inputs ---

// oneVideoRef is a read that takes a video and no options.
type oneVideoRef struct {
	Ref    string  `kit:"arg" help:"video id or URL"`
	Client *Client `kit:"inject"`
}

// formatsRef is the stream list and its three filters. --urls is not here: it
// deciphers playable URLs that expire in six hours and are bound to the IP that
// asked, so serving them to somebody else hands out something that does not work.
type formatsRef struct {
	Ref    string  `kit:"arg" help:"video id or URL"`
	Audio  bool    `kit:"flag" help:"audio-only adaptive formats"`
	Video  bool    `kit:"flag" help:"video-only adaptive formats"`
	Muxed  bool    `kit:"flag" help:"progressive (muxed) formats only"`
	Client *Client `kit:"inject"`
}

type transcriptRef struct {
	Ref       string  `kit:"arg" help:"video id or URL"`
	Lang      string  `kit:"flag" help:"caption language code"`
	Auto      bool    `kit:"flag" help:"take the auto-generated track"`
	Translate string  `kit:"flag" help:"machine-translate into this language code"`
	Client    *Client `kit:"inject"`
}

type thumbnailsRef struct {
	Ref         string  `kit:"arg" help:"video id or URL"`
	Unconfirmed bool    `kit:"flag" help:"list the constructed URLs without HEADing them"`
	Client      *Client `kit:"inject"`
}

type sponsorRef struct {
	Ref        string   `kit:"arg" help:"video id or URL"`
	Categories []string `kit:"flag" help:"segment categories (default all): sponsor,selfpromo,intro,outro"`
	Client     *Client  `kit:"inject"`
}

type musicSearchRef struct {
	Query    []string `kit:"arg,variadic" help:"search terms"`
	Type     string   `kit:"flag" help:"song|video|album|artist|playlist|podcast|episode"`
	MaxPages int      `kit:"flag,name=max-pages" help:"max continuation pages (0 = unlimited)"`
	Client   *Client  `kit:"inject"`
}

type musicRef struct {
	Ref    string  `kit:"arg" help:"browse id, video id, or URL"`
	Client *Client `kit:"inject"`
}

type trackRef struct {
	Ref    string  `kit:"arg" help:"video id or URL"`
	Lyrics bool    `kit:"flag" help:"fetch the lyrics tab as well (1 more request)"`
	Client *Client `kit:"inject"`
}

// claimRef is a claim collection. Every flag past the references is another read
// and says what it costs, exactly as the command line states it.
type claimRef struct {
	Refs     []string `kit:"arg,variadic,name=ref" help:"any id, @handle, or URL"`
	Captions bool     `kit:"flag" help:"read the ANDROID player for the caption list (1 request)"`
	Comments int      `kit:"flag" help:"read up to n comments (1 request per page of about 20)"`
	Items    int      `kit:"flag" help:"read up to n playlist items (1 request per page)"`
	Featured bool     `kit:"flag" help:"read a channel's home tab for its featured channels (1 request)"`
	Posts    int      `kit:"flag" help:"read up to n community posts (the tab refuses tier 0 today)"`
	Music    bool     `kit:"flag" help:"read the same id through YouTube Music (1 request)"`
	Client   *Client  `kit:"inject"`
}

func (in claimRef) options() ClaimOptions {
	return ClaimOptions{
		Captions: in.Captions,
		Comments: in.Comments,
		Items:    in.Items,
		Featured: in.Featured,
		Posts:    in.Posts,
		Music:    in.Music,
	}
}

// graphRef is claimRef with the walk on top of it.
//
// The fields are spelled out rather than embedded, because kit binds an input's
// declared fields and does not walk a promoted one. Embedding compiles, binds
// nothing, and hands the handler a nil client, which is a panic and not a
// message. Anything added to claimRef belongs here too.
type graphRef struct {
	Refs     []string `kit:"arg,variadic,name=seed" help:"any id, @handle, or URL"`
	Captions bool     `kit:"flag" help:"read the ANDROID player for the caption list (1 request)"`
	Comments int      `kit:"flag" help:"read up to n comments (1 request per page of about 20)"`
	Items    int      `kit:"flag" help:"read up to n playlist items (1 request per page)"`
	Featured bool     `kit:"flag" help:"read a channel's home tab for its featured channels (1 request)"`
	Posts    int      `kit:"flag" help:"read up to n community posts (the tab refuses tier 0 today)"`
	Music    bool     `kit:"flag" help:"read the same id through YouTube Music (1 request)"`
	Depth    int      `kit:"flag" help:"hops to follow from each seed (0 = the seeds only)" default:"1"`
	Budget   int      `kit:"flag" help:"how many requests the walk may spend" default:"25"`
	Client   *Client  `kit:"inject"`
}

func (in graphRef) options() ClaimOptions {
	return ClaimOptions{
		Captions: in.Captions,
		Comments: in.Comments,
		Items:    in.Items,
		Featured: in.Featured,
		Posts:    in.Posts,
		Music:    in.Music,
	}
}

type discoverRef struct {
	Seeds  []string `kit:"arg,variadic,name=seed" help:"video, channel or playlist reference"`
	Depth  int      `kit:"flag" help:"hops to follow from each seed (0 = seeds only)" default:"1"`
	Fanout int      `kit:"flag" help:"max neighbors to follow per edge (0 = unlimited)" default:"25"`
	Follow string   `kit:"flag" help:"edges to follow: a preset or a comma list" default:"content"`
	Max    int      `kit:"flag,name=limit,inherit" help:"stop after n nodes"`
	Client *Client  `kit:"inject"`
}

// --- handlers ---

func listFormats(ctx context.Context, in formatsRef, emit func(VideoFormat) error) error {
	list, err := in.Client.FormatList(ctx, in.Ref)
	if err != nil {
		return ExitError(err)
	}
	if list == nil {
		return errs.NoResults("no formats available")
	}
	for _, f := range list.Formats {
		if !FormatMatches(f, in.Audio, in.Video, in.Muxed) {
			continue
		}
		if err := emit(f); err != nil {
			return err
		}
	}
	return nil
}

func listCaptions(ctx context.Context, in oneVideoRef, emit func(CaptionTrack) error) error {
	tracks, err := in.Client.Captions(ctx, in.Ref)
	if err != nil {
		return ExitError(err)
	}
	for _, t := range tracks {
		if err := emit(t); err != nil {
			return err
		}
	}
	return nil
}

func listTranscript(ctx context.Context, in transcriptRef, emit func(srv3.Cue) error) error {
	doc, _, err := in.Client.Transcript(ctx, in.Ref, TranscriptOptions{
		Lang: in.Lang, Auto: in.Auto, TranslateTo: in.Translate,
	})
	if err != nil {
		return ExitError(err)
	}
	for _, c := range doc.Cues {
		if err := emit(c); err != nil {
			return err
		}
	}
	return nil
}

func listChapters(ctx context.Context, in oneVideoRef, emit func(Chapter) error) error {
	res, err := in.Client.FetchVideo(ctx, in.Ref, VideoOptions{Next: true})
	if err != nil {
		return ExitError(err)
	}
	if res == nil {
		return errs.NotFound("video %q not found", in.Ref)
	}
	for _, c := range res.Chapters {
		if err := emit(c); err != nil {
			return err
		}
	}
	return nil
}

func listThumbnails(ctx context.Context, in thumbnailsRef, emit func(Thumbnail) error) error {
	id := ExtractVideoID(in.Ref)
	if id == "" {
		id = in.Ref
	}
	thumbs := Thumbnails(id)
	if !in.Unconfirmed {
		thumbs = in.Client.ConfirmThumbnails(ctx, thumbs)
	}
	for _, t := range thumbs {
		if err := emit(t); err != nil {
			return err
		}
	}
	return nil
}

func listSponsorSegments(ctx context.Context, in sponsorRef, emit func(SponsorSegment) error) error {
	id := ExtractVideoID(in.Ref)
	if id == "" {
		id = in.Ref
	}
	segs, err := in.Client.SponsorSegments(ctx, id, in.Categories)
	if err != nil {
		return ExitError(err)
	}
	for _, s := range segs {
		if err := emit(s); err != nil {
			return err
		}
	}
	return nil
}

func musicSearch(ctx context.Context, in musicSearchRef, emit func(any) error) error {
	query := strings.Join(in.Query, " ")
	return ExitError(in.Client.MusicSearch(ctx, query, in.Type, pageOpts(in.MaxPages, false), emit))
}

func getArtist(ctx context.Context, in musicRef, emit func(*Artist) error) error {
	artist, err := in.Client.FetchArtist(ctx, in.Ref)
	if err != nil {
		return ExitError(err)
	}
	if artist == nil {
		return errs.NotFound("artist %q not found", in.Ref)
	}
	return emit(artist)
}

// getAlbum emits the album and then its tracks, the same stream the command
// line prints. The album record has no track list on it, because a track is
// addressable on its own and nesting it would give the same song two homes.
func getAlbum(ctx context.Context, in musicRef, emit func(any) error) error {
	album, tracks, err := in.Client.FetchAlbum(ctx, in.Ref)
	if err != nil {
		return ExitError(err)
	}
	if album == nil {
		return errs.NotFound("album %q not found", in.Ref)
	}
	if err := emit(album); err != nil {
		return err
	}
	for _, t := range tracks {
		if err := emit(t); err != nil {
			return err
		}
	}
	return nil
}

func getMusicPlaylist(ctx context.Context, in musicRef, emit func(any) error) error {
	header, tracks, err := in.Client.FetchMusicPlaylist(ctx, in.Ref)
	if err != nil {
		return ExitError(err)
	}
	if header != nil {
		if err := emit(header); err != nil {
			return err
		}
	}
	for _, t := range tracks {
		if err := emit(t); err != nil {
			return err
		}
	}
	return nil
}

func getTrack(ctx context.Context, in trackRef, emit func(*Track) error) error {
	track, err := in.Client.FetchTrack(ctx, in.Ref, in.Lyrics)
	if err != nil {
		return ExitError(err)
	}
	if track == nil {
		return errs.NotFound("track %q not found", in.Ref)
	}
	return emit(track)
}

func listEdges(ctx context.Context, in claimRef, emit func(graph.Edge) error) error {
	set := graph.NewSet()
	var failed error
	for _, ref := range in.Refs {
		if err := in.Client.Claims(ctx, ref, in.options(), set); err != nil {
			// One bad reference should not throw away the claims the others made.
			failed = err
		}
	}
	if set.Len() == 0 && failed != nil {
		return ExitError(failed)
	}
	for _, e := range set.Edges() {
		if err := emit(e); err != nil {
			return err
		}
	}
	return nil
}

func walkGraph(ctx context.Context, in graphRef, emit func(graph.Edge) error) error {
	set := graph.NewSet()
	if err := in.Client.WalkClaims(ctx, in.Refs, in.options(), ClaimWalk{Depth: in.Depth, Budget: in.Budget}, set); err != nil {
		return ExitError(err)
	}
	for _, e := range set.Edges() {
		if err := emit(e); err != nil {
			return err
		}
	}
	return nil
}

func listPredicates(_ context.Context, _ struct{}, emit func(graph.PredicateInfo) error) error {
	for _, info := range graph.All() {
		if err := emit(info); err != nil {
			return err
		}
	}
	return nil
}

func listDiscovered(ctx context.Context, in discoverRef, emit func(*Node) error) error {
	edges, err := ParseEdges(in.Follow)
	if err != nil {
		return errs.Usage("%s", err)
	}
	seeds := make([]Seed, 0, len(in.Seeds))
	for _, ref := range in.Seeds {
		s, err := ParseSeed(ref)
		if err != nil {
			return errs.Usage("%s", err)
		}
		seeds = append(seeds, s)
	}
	// The default budget is the command line's, so a caller who names no limit
	// gets a walk that stops rather than one that spiders YouTube forever.
	max := in.Max
	if max <= 0 {
		max = DefaultWalkBudget
	}
	return ExitError(in.Client.Walk(ctx, seeds, WalkOptions{
		Depth: in.Depth, Max: max, Fanout: in.Fanout, Edges: edges,
	}, emit))
}

// --- shared with cli ---

// FormatMatches applies the audio/video/muxed filter to one format. It is here
// rather than in cli because the command line and the served op filter the same
// list and there is no version of this where the two should differ.
func FormatMatches(f VideoFormat, audio, video, muxed bool) bool {
	isAudio := strings.HasPrefix(f.MimeType, "audio/")
	isVideoOnly := f.IsAdaptive && strings.HasPrefix(f.MimeType, "video/")
	switch {
	case muxed:
		return !f.IsAdaptive
	case audio:
		return isAudio
	case video:
		return isVideoOnly
	default:
		return true
	}
}

// ClaimWalk bounds a claim walk: how many hops to follow, and how many requests
// it may spend getting there.
type ClaimWalk struct {
	// Depth is hops from the seeds. Depth 0 is the seeds and nothing else, which
	// is exactly what a claim collection over the same references does.
	Depth int
	// Budget is in requests and it is counted, not estimated. A cache hit never
	// reaches the counter, so a second walk over the same seeds gets further on
	// the same budget.
	Budget int
	// Note receives the walk's asides: a reference that would not read, and the
	// budget running out mid-level. It may be nil.
	Note func(string)
}

// WalkClaims collects claims from the seeds, then from the nodes those claims
// named, and so on, adding everything it sees to set.
//
// The frontier is every node the claims so far have named and nothing has read.
// Most of it is videos, because one watch page names twenty related ones. Only
// nodes ytb can read are followed, so an external URL and a hashtag are named
// and never fetched.
func (c *Client) WalkClaims(ctx context.Context, seeds []string, opt ClaimOptions, w ClaimWalk, set *graph.Set) error {
	note := w.Note
	if note == nil {
		note = func(string) {}
	}

	// The hook is chained rather than replaced, because the command line installs
	// its own counter around this call to print what the walk spent, and a walk
	// that quietly took the hook away would report zero.
	var spent atomic.Int64
	prev := c.onRequest
	c.SetOnRequest(func(method, url string) {
		spent.Add(1)
		if prev != nil {
			prev(method, url)
		}
	})
	defer c.SetOnRequest(prev)

	// read is by reference rather than by URI because a seed can be a handle,
	// which has no URI until something resolves it, and reading the same channel
	// twice under two spellings is still two requests.
	read := map[string]bool{}
	seen := map[graph.URI]bool{}
	frontier := seeds

	for hop := 0; hop <= w.Depth; hop++ {
		stopped := false
		for _, ref := range frontier {
			if read[ref] {
				continue
			}
			if w.Budget > 0 && spent.Load() >= int64(w.Budget) {
				note(fmt.Sprintf("budget of %d requests reached at hop %d", w.Budget, hop))
				stopped = true
				break
			}
			read[ref] = true
			if err := c.Claims(ctx, ref, opt, set); err != nil {
				note(err.Error())
			}
		}
		if stopped || hop == w.Depth {
			break
		}
		var next []string
		for _, u := range set.Nodes() {
			if seen[u] {
				continue
			}
			seen[u] = true
			if ref := ReadableRef(u); ref != "" && !read[ref] {
				next = append(next, ref)
			}
		}
		frontier = next
		if len(frontier) == 0 {
			break
		}
	}
	if set.Len() == 0 {
		return errs.NoResults("no claims")
	}
	return nil
}
