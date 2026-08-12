package ytb

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/tamnd/any-cli/kit/errs"
	"github.com/tamnd/ytb-cli/pkg/ytid"
)

// ChannelOptions is what a channel read was asked for beyond the page itself.
type ChannelOptions struct {
	// NoAbout skips the about panel continuation. The links then come from the
	// ld+json, which has every destination and none of their titles, and the join
	// date, the lifetime view count and the country are not read at all.
	NoAbout bool
	// Counts reads the four derived playlists for their real counts, four extra
	// requests. Doc 01 section 2.5.
	Counts bool
}

// FetchChannel reads a channel.
//
// The page is one request and answers most of the record. The about panel is a
// second, a continuation off a token that is already on the page, and it is the
// only source of the join date, the lifetime view count, the country in words and
// the links with their titles. --counts is four more.
func (c *Client) FetchChannel(ctx context.Context, idOrURL string, opt ChannelOptions) (*Channel, error) {
	channelURL := ChannelHomeURL(idOrURL)
	data, code, err := c.FetchPageData(ctx, channelURL)
	if err != nil {
		return nil, fmt.Errorf("fetch channel %q: %w", idOrURL, err)
	}
	if code == 404 || data == nil || data.InitialData == nil {
		return nil, fmt.Errorf("channel %q: %w", idOrURL, ErrChannelNotFound)
	}
	ch := ParseChannelRecord(data, channelURL)
	if ch == nil {
		return nil, fmt.Errorf("channel %q: %w", idOrURL, ErrChannelNotFound)
	}
	root, _ := data.InitialData.(map[string]any)

	if opt.NoAbout {
		ch.miss("--no-about: no join date, no lifetime view count, no country, and the links have their destinations without their titles")
	} else {
		c.attachAbout(ctx, root, ch)
	}
	if opt.Counts {
		c.attachCounts(ctx, ch)
	} else {
		ch.miss("the four derived playlist counts were not read; --counts reads them, four requests")
	}
	c.stamp(ch)
	return ch, nil
}

// attachAbout folds the about panel into the record.
//
// The token is already on the page, so this is one continuation and not a second
// browse. A channel that serves no panel is not an error: the record keeps what
// the page said and missed names the fields the panel would have added.
func (c *Client) attachAbout(ctx context.Context, root map[string]any, ch *Channel) {
	token := FindAboutToken(root)
	if token == "" {
		ch.miss("this channel served no about panel, so there is no join date, no lifetime view count and no country")
		return
	}
	resp, err := NewInnerTube(c).BrowseContinuation(ctx, token)
	if err != nil {
		ch.miss("the about panel did not answer (%v), so there is no join date, no lifetime view count and no country", err)
		return
	}
	about := ParseChannelAbout(resp)
	if about == nil {
		ch.miss("the about panel answered with no aboutChannelViewModel in it")
		return
	}
	ch.addSurface(SurfaceInnerTube)
	ch.addClient("WEB")
	ch.ArtistBio = about.ArtistBio
	ch.Country = about.Country
	ch.JoinedText = about.JoinedDateText
	ch.JoinedAt = parseJoinedDate(about.JoinedDateText)
	if ch.JoinedText != "" && ch.JoinedAt.IsZero() {
		ch.miss("joined_text is %q and this read could not parse a date out of it, so joined_at is absent", ch.JoinedText)
	}
	if n := parseCountText(about.ViewsText); n > 0 {
		ch.ViewCount = n
		ch.setVia("view_count", "s2 about panel viewCountText, exact")
	}
	// The panel and the header both print a video count and they are not always the
	// same number. Where they differ the panel wins, because it is the fuller of
	// the two blocks, and via says which one this is. Neither of them is the
	// uploads playlist: on @RickAstleyYT both said 433 and the playlist held 435,
	// which is what --counts is for.
	if n := parseCountText(about.VideosText); n > 0 && n != ch.VideoCount {
		ch.setVia("video_count", fmt.Sprintf("s2 about panel videoCountText %s, where the s4 header said %s", about.VideosText, ch.VideoCountText))
		ch.VideoCount = n
		ch.VideoCountText = about.VideosText
	}
	if len(about.Links) > 0 {
		ch.Links = about.Links
		ch.setVia("links", "s2 about panel, with the titles the ld+json has no room for")
	}
	if about.CanonicalURL != "" {
		ch.CanonicalURL = about.CanonicalURL
	}
	if about.ArtistBio != "" {
		ch.IsArtist = true
	}
	if about.BusinessEmailText != "" {
		ch.miss("a business email exists on this channel and signed out YouTube answers %q rather than the address", about.BusinessEmailText)
	}
}

// attachCounts reads the four derived playlists and checks the arithmetic.
func (c *Client) attachCounts(ctx context.Context, ch *Channel) {
	ids, ok := ytid.PlaylistsFor(ch.ChannelID)
	if !ok {
		ch.miss("--counts needs a UC channel id and this read has none")
		return
	}
	counts := &ChannelCounts{
		UploadsID: ids.Uploads,
		VideosID:  ids.Videos,
		ShortsID:  ids.Shorts,
		StreamsID: ids.Streams,
	}
	for _, pair := range []struct {
		id  string
		out *int
	}{
		{ids.Uploads, &counts.Uploads},
		{ids.Videos, &counts.Videos},
		{ids.Shorts, &counts.Shorts},
		{ids.Streams, &counts.Streams},
	} {
		pl, err := c.FetchPlaylist(ctx, pair.id)
		if err != nil || pl == nil {
			// A channel with no streams has no UULV playlist at all, which is an
			// answer and not a failure: the count is zero and the sum still holds.
			ch.miss("playlist %s did not answer, so its count is 0 and the check below is not a check", pair.id)
			continue
		}
		*pair.out = int(pl.VideoCount)
		ch.addSource(NormalizePlaylistURL(pair.id))
	}
	counts.Agrees = counts.Videos+counts.Shorts+counts.Streams == counts.Uploads
	ch.Counts = counts
	if !counts.Agrees {
		ch.miss("the derived playlists do not add up: %s", counts)
	}
	// The uploads playlist is the count worth having, so it wins, and via names
	// the number it beat. On @RickAstleyYT the page says 433 videos and the
	// uploads playlist holds 435, and the two extra are real uploads that the
	// page's own counter does not include.
	if counts.Uploads > 0 && int64(counts.Uploads) != ch.VideoCount {
		ch.setVia("video_count", fmt.Sprintf("--counts, the uploads playlist %s holds %d where the page said %s",
			counts.UploadsID, counts.Uploads, ch.VideoCountText))
		ch.VideoCount = int64(counts.Uploads)
	}
}

// UploadsOptions is what ytb uploads was asked for.
type UploadsOptions struct {
	// Kind is all, videos, shorts, streams or popular. all is the uploads
	// playlist, which is the other three together.
	Kind string
	// Via is playlist or tab. playlist is the default because a derived playlist
	// id needs no channel read and no tab params; tab is what a browser does, and
	// the two occasionally disagree, which is worth being able to see.
	Via string
	// Exact cross reads the Atom feed so the newest fifteen rows get exact
	// timestamps in place of "1 month ago". One extra request.
	Exact bool
	Page  PageOptions
}

// uploadKinds maps the word a caller types to the derived playlist that holds it
// and to the tab that shows it. Doc 01 section 2.5.
//
// The empty tab entries are the honest part. There is no tab that lists a
// channel's uploads: the Videos tab is long form only and equals UULF, so
// --kind all --via tab has no answer and says so rather than quietly returning
// the long form videos as if they were everything.
var uploadKinds = map[string]struct {
	playlist func(ytid.ChannelPlaylists) string
	tab      string
}{
	"all":     {func(p ytid.ChannelPlaylists) string { return p.Uploads }, ""},
	"videos":  {func(p ytid.ChannelPlaylists) string { return p.Videos }, "videos"},
	"shorts":  {func(p ytid.ChannelPlaylists) string { return p.Shorts }, "shorts"},
	"streams": {func(p ytid.ChannelPlaylists) string { return p.Streams }, "streams"},
	"popular": {func(p ytid.ChannelPlaylists) string { return p.Popular }, ""},
}

// StreamUploads streams a channel's uploads through whichever plane was asked
// for.
func (c *Client) StreamUploads(ctx context.Context, idOrURL string, opt UploadsOptions, emit func(Video) error) error {
	emit = stampEmit(c, emit)
	kind := strings.ToLower(opt.Kind)
	if kind == "" {
		kind = "all"
	}
	spec, ok := uploadKinds[kind]
	if !ok {
		return errs.Usage("unknown kind %q: it is one of all, videos, shorts, streams, popular", kind)
	}
	via := strings.ToLower(opt.Via)
	if via == "" {
		via = "playlist"
	}

	patch := func(v *Video) {}
	if opt.Exact {
		entries, err := c.FetchChannelFeed(ctx, idOrURL)
		if err != nil {
			return fmt.Errorf("--exact: %w", err)
		}
		index := feedIndex(entries)
		patch = func(v *Video) {
			if entry, ok := index[v.VideoID]; ok {
				applyFeedExact(v, entry)
				return
			}
			v.miss("--exact: this video is not in the channel feed's newest fifteen, so its timestamp is still the relative one")
		}
	}
	emit1 := func(v Video) error {
		patch(&v)
		return emit(v)
	}

	switch via {
	case "tab":
		if spec.tab == "" {
			return errs.Usage("there is no tab behind --kind %s: the Videos tab is long form only, so --kind %s needs --via playlist", kind, kind)
		}
		return c.StreamChannelTab(ctx, idOrURL, spec.tab, opt.Page, emit1)
	case "playlist":
		channelID, err := c.ResolveChannelID(ctx, idOrURL)
		if err != nil {
			return err
		}
		ids, ok := ytid.PlaylistsFor(channelID)
		if !ok {
			return errs.NoResults("%q resolved to %q, which is not a channel id, so no uploads playlist can be derived from it", idOrURL, channelID)
		}
		return c.StreamPlaylistItems(ctx, spec.playlist(ids), opt.Page, func(_ PlaylistVideo, v Video) error {
			if v.ChannelID == "" {
				v.ChannelID = channelID
			}
			return emit1(v)
		})
	default:
		return errs.Usage("unknown --via %q: it is playlist or tab", via)
	}
}

// StreamChannelTab streams videos from a channel tab (videos, shorts, or streams).
// tab must be one of "videos", "shorts", "streams".
// If opt.Enrich is true, each video is enriched with a /player call.
// The emit function receives each Video; returning ErrStop halts iteration cleanly.
func (c *Client) StreamChannelTab(ctx context.Context, idOrURL, tab string, opt PageOptions, emit func(Video) error) error {
	emit = stampEmit(c, emit)
	tab = strings.ToLower(tab)
	if tab == "" {
		tab = "videos"
	}
	isShort := tab == "shorts"

	channelBase := NormalizeChannelURL(idOrURL)
	// Replace the trailing /videos with the requested tab.
	channelURL := strings.Replace(channelBase, "/videos", "/"+tab, 1)

	data, code, err := c.FetchPageData(ctx, channelURL)
	if err != nil {
		return fmt.Errorf("fetch channel tab %q: %w", channelURL, err)
	}
	if code == 404 || data == nil {
		return errs.NotFound("channel tab not found: %s", channelURL)
	}

	// The page arrived, which says nothing about which tab is on it. A channel with
	// no shorts answers /shorts with its featured page and a 200, and the videos on
	// that page are real videos, so this is the only place the difference is still
	// visible. The strip on the page names the tab it selected.
	if initial, ok := data.InitialData.(map[string]any); ok {
		if err := AssertTab(initial, tabSlugFor(tab)); err != nil {
			return err
		}
	}

	ch, videos, contToken, err := ParseChannelPage(data, channelURL)
	if err != nil {
		return err
	}

	it := NewInnerTube(c)

	total := 0
	pages := 0

	emit1 := func(v Video) error {
		if isShort {
			v.IsShort = boolPtr(true)
		}
		if v.ChannelID == "" {
			v.ChannelID = ch.ChannelID
		}
		if v.ChannelTitle == "" {
			v.ChannelTitle = ch.Title
		}
		// --enrich turns a listing row into a real read, one /player call per video.
		// The parser fills in place, so what the row already had survives and the
		// envelope gains the second surface rather than replacing the first.
		if opt.Enrich {
			if resp, enrichErr := it.Player(ctx, v.VideoID); enrichErr == nil && resp != nil {
				ParsePlayerResponse(resp, &v, SurfaceInnerTube)
				v.addClient("WEB")
				// The lockup's misses no longer hold, and /player has its own: it
				// carries the description as flat text with no endpoints in it.
				v.Missed = []string{}
				v.miss("enriched from /player: no description links, mentions or comment count")
			}
		}
		if err := emit(v); err != nil {
			return err
		}
		total++
		return nil
	}

	for _, v := range videos {
		if opt.Max > 0 && total >= opt.Max {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := emit1(v); err != nil {
			if err == ErrStop {
				return nil
			}
			return err
		}
	}
	pages++

	for contToken != "" {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if opt.Max > 0 && total >= opt.Max {
			return nil
		}
		if opt.MaxPages > 0 && pages >= opt.MaxPages {
			return nil
		}

		resp, err := it.BrowseContinuation(ctx, contToken)
		if err != nil {
			return fmt.Errorf("channel tab continuation: %w", err)
		}
		pageVideos, nextToken := ParseContinuationVideos(resp)
		for _, v := range pageVideos {
			if opt.Max > 0 && total >= opt.Max {
				return nil
			}
			if err := emit1(v); err != nil {
				if err == ErrStop {
					return nil
				}
				return err
			}
		}
		pages++
		contToken = nextToken
		if len(pageVideos) == 0 {
			break
		}
	}
	return nil
}

// PlaylistTabs are the channel tabs that serve a grid of playlists.
//
// They are one read with one parser and four names. releases is albums and
// singles, podcasts is shows, courses is course playlists, and all three used to
// be unreachable: the channel record listed them in its tab strip and no command
// could open one.
var PlaylistTabs = []string{"playlists", "releases", "podcasts", "courses"}

// StreamChannelPlaylists streams playlists from a channel's playlists tab.
// The emit function receives each Playlist; returning ErrStop halts iteration cleanly.
func (c *Client) StreamChannelPlaylists(ctx context.Context, idOrURL string, opt PageOptions, emit func(Playlist) error) error {
	return c.StreamChannelPlaylistTab(ctx, idOrURL, "playlists", opt, emit)
}

// StreamChannelPlaylistTab streams one of the four playlist-grid tabs.
//
// Every one of them is a page of playlist rows under a different name, so the
// tab is a parameter rather than four copies of this function. A channel that
// does not have the tab exits 3 naming the tabs it does have, which is
// AssertTab's job and matters more here than elsewhere: only two channels in ten
// have courses, and YouTube answers a missing tab with the home page and a 200.
func (c *Client) StreamChannelPlaylistTab(ctx context.Context, idOrURL, tab string, opt PageOptions, emit func(Playlist) error) error {
	emit = stampEmit(c, emit)
	tab = strings.ToLower(strings.TrimSpace(tab))
	if tab == "" {
		tab = "playlists"
	}
	if !slices.Contains(PlaylistTabs, tab) {
		return errs.Usage("%q is not a playlist tab: pick one of %s", tab, strings.Join(PlaylistTabs, ", "))
	}
	channelBase := NormalizeChannelURL(idOrURL)
	playlistsURL := strings.Replace(channelBase, "/videos", "/"+tab, 1)

	data, code, err := c.FetchPageData(ctx, playlistsURL)
	if err != nil {
		return fmt.Errorf("fetch channel %s %q: %w", tab, idOrURL, err)
	}
	if code == 404 || data == nil {
		return errs.NotFound("channel %s not found: %s", tab, idOrURL)
	}
	if initial, ok := data.InitialData.(map[string]any); ok {
		if err := AssertTab(initial, tab); err != nil {
			return err
		}
	}

	// Resolve channel ID for enrichment.
	chID := ""
	chTitle := ""
	if id, ok := data.InitialData.(map[string]any); ok {
		ch := &Channel{}
		walkJSON(id, func(m map[string]any) {
			if r, ok := m["channelMetadataRenderer"].(map[string]any); ok {
				if ch.ChannelID == "" {
					ch.ChannelID = stringValue(r["externalId"])
				}
				if ch.Title == "" {
					ch.Title = stringValue(r["title"])
				}
			}
		})
		chID = ch.ChannelID
		chTitle = ch.Title
	}

	playlists := parsePlaylistsFromTree(data.InitialData)
	contToken := extractContinuationToken(data.InitialData)

	it := NewInnerTube(c)
	total := 0
	pages := 0

	// The rows off the page came from the page and the rows after them came from a
	// browse call, and a reader who wants to check one has to be told which. Every
	// row used to say nothing at all, so a courses row and a playlists row for the
	// same channel were indistinguishable once they left the tool.
	emit1 := func(p Playlist, source string) error {
		if p.ChannelID == "" {
			p.ChannelID = chID
		}
		if p.ChannelTitle == "" {
			p.ChannelTitle = chTitle
		}
		p.addSource(source)
		if err := emit(p); err != nil {
			return err
		}
		total++
		return nil
	}

	for _, p := range playlists {
		if opt.Max > 0 && total >= opt.Max {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := emit1(p, playlistsURL); err != nil {
			if err == ErrStop {
				return nil
			}
			return err
		}
	}
	pages++

	for contToken != "" {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if opt.Max > 0 && total >= opt.Max {
			return nil
		}
		if opt.MaxPages > 0 && pages >= opt.MaxPages {
			return nil
		}
		resp, err := it.BrowseContinuation(ctx, contToken)
		if err != nil {
			return fmt.Errorf("channel %s continuation: %w", tab, err)
		}
		pagePlaylists, nextToken := ParseContinuationPlaylists(resp)
		for _, p := range pagePlaylists {
			if opt.Max > 0 && total >= opt.Max {
				return nil
			}
			if err := emit1(p, ClientWEB().Endpoint("browse")); err != nil {
				if err == ErrStop {
					return nil
				}
				return err
			}
		}
		pages++
		contToken = nextToken
		if len(pagePlaylists) == 0 {
			break
		}
	}
	return nil
}
