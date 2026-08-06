package youtube

import (
	"context"
	"fmt"

	"github.com/tamnd/ytb-cli/pkg/graph"
	"github.com/tamnd/ytb-cli/pkg/rdf"
	"github.com/tamnd/ytb-cli/pkg/ytid"
)

// claims.go is the read side of the graph plane: point it at anything YouTube
// has an id for and it comes back with the claims that read produced.
//
// The default is one request. Everything a watch page already carries is free,
// and each option below is a request the caller asked for, which is the same
// bargain VideoOptions makes in doc 03.

// ClaimOptions says which extra reads a claim collection may make. Each field
// costs requests and the comment on it says how many.
type ClaimOptions struct {
	// Captions calls the ANDROID player, which is the only client that answers
	// with a caption list on a video the WEB client calls UNPLAYABLE. One request.
	Captions bool
	// Comments streams up to this many comments. Zero reads none. One request per
	// page of about twenty.
	Comments int
	// Items streams up to this many playlist items. Zero reads none. One request
	// per page, which was 98 items on the playlist doc 04 measured.
	Items int
	// Featured reads a channel's home tab for its featured channels shelf, which
	// is the only tier 0 source of a channel to channel claim. One request.
	Featured bool
	// Posts streams up to this many community posts. The tab refuses tier 0 today,
	// so this usually returns nothing and says why.
	Posts int
	// Music reads the same id through YouTube Music. One request.
	Music bool
	// Microdata parses the page's own schema.org markup into Collector.Page,
	// which is the other side of ytb rdf --check. It costs no request: the markup
	// is already in the response.
	Microdata bool
}

// Collector is what a read fills in: the claims, and the record's own literals.
//
// Two lists because they are two different things. A claim is an edge between
// nodes and carries the observation that produced it; a title and a duration
// are neither. The RDF writer merges them, the store keeps them apart, and
// ytb edges prints only the first.
type Collector struct {
	Set *graph.Set
	// Statements are the record's fields as RDF, filled for every read.
	Statements []rdf.Statement
	// Page is what the page's own schema.org markup said, filled only when
	// ClaimOptions.Microdata asked for it. It is never merged into Statements:
	// the whole point of holding it separately is to compare the two.
	Page []rdf.Statement
	// Aliases are the addresses the site used for things this tool names by URI,
	// so a comparison can tell a different spelling from a different answer.
	Aliases map[string]string
}

// NewCollector returns a collector with an empty set.
func NewCollector() *Collector { return &Collector{Set: graph.NewSet()} }

func (c *Collector) addAliases(m map[string]string) {
	if len(m) == 0 {
		return
	}
	if c.Aliases == nil {
		c.Aliases = map[string]string{}
	}
	for k, v := range m {
		c.Aliases[k] = v
	}
}

// Claims reads one reference and writes what it says into set.
//
// The reference is anything ytid can classify. A handle costs one extra request
// to resolve, because a handle is not an identity and cannot be a node.
func (c *Client) Claims(ctx context.Context, ref string, opt ClaimOptions, set *graph.Set) error {
	col := &Collector{Set: set}
	return c.Collect(ctx, ref, opt, col)
}

// Collect is Claims with the record's literals kept as well as its claims.
func (c *Client) Collect(ctx context.Context, ref string, opt ClaimOptions, col *Collector) error {
	if col == nil {
		col = NewCollector()
	}
	if col.Set == nil {
		col.Set = graph.NewSet()
	}
	info := ytid.Classify(ref)
	switch info.Kind {
	case ytid.Video:
		return c.videoClaims(ctx, info.ID, opt, col)
	case ytid.Channel:
		return c.channelClaims(ctx, info.ID, opt, col)
	case ytid.Handle, ytid.LegacyUser, ytid.LegacyCustom:
		id, err := c.ResolveChannelID(ctx, ref)
		if err != nil {
			return err
		}
		return c.channelClaims(ctx, id, opt, col)
	case ytid.Playlist, ytid.Uploads, ytid.Videos, ytid.Shorts, ytid.Streams, ytid.Popular, ytid.Album:
		return c.playlistClaims(ctx, info.ID, opt, col)
	case ytid.MusicAlbum:
		return c.albumClaims(ctx, info.ID, col)
	case ytid.MusicArtist:
		return c.artistClaims(ctx, info.ID, col)
	default:
		return fmt.Errorf("nothing to read from %q: %s", ref, describeUnreadable(info))
	}
}

// describeUnreadable says why a classified id has no claims to give, in the
// words ytid used, because "unsupported" tells nobody anything.
func describeUnreadable(info ytid.Info) string {
	if info.Note != "" {
		return info.Note
	}
	return "it is not an id this tool can read"
}

func (c *Client) videoClaims(ctx context.Context, id string, opt ClaimOptions, col *Collector) error {
	res, err := c.FetchVideo(ctx, id, VideoOptions{Captions: opt.Captions, Microdata: opt.Microdata})
	if err != nil {
		return err
	}
	if res == nil {
		return fmt.Errorf("video not found: %s", id)
	}
	set := col.Set
	VideoClaims(set, res.Video)
	RelatedClaims(set, res.Video.VideoID, res.Related, res.Video.ProvFor("related"))
	col.Statements = append(col.Statements, VideoStatements(res.Video)...)
	if opt.Microdata {
		col.Page = append(col.Page, MicrodataStatements(res.Video.Microdata, res.Video.VideoID)...)
		col.addAliases(VideoAliases(res.Video))
	}

	if opt.Comments > 0 {
		var comments []Comment
		err := c.StreamComments(ctx, res.Video.VideoID, CommentOptions{Max: opt.Comments, Replies: true}, func(cm Comment) error {
			comments = append(comments, cm)
			return nil
		})
		// A refused comment section is an answer about the network, not a failed
		// read of the video, so the claims already collected stand.
		if err == nil {
			CommentClaims(set, comments, graph.Provenance{
				Source:  NormalizeVideoURL(res.Video.VideoID),
				Surface: SurfaceInnerTube,
				Client:  "WEB",
			})
		}
	}

	if opt.Music {
		if song, songErr := c.FetchSong(ctx, res.Video.VideoID, false); songErr == nil && song != nil {
			prov := graph.Provenance{Source: song.URL, Surface: SurfaceMusic, Client: "WEB_REMIX"}
			SongClaims(set, *song, prov)
			SeenAsClaims(set, res.Video.VideoID, song.VideoID, prov, song.Title)
		}
	}
	return nil
}

func (c *Client) channelClaims(ctx context.Context, id string, opt ClaimOptions, col *Collector) error {
	ch, err := c.FetchChannel(ctx, id, ChannelOptions{})
	if err != nil {
		return err
	}
	if ch == nil {
		return fmt.Errorf("channel not found: %s", id)
	}
	set := col.Set
	ChannelClaims(set, *ch)
	col.Statements = append(col.Statements, ChannelStatements(*ch)...)

	if opt.Featured {
		if _, featured, ferr := c.FeaturedChannels(ctx, ch.ChannelID); ferr == nil {
			FeaturedClaims(set, ch.ChannelID, featured, graph.Provenance{
				Source:  NormalizeChannelURL(ch.ChannelID),
				Surface: SurfaceBrowseHTML,
			})
		}
	}

	if opt.Posts > 0 {
		var posts []CommunityPost
		perr := c.StreamCommunity(ctx, ch.ChannelID, PageOptions{Max: opt.Posts}, func(p CommunityPost) error {
			posts = append(posts, p)
			return nil
		})
		if perr == nil {
			PostClaims(set, posts, graph.Provenance{
				Source:  NormalizeChannelURL(ch.ChannelID),
				Surface: SurfaceInnerTube,
				Client:  "WEB",
			})
		}
	}
	return nil
}

func (c *Client) playlistClaims(ctx context.Context, id string, opt ClaimOptions, col *Collector) error {
	set := col.Set
	if opt.Items <= 0 {
		p, err := c.FetchPlaylist(ctx, id)
		if err != nil {
			return err
		}
		if p == nil {
			return fmt.Errorf("playlist not found: %s", id)
		}
		PlaylistClaims(set, *p)
		col.Statements = append(col.Statements, PlaylistStatements(*p)...)
		return nil
	}

	// The header and the items come off the same browse, so asking for items
	// costs nothing beyond the paging.
	var items []Video
	p, err := c.StreamPlaylistWithHeader(ctx, id, PageOptions{Max: opt.Items}, func(pv PlaylistVideo, v Video) error {
		v.Position = pv.Position
		items = append(items, v)
		return nil
	})
	if err != nil {
		return err
	}
	if p == nil {
		return fmt.Errorf("playlist not found: %s", id)
	}
	PlaylistClaims(set, *p)
	PlaylistItemClaims(set, p.PlaylistID, items, p.Prov())
	col.Statements = append(col.Statements, PlaylistStatements(*p)...)
	return nil
}

func (c *Client) albumClaims(ctx context.Context, id string, col *Collector) error {
	album, songs, err := c.FetchAlbum(ctx, id)
	if err != nil {
		return err
	}
	if album == nil {
		return fmt.Errorf("album not found: %s", id)
	}
	AlbumClaims(col.Set, *album, songs, graph.Provenance{Source: album.URL, Surface: SurfaceMusic, Client: "WEB_REMIX"})
	return nil
}

func (c *Client) artistClaims(ctx context.Context, id string, col *Collector) error {
	artist, albums, songs, err := c.FetchArtist(ctx, id)
	if err != nil {
		return err
	}
	if artist == nil {
		return fmt.Errorf("artist not found: %s", id)
	}
	prov := graph.Provenance{Source: artist.URL, Surface: SurfaceMusic, Client: "WEB_REMIX"}
	for _, a := range albums {
		AlbumClaims(col.Set, a, nil, prov)
	}
	for _, s := range songs {
		SongClaims(col.Set, s, prov)
	}
	return nil
}
