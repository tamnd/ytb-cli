package youtube

import (
	"context"
	"fmt"

	"github.com/tamnd/ytb-cli/pkg/ytid"
)

// playlist.go is the playlist read. Doc 05 section 2, ytb playlist and ytb items.
//
// Both commands take an id with or without VL and add the prefix on the wire,
// once, in the request builder. A bare playlist id sent as a browseId is a 400
// with a body that names no field, which is why ytid.WireID exists and why
// nothing else in this package writes "VL".

// FetchPlaylist reads a playlist's header.
func (c *Client) FetchPlaylist(ctx context.Context, ref string) (*Playlist, error) {
	resp, id, err := c.browsePlaylist(ctx, ref)
	if err != nil {
		return nil, err
	}
	p := ParsePlaylistRecord(resp, id, SurfaceInnerTube)
	if p == nil {
		return nil, fmt.Errorf("playlist %q: %w", ref, ErrPlaylistNotFound)
	}
	p.addSource(NormalizePlaylistURL(id))
	p.addClient("WEB")
	p.miss("the item list is a separate read; ytb items pages it")
	c.stamp(p)
	return p, nil
}

// browsePlaylist browses VL<id> and turns YouTube's own refusal into one.
//
// A mix answers 200 with no header and no contents at all: alerts, microformat,
// responseContext, trackingParams, and an ERROR alert reading "This playlist type
// is unviewable." Read as content that is an empty playlist, which is a claim
// that the mix has no videos in it. It has fifty. So the alert is checked before
// the body, and a mix exits 4 quoting YouTube rather than exiting 0 with nothing.
func (c *Client) browsePlaylist(ctx context.Context, ref string) (map[string]any, string, error) {
	info := ytid.Classify(ref)
	id := info.ID
	if id == "" {
		id = extractPlaylistID(ref)
	}
	if id == "" {
		return nil, "", fmt.Errorf("%q is not a playlist id or a playlist url", ref)
	}
	id = ytid.StripWire(id)
	resp, err := NewInnerTube(c).Browse(ctx, ytid.WireID(id), "", "")
	if err != nil {
		// A playlist id with the right shape and nothing behind it answers 404
		// with "Requested entity was not found.", which is the deleted playlist
		// and the invented one both. That is a subject that does not exist and
		// not a read that broke, so it is reported as one.
		if httpStatus(err) == 404 {
			return nil, id, fmt.Errorf("playlist %q: %w", ref, ErrPlaylistNotFound)
		}
		return nil, id, fmt.Errorf("browse playlist %q: %w", ref, err)
	}
	if r := alertRefusal(resp, id, SurfaceInnerTube); r != nil {
		return nil, id, r
	}
	if r := messageRefusal(resp, id, SurfaceInnerTube); r != nil {
		return nil, id, r
	}
	return resp, id, nil
}

// StreamPlaylistItems streams a playlist's videos in playlist order.
//
// The header's count and the number of items yielded do not have to agree, and
// when they do not the reason is YouTube's own alert, quoted. One playlist here
// states 128 videos in its header, serves 64 lockups, and says
// "64 unavailable videos are hidden" in an INFO alert. That sentence is the whole
// explanation and it is put on the record rather than thrown away.
//
// Returning ErrStop from emit halts iteration cleanly.
func (c *Client) StreamPlaylistItems(ctx context.Context, ref string, opt PageOptions, emit func(PlaylistVideo, Video) error) error {
	_, err := c.streamPlaylist(ctx, ref, opt, emit)
	return err
}

// StreamPlaylistWithHeader is StreamPlaylistItems with the header returned, for
// the caller that wants to report what the playlist claimed alongside what it
// yielded.
func (c *Client) StreamPlaylistWithHeader(ctx context.Context, ref string, opt PageOptions, emit func(PlaylistVideo, Video) error) (*Playlist, error) {
	return c.streamPlaylist(ctx, ref, opt, emit)
}

func (c *Client) streamPlaylist(ctx context.Context, ref string, opt PageOptions, emit func(PlaylistVideo, Video) error) (*Playlist, error) {
	// The row is the record here and the edge beside it is a pair of ids, so the
	// stamp goes on the video and the header below.
	if !c.session.Empty() {
		row := emit
		emit = func(e PlaylistVideo, v Video) error {
			c.stampEnvelope(&v.Envelope)
			return row(e, v)
		}
	}
	resp, id, err := c.browsePlaylist(ctx, ref)
	if err != nil {
		return nil, err
	}
	p := ParsePlaylistRecord(resp, id, SurfaceInnerTube)
	if p == nil {
		p = ptr(newPlaylist(id, SurfaceInnerTube))
		p.miss("this response carried neither header shape, so there is no title and no stated count")
	}
	p.addSource(NormalizePlaylistURL(id))
	p.addClient("WEB")
	c.stamp(p)

	// The alerts on the first page are why the list about to be streamed is
	// shorter than the header's count, so every item carries them. A consumer
	// holding one row out of this stream can then tell that the list it came from
	// was incomplete, which a note on a header record it never saw would not do.
	//
	// The sentence is quoted and never matched: it is localized, and on one
	// playlist here it reads "64 unavailable videos are hidden" with the count in
	// it while on another it states no number at all.
	notes := alertWarnings(resp)

	// A row on a channel's own uploads names no owner, because the page is the
	// owner and the site does not repeat it on every line. A UU-family id is the
	// channel id with the prefix swapped, so filling the owner in here is
	// arithmetic on the id and not a guess about who uploaded what. It is only
	// done for that family: a PL playlist holds other people's videos and the
	// owner of the list is not the owner of the row.
	owner, ownerIsChannel := ytid.ChannelFor(id)

	it := NewInnerTube(c)
	total := 0
	pages := 0
	stopped := false

	for {
		if ctx.Err() != nil {
			return p, ctx.Err()
		}
		items, next := ParsePlaylistItems(resp, total)
		for _, v := range items {
			if opt.Max > 0 && total >= opt.Max {
				stopped = true
				break
			}
			v.addClient("WEB")
			v.addSource(NormalizePlaylistURL(id))
			if v.ChannelID == "" && ownerIsChannel {
				v.ChannelID = owner
				v.setVia("channel_id", "s2 the uploads playlist id, which is the channel id with the prefix swapped")
				if v.ChannelTitle == "" {
					v.ChannelTitle = p.ChannelTitle
				}
			}
			for _, n := range notes {
				v.miss("this playlist's own alert says: %s", n)
			}
			edge := PlaylistVideo{PlaylistID: id, VideoID: v.VideoID, Position: v.Position}
			if err := emit(edge, v); err != nil {
				if err == ErrStop {
					stopped = true
					break
				}
				return p, err
			}
			total++
		}
		pages++
		if stopped || next == "" || len(items) == 0 {
			break
		}
		if opt.MaxPages > 0 && pages >= opt.MaxPages {
			p.miss("stopped after %d pages; the playlist has more", pages)
			break
		}
		resp, err = it.BrowseContinuation(ctx, next)
		if err != nil {
			return p, fmt.Errorf("playlist continuation: %w", err)
		}
	}

	// The count and the yield disagreeing is normal and is worth saying out loud,
	// once, with whatever reason YouTube gave for it.
	if !stopped && p.VideoCount > 0 && int64(total) != p.VideoCount {
		p.miss("the header says %d videos and this read yielded %d", p.VideoCount, total)
	}
	return p, nil
}

// ptr is the one-liner for taking the address of a value returned by a
// constructor, which Go has no syntax for.
func ptr[T any](v T) *T { return &v }
