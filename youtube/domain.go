package youtube

import (
	"bufio"
	"context"
	"errors"
	"os"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes YouTube as a kit Domain: a driver that a multi-domain host
// (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/ytb-cli/youtube"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then dereferences
// youtube:// URIs by routing to the operations Register installs. The standalone
// ytb binary assembles the same operations through cli.NewApp, so there is one
// definition of each operation, not two.
func init() { kit.Register(Domain{}) }

// Domain is the YouTube driver. It carries no state; the per-run client is built
// by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against, and
// the identity a host reuses for help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme:  "youtube",
		Aliases: []string{"yt", "ytb"},
		Hosts:   []string{"youtube.com", "www.youtube.com", "m.youtube.com", "youtu.be", "music.youtube.com"},
		Identity: kit.Identity{
			Binary: "ytb",
			Short:  "Read YouTube videos, channels, playlists, and more",
			Site:   "https://www.youtube.com",
			Repo:   "https://github.com/tamnd/ytb-cli",
		},
	}
}

// Register installs the client factory and every YouTube record operation onto
// app. A resolver op names its own record type and answers `ant get`; a List op
// enumerates a parent resource's members and answers `ant ls`. The non-record
// commands (download, transcript text, the local store, config) are not
// operations; the standalone binary adds them as escape hatches in cli.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	// Resolver ops: one canonical record per id, the home of `ant get`.
	kit.Handle(app, kit.OpMeta{Name: "video", Group: "read",
		Summary: "Resolve one or more videos to full metadata",
		URIType: "video", Resolver: true,
		Args: []kit.Arg{{Name: "ref", Help: "video id or URL (or - for stdin)", Variadic: true}}}, getVideo)
	kit.Handle(app, kit.OpMeta{Name: "channel", Group: "read", Single: true,
		Summary: "Resolve a channel to its header metadata",
		URIType: "channel", Resolver: true,
		Args: []kit.Arg{{Name: "ref", Help: "channel id, @handle, or URL"}}}, getChannel)
	kit.Handle(app, kit.OpMeta{Name: "playlist", Group: "read", Single: true,
		Summary: "Resolve a playlist to its header metadata",
		URIType: "playlist", Resolver: true,
		Args: []kit.Arg{{Name: "ref", Help: "playlist id or URL"}}}, getPlaylist)

	// List ops: members of a parent resource, the home of `ant ls`. They emit
	// records that are themselves addressable, so a host can keep following.
	kit.Handle(app, kit.OpMeta{Name: "uploads", Group: "read", List: true,
		Summary: "Stream a channel's uploads (--tab videos|shorts|streams)",
		URIType: "channel",
		Args:    []kit.Arg{{Name: "ref", Help: "channel id, @handle, or URL"}}}, listUploads)
	kit.Handle(app, kit.OpMeta{Name: "playlists", Group: "read", List: true,
		Summary: "List a channel's playlists",
		URIType: "channel",
		Args:    []kit.Arg{{Name: "ref", Help: "channel id, @handle, or URL"}}}, listChannelPlaylists)
	kit.Handle(app, kit.OpMeta{Name: "items", Group: "read", List: true,
		Summary: "Stream a playlist's videos",
		URIType: "playlist",
		Args:    []kit.Arg{{Name: "ref", Help: "playlist id or URL"}}}, listItems)
	kit.Handle(app, kit.OpMeta{Name: "related", Group: "read", List: true,
		Summary: "List the related-videos graph for a video",
		URIType: "video",
		Args:    []kit.Arg{{Name: "ref", Help: "video id or URL"}}}, listRelated)
	kit.Handle(app, kit.OpMeta{Name: "comments", Group: "read",
		Summary: "Stream a video's comments and replies",
		URIType: "video",
		Args:    []kit.Arg{{Name: "ref", Help: "video id or URL"}}}, listComments)
	kit.Handle(app, kit.OpMeta{Name: "community", Group: "read",
		Summary: "Stream a channel's community posts",
		URIType: "channel",
		Args:    []kit.Arg{{Name: "ref", Help: "channel id, @handle, or URL"}}}, listCommunity)

	// Feeds and discovery. Each hit names a video, so the surface stays followable.
	kit.Handle(app, kit.OpMeta{Name: "hashtag", Group: "read",
		Summary: "Stream a hashtag feed",
		URIType: "video",
		Args:    []kit.Arg{{Name: "tag", Help: "hashtag, with or without the #"}}}, listHashtag)
	kit.Handle(app, kit.OpMeta{Name: "trending", Group: "read",
		Summary: "Stream trending videos",
		URIType: "video"}, listTrending)
	kit.Handle(app, kit.OpMeta{Name: "search", Group: "read",
		Summary: "Search with the full filter grid",
		URIType: "video",
		Args:    []kit.Arg{{Name: "query", Help: "search terms", Variadic: true}}}, search)
	kit.Handle(app, kit.OpMeta{Name: "suggest", Group: "read",
		Summary: "Search autocomplete suggestions",
		Args:    []kit.Arg{{Name: "query", Help: "partial query", Variadic: true}}}, suggest)
}

// newClient builds the YouTube client from the host-resolved config. The
// interface language and content country travel in Config.Extra, where the
// standalone binary's --hl/--gl flags deposit them; a host leaves them at the
// en/US defaults.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	yc := DefaultConfig()
	if cfg.Workers > 0 {
		yc.Workers = cfg.Workers
	}
	if cfg.Rate > 0 {
		yc.Delay = cfg.Rate
	}
	if cfg.Retries > 0 {
		yc.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		yc.Timeout = cfg.Timeout
	}
	if hl := cfg.Extra["hl"]; hl != "" {
		yc.HL = hl
	}
	if gl := cfg.Extra["gl"]; gl != "" {
		yc.GL = gl
	}
	return NewClient(yc), nil
}

// --- inputs ---

type videoRef struct {
	Refs       []string `kit:"arg,variadic" help:"video id or URL (or - for stdin)"`
	Transcript bool     `kit:"flag" help:"fetch and attach the transcript text"`
	Lang       string   `kit:"flag" help:"preferred caption language for --transcript"`
	NoPlayer   bool     `kit:"flag,name=no-player" help:"skip /player (HTML-only, faster)"`
	Client     *Client  `kit:"inject"`
}

type channelRef struct {
	Ref    string  `kit:"arg" help:"channel id, @handle, or URL"`
	Client *Client `kit:"inject"`
}

type playlistRef struct {
	Ref    string  `kit:"arg" help:"playlist id or URL"`
	Client *Client `kit:"inject"`
}

type uploadsRef struct {
	Ref      string  `kit:"arg" help:"channel id, @handle, or URL"`
	Tab      string  `kit:"flag" help:"which tab: videos|shorts|streams" default:"videos" enum:"videos,shorts,streams"`
	Enrich   bool    `kit:"flag" help:"call /player per video for full metadata"`
	MaxPages int     `kit:"flag,name=max-pages" help:"max continuation pages (0 = unlimited)"`
	Client   *Client `kit:"inject"`
}

type pagedRef struct {
	Ref      string  `kit:"arg" help:"id, @handle, or URL"`
	MaxPages int     `kit:"flag,name=max-pages" help:"max continuation pages (0 = unlimited)"`
	Client   *Client `kit:"inject"`
}

type commentsRef struct {
	Ref      string  `kit:"arg" help:"video id or URL"`
	Replies  bool    `kit:"flag" help:"also fetch replies (parent_id set)"`
	Sort     string  `kit:"flag" help:"top|new"`
	MaxPages int     `kit:"flag,name=max-pages" help:"max continuation pages (0 = unlimited)"`
	Client   *Client `kit:"inject"`
}

type hashtagRef struct {
	Tag      string  `kit:"arg" help:"hashtag, with or without the #"`
	MaxPages int     `kit:"flag,name=max-pages" help:"max continuation pages (0 = unlimited)"`
	Client   *Client `kit:"inject"`
}

type trendingRef struct {
	Category string  `kit:"flag" help:"music|gaming|movies|news"`
	MaxPages int     `kit:"flag,name=max-pages" help:"max continuation pages (0 = unlimited)"`
	Client   *Client `kit:"inject"`
}

type searchRef struct {
	Query      []string `kit:"arg,variadic" help:"search terms"`
	Type       string   `kit:"flag" help:"video|channel|playlist"`
	Sort       string   `kit:"flag" help:"relevance|date|views|rating"`
	Duration   string   `kit:"flag" help:"short|medium|long"`
	UploadDate string   `kit:"flag,name=upload-date" help:"hour|today|week|month|year"`
	HD         bool     `kit:"flag,name=hd" help:"HD only"`
	CC         bool     `kit:"flag,name=cc" help:"closed captions / subtitles"`
	Creative   bool     `kit:"flag,name=creative-commons" help:"Creative Commons license"`
	Live       bool     `kit:"flag" help:"live only"`
	FourK      bool     `kit:"flag,name=4k" help:"4K only"`
	ThreeSixty bool     `kit:"flag,name=360" help:"360-degree video"`
	HDR        bool     `kit:"flag,name=hdr" help:"HDR only"`
	VR180      bool     `kit:"flag,name=vr180" help:"VR180 only"`
	MaxPages   int      `kit:"flag,name=max-pages" help:"max continuation pages (0 = unlimited)"`
	Client     *Client  `kit:"inject"`
}

type suggestRef struct {
	Query  []string `kit:"arg,variadic" help:"partial query"`
	Client *Client  `kit:"inject"`
}

// pageOpts builds the streaming bound from a continuation-page cap and the
// enrich flag. The row limit (-n) is enforced by kit around emit, so Max stays 0.
func pageOpts(maxPages int, enrich bool) PageOptions {
	return PageOptions{Max: 0, MaxPages: maxPages, Enrich: enrich}
}

// --- handlers ---

func getVideo(ctx context.Context, in videoRef, emit func(*Video) error) error {
	refs := expandStdin(in.Refs)
	opt := VideoOptions{
		Player:     !in.NoPlayer,
		Next:       !in.NoPlayer,
		Transcript: in.Transcript,
		Lang:       in.Lang,
	}
	single := len(refs) == 1
	for _, ref := range refs {
		res, err := in.Client.FetchVideo(ctx, ref, opt)
		if err != nil {
			if single {
				return mapErr(err)
			}
			continue
		}
		if res == nil {
			if single {
				return errs.NotFound("video %q not found", ref)
			}
			continue
		}
		if err := emit(&res.Video); err != nil {
			return err
		}
	}
	return nil
}

func getChannel(ctx context.Context, in channelRef, emit func(*Channel) error) error {
	ch, err := in.Client.FetchChannel(ctx, in.Ref)
	if err != nil {
		return mapErr(err)
	}
	if ch == nil {
		return errs.NotFound("channel %q not found", in.Ref)
	}
	return emit(ch)
}

func getPlaylist(ctx context.Context, in playlistRef, emit func(*Playlist) error) error {
	pl, err := in.Client.FetchPlaylist(ctx, in.Ref)
	if err != nil {
		return mapErr(err)
	}
	if pl == nil {
		return errs.NotFound("playlist %q not found", in.Ref)
	}
	return emit(pl)
}

func listUploads(ctx context.Context, in uploadsRef, emit func(Video) error) error {
	tab := in.Tab
	if tab == "" {
		tab = "videos"
	}
	return mapErr(in.Client.StreamChannelTab(ctx, in.Ref, tab, pageOpts(in.MaxPages, in.Enrich), emit))
}

func listChannelPlaylists(ctx context.Context, in pagedRef, emit func(Playlist) error) error {
	return mapErr(in.Client.StreamChannelPlaylists(ctx, in.Ref, pageOpts(in.MaxPages, false), emit))
}

func listItems(ctx context.Context, in pagedRef, emit func(Video) error) error {
	return mapErr(in.Client.StreamPlaylistItems(ctx, in.Ref, pageOpts(in.MaxPages, false), func(pv PlaylistVideo, v Video) error {
		if v.VideoID == "" {
			v.VideoID = pv.VideoID
		}
		if v.URL == "" {
			v.URL = "https://www.youtube.com/watch?v=" + pv.VideoID
		}
		return emit(v)
	}))
}

func listRelated(ctx context.Context, in playlistRef, emit func(Video) error) error {
	res, err := in.Client.FetchVideo(ctx, in.Ref, VideoOptions{Player: false, Next: true})
	if err != nil {
		return mapErr(err)
	}
	if res == nil {
		return errs.NotFound("video %q not found", in.Ref)
	}
	for _, r := range res.Related {
		if err := emit(Video{
			VideoID: r.RelatedVideoID,
			URL:     "https://www.youtube.com/watch?v=" + r.RelatedVideoID,
		}); err != nil {
			return err
		}
	}
	return nil
}

func listComments(ctx context.Context, in commentsRef, emit func(Comment) error) error {
	opt := CommentOptions{MaxPages: in.MaxPages, Replies: in.Replies, Sort: in.Sort}
	return mapErr(in.Client.StreamComments(ctx, in.Ref, opt, emit))
}

func listCommunity(ctx context.Context, in pagedRef, emit func(CommunityPost) error) error {
	return mapErr(in.Client.StreamCommunity(ctx, in.Ref, pageOpts(in.MaxPages, false), emit))
}

func listHashtag(ctx context.Context, in hashtagRef, emit func(Video) error) error {
	return mapErr(in.Client.StreamHashtag(ctx, in.Tag, pageOpts(in.MaxPages, false), emit))
}

func listTrending(ctx context.Context, in trendingRef, emit func(Video) error) error {
	return mapErr(in.Client.Trending(ctx, in.Category, pageOpts(in.MaxPages, false), emit))
}

func search(ctx context.Context, in searchRef, emit func(any) error) error {
	filters := SearchFilters{
		Sort:           in.Sort,
		Type:           in.Type,
		Duration:       in.Duration,
		UploadDate:     in.UploadDate,
		HD:             in.HD,
		CC:             in.CC,
		CreativeCommon: in.Creative,
		Live:           in.Live,
		FourK:          in.FourK,
		ThreeSixty:     in.ThreeSixty,
		HDR:            in.HDR,
		VR180:          in.VR180,
	}
	return mapErr(in.Client.Search(ctx, strings.Join(in.Query, " "), filters, pageOpts(in.MaxPages, false), emit))
}

func suggest(ctx context.Context, in suggestRef, emit func(Suggestion) error) error {
	suggestions, err := in.Client.Suggest(ctx, strings.Join(in.Query, " "))
	if err != nil {
		return mapErr(err)
	}
	for _, s := range suggestions {
		if err := emit(Suggestion{Text: s}); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver: the URI-native string functions ---

// Classify turns any accepted input into the canonical (type, id), so a host
// resolves a youtube:// reference without a network call. A URL is read by what
// its path and query say; a watch link carries both a video and a playlist id
// and the video wins, matching what a person means by the link. A bare string is
// read by its shape: @handle and UC ids are channels, the playlist prefixes are
// playlists, and anything else is taken as a video id.
func (Domain) Classify(input string) (uriType, id string, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", errs.Usage("empty youtube reference")
	}
	if strings.Contains(input, "://") {
		if vid := ExtractVideoID(input); vid != "" {
			return "video", vid, nil
		}
		if pid := ExtractPlaylistID(input); pid != "" {
			return "playlist", pid, nil
		}
		if ch := channelRefID(input); ch != "" {
			return "channel", ch, nil
		}
		return "", "", errs.Usage("unrecognized youtube url: %q", input)
	}
	if ch := channelRefID(input); ch != "" {
		return "channel", ch, nil
	}
	if pid := ExtractPlaylistID(input); pid != "" {
		return "playlist", pid, nil
	}
	if vid := ExtractVideoID(input); vid != "" {
		return "video", vid, nil
	}
	return "", "", errs.Usage("unrecognized youtube reference: %q", input)
}

// Locate is the inverse: the live page URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "video":
		return "https://www.youtube.com/watch?v=" + id, nil
	case "playlist":
		return "https://www.youtube.com/playlist?list=" + id, nil
	case "channel":
		return NormalizeChannelURL(id), nil
	default:
		return "", errs.Usage("youtube has no resource type %q", uriType)
	}
}

// --- helpers ---

// channelRefID returns the canonical channel id for a bare reference: a UC id, a
// @handle, or a channel URL path, or "" when the input names no channel.
func channelRefID(input string) string {
	s := strings.TrimSpace(input)
	if strings.HasPrefix(s, "UC") && !strings.ContainsAny(s, "/ ") {
		return s
	}
	if strings.HasPrefix(s, "@") {
		return s
	}
	if strings.Contains(strings.ToLower(s), "youtube.com/") {
		// Pull the channel id from a /channel/, /@handle, /c/, or /user/ URL.
		if seg := lastChannelSegment(s); seg != "" {
			return seg
		}
	}
	return ""
}

// lastChannelSegment extracts the channel identifier from a youtube.com URL,
// keeping the @ on a handle so NormalizeChannelURL rebuilds the right path.
func lastChannelSegment(u string) string {
	i := strings.Index(strings.ToLower(u), "youtube.com/")
	if i < 0 {
		return ""
	}
	path := u[i+len("youtube.com/"):]
	if q := strings.IndexAny(path, "?#"); q >= 0 {
		path = path[:q]
	}
	path = strings.TrimSuffix(path, "/")
	switch {
	case strings.HasPrefix(path, "channel/"):
		return strings.TrimPrefix(path, "channel/")
	case strings.HasPrefix(path, "c/"):
		return strings.TrimPrefix(path, "c/")
	case strings.HasPrefix(path, "user/"):
		return strings.TrimPrefix(path, "user/")
	case strings.HasPrefix(path, "@"):
		if j := strings.Index(path, "/"); j >= 0 {
			return path[:j]
		}
		return path
	}
	return ""
}

// expandStdin replaces a lone "-" argument with the newline-delimited ids read
// from stdin, powering `ytb search -o url | ytb video -`.
func expandStdin(refs []string) []string {
	if len(refs) != 1 || refs[0] != "-" {
		return refs
	}
	var out []string
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// mapErr converts a youtube library error into the kit error kind that carries
// the right exit code, so a host renders the same outcomes the binary does. The
// stream stop sentinels (kit's own limit signal and youtube's ErrStop) are clean
// stops, never failures.
func mapErr(err error) error {
	switch {
	case err == nil, errors.Is(err, ErrStop):
		return nil
	case errors.Is(err, ErrCommentsRestricted):
		return errs.Unsupported("comments are hidden by Restricted Mode; YouTube applies this to some datacenter requests")
	default:
		return err
	}
}
