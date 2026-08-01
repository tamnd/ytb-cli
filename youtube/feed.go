package youtube

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"
	"time"
)

// feed.go reads a channel's Atom feed, surface s6.
//
// It is the smallest surface in the tool and the only one that answers with
// exact times. A listing row says "1 month ago" and the feed says
// 2026-07-27T14:30:23+00:00, to the second, with the uploader's offset on it.
// The view count is exact too, where a lockup rounds it to "22K views".
//
// What it costs is coverage: fifteen entries and no paging, ever. So it is not a
// way to read a channel, it is a way to correct the newest fifteen rows of one,
// which is what ytb uploads --exact does.
//
// Two oddities of the format are worth knowing. The feed level <id> and
// <yt:channelId> have the UC stripped off, so they read
// yt:channel:uAXFkgsw1L7xaCfnd5JJOw for a channel whose id is
// UCuAXFkgsw1L7xaCfnd5JJOw; the per entry yt:channelId is the full id and is what
// this reads. And the alternate link on an entry is the only place outside the
// player that says a video is a short: it is /shorts/<id> rather than /watch?v=.

// atomFeed is the Atom document, named down to the fields that carry facts. The
// rest of it is the same channel url in four spellings.
type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Title   string      `xml:"title"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	VideoID   string `xml:"videoId"`
	ChannelID string `xml:"channelId"`
	Title     string `xml:"title"`
	Link      struct {
		Href string `xml:"href,attr"`
	} `xml:"link"`
	AuthorName string `xml:"author>name"`
	AuthorURI  string `xml:"author>uri"`
	Published  string `xml:"published"`
	Updated    string `xml:"updated"`
	Group      struct {
		Description string `xml:"description"`
		Thumbnail   struct {
			URL    string `xml:"url,attr"`
			Width  int    `xml:"width,attr"`
			Height int    `xml:"height,attr"`
		} `xml:"thumbnail"`
		Community struct {
			StarRating struct {
				Count   int64  `xml:"count,attr"`
				Average string `xml:"average,attr"`
			} `xml:"starRating"`
			Statistics struct {
				Views int64 `xml:"views,attr"`
			} `xml:"statistics"`
		} `xml:"community"`
	} `xml:"group"`
}

// FetchChannelFeed reads the fifteen newest uploads from the channel's Atom feed.
func (c *Client) FetchChannelFeed(ctx context.Context, idOrURL string) ([]Video, error) {
	channelID, err := c.ResolveChannelID(ctx, idOrURL)
	if err != nil {
		return nil, err
	}
	feedURL := ChannelFeedURL(channelID)
	body, code, err := c.Fetch(ctx, feedURL)
	if err != nil {
		return nil, fmt.Errorf("channel feed %s: %w", channelID, err)
	}
	if code == 404 {
		return nil, fmt.Errorf("channel feed %s: %w", channelID, ErrChannelNotFound)
	}
	return ParseChannelFeed(body, feedURL)
}

// ParseChannelFeed turns the Atom document into video records.
func ParseChannelFeed(body []byte, feedURL string) ([]Video, error) {
	var feed atomFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("parse channel feed: %w", err)
	}
	out := make([]Video, 0, len(feed.Entries))
	for _, e := range feed.Entries {
		if e.VideoID == "" {
			continue
		}
		v := Video{
			VideoID:      e.VideoID,
			URL:          NormalizeVideoURL(e.VideoID),
			ShortURL:     "https://youtu.be/" + e.VideoID,
			Title:        e.Title,
			Description:  e.Group.Description,
			ChannelID:    e.ChannelID,
			ChannelTitle: e.AuthorName,
			ChannelURL:   e.AuthorURI,
			ViewCount:    e.Group.Community.Statistics.Views,
			PublishedAt:  atomTime(e.Published),
			UpdatedAt:    atomTime(e.Updated),
			ThumbnailURL: e.Group.Thumbnail.URL,
			Envelope:     newEnvelope("video", SurfaceFeed),
		}
		// The alternate link is the only thing in the feed that separates a short
		// from a video, and it says so for every entry, so this is a real answer
		// rather than an absence.
		v.IsShort = boolPtr(strings.Contains(e.Link.Href, "/shorts/"))
		if e.Group.Thumbnail.URL != "" {
			v.Thumbnails = []Thumbnail{{
				URL:    e.Group.Thumbnail.URL,
				Width:  e.Group.Thumbnail.Width,
				Height: e.Group.Thumbnail.Height,
				Name:   renditionName(e.Group.Thumbnail.URL),
				Source: ThumbnailFromPayload,
			}}
		}
		v.addSource(feedURL)
		v.setVia("published_at", "s6 <published>, exact to the second")
		v.setVia("view_count", "s6 media:statistics, exact")
		// The feed also carries media:starRating count, which is the number of
		// ratings and not the like count. Since dislikes went away the two are
		// nearly the same number and nearly is not the same as the same, so it is
		// not written into like_count.
		v.miss("read from the channel feed: the newest fifteen uploads and nothing older, with no duration, no keywords and no like count")
		out = append(out, v)
	}
	return out, nil
}

// atomTime parses an Atom timestamp, which is RFC 3339 with the uploader's own
// offset on it rather than a Z time.
func atomTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// feedIndex maps video id to feed entry, for the cross read --exact does.
func feedIndex(videos []Video) map[string]Video {
	index := make(map[string]Video, len(videos))
	for _, v := range videos {
		index[v.VideoID] = v
	}
	return index
}

// applyFeedExact copies the feed's exact numbers onto a listing row, and says on
// the record that it did.
//
// It overwrites rather than filling gaps: a listing row already carries a rounded
// view count and a relative "1 month ago", and the whole point of the cross read
// is that the feed's numbers are better. What it does not do is invent the fields
// the feed has no answer for, so a row that came from a lockup keeps its duration
// and its rounded view_count_text.
func applyFeedExact(v *Video, entry Video) {
	v.PublishedAt = entry.PublishedAt
	v.UpdatedAt = entry.UpdatedAt
	if entry.ViewCount > 0 {
		v.ViewCount = entry.ViewCount
	}
	v.addSurface(SurfaceFeed)
	for _, src := range entry.Sources {
		v.addSource(src)
	}
	v.setVia("published_at", "s6 <published>, exact to the second")
	v.setVia("updated_at", "s6 <updated>")
	if entry.ViewCount > 0 {
		v.setVia("view_count", "s6 media:statistics, exact")
	}
}
