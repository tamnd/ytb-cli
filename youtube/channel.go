package youtube

import (
	"context"
	"fmt"
	"strings"
)

// FetchChannel fetches a channel's header metadata.
func (c *Client) FetchChannel(ctx context.Context, idOrURL string) (*Channel, error) {
	channelURL := NormalizeChannelURL(idOrURL)
	data, code, err := c.FetchPageData(ctx, channelURL)
	if err != nil {
		return nil, fmt.Errorf("fetch channel %q: %w", idOrURL, err)
	}
	if code == 404 || data == nil {
		return nil, fmt.Errorf("channel not found: %s", idOrURL)
	}
	ch, _, _, err := ParseChannelPage(data, channelURL)
	if err != nil {
		return nil, err
	}
	if id, ok := data.InitialData.(map[string]any); ok {
		ParseChannelNumericCounts(id, ch)
	}
	return ch, nil
}

// StreamChannelTab streams videos from a channel tab (videos, shorts, or streams).
// tab must be one of "videos", "shorts", "streams".
// If opt.Enrich is true, each video is enriched with a /player call.
// The emit function receives each Video; returning ErrStop halts iteration cleanly.
func (c *Client) StreamChannelTab(ctx context.Context, idOrURL, tab string, opt PageOptions, emit func(Video) error) error {
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
		return fmt.Errorf("channel tab not found: %s", channelURL)
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
			v.IsShort = true
		}
		if v.ChannelID == "" {
			v.ChannelID = ch.ChannelID
		}
		if v.ChannelName == "" {
			v.ChannelName = ch.Title
		}
		if opt.Enrich {
			resp, enrichErr := it.Player(ctx, v.VideoID)
			if enrichErr == nil && resp != nil {
				details := ParsePlayerDetails(resp, v.VideoID)
				if details != nil {
					if v.Description == "" {
						v.Description = details.Description
					}
					if v.DurationSeconds == 0 {
						v.DurationSeconds = details.DurationSeconds
					}
					if v.ViewCount == 0 {
						v.ViewCount = details.ViewCount
					}
					if v.Category == "" {
						v.Category = details.Category
					}
					if v.UploadDate == "" {
						v.UploadDate = details.UploadDate
					}
					if v.PublishedAt.IsZero() {
						v.PublishedAt = details.PublishedAt
					}
				}
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

// StreamChannelPlaylists streams playlists from a channel's playlists tab.
// The emit function receives each Playlist; returning ErrStop halts iteration cleanly.
func (c *Client) StreamChannelPlaylists(ctx context.Context, idOrURL string, opt PageOptions, emit func(Playlist) error) error {
	channelBase := NormalizeChannelURL(idOrURL)
	playlistsURL := strings.Replace(channelBase, "/videos", "/playlists", 1)

	data, code, err := c.FetchPageData(ctx, playlistsURL)
	if err != nil {
		return fmt.Errorf("fetch channel playlists %q: %w", idOrURL, err)
	}
	if code == 404 || data == nil {
		return fmt.Errorf("channel playlists not found: %s", idOrURL)
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

	emit1 := func(p Playlist) error {
		if p.ChannelID == "" {
			p.ChannelID = chID
		}
		if p.ChannelName == "" {
			p.ChannelName = chTitle
		}
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
		if err := emit1(p); err != nil {
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
			return fmt.Errorf("channel playlists continuation: %w", err)
		}
		pagePlaylists, nextToken := ParseContinuationPlaylists(resp)
		for _, p := range pagePlaylists {
			if opt.Max > 0 && total >= opt.Max {
				return nil
			}
			if err := emit1(p); err != nil {
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
