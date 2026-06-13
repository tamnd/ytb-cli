package youtube

import (
	"context"
	"fmt"
)

// FetchPlaylist fetches a playlist's header metadata.
func (c *Client) FetchPlaylist(ctx context.Context, idOrURL string) (*Playlist, error) {
	playlistURL := NormalizePlaylistURL(idOrURL)
	data, code, err := c.FetchPageData(ctx, playlistURL)
	if err != nil {
		return nil, fmt.Errorf("fetch playlist %q: %w", idOrURL, err)
	}
	if code == 404 || data == nil {
		return nil, fmt.Errorf("playlist not found: %s", idOrURL)
	}
	playlist, _, _, _, err := ParsePlaylistPage(data, playlistURL)
	if err != nil {
		return nil, err
	}
	return playlist, nil
}

// StreamPlaylistItems streams items from a playlist.
// emit receives each (PlaylistVideo, Video) pair in playlist order.
// Returning ErrStop from emit halts iteration cleanly.
func (c *Client) StreamPlaylistItems(ctx context.Context, idOrURL string, opt PageOptions, emit func(PlaylistVideo, Video) error) error {
	playlistURL := NormalizePlaylistURL(idOrURL)
	data, code, err := c.FetchPageData(ctx, playlistURL)
	if err != nil {
		return fmt.Errorf("fetch playlist %q: %w", idOrURL, err)
	}
	if code == 404 || data == nil {
		return fmt.Errorf("playlist not found: %s", idOrURL)
	}

	playlist, edges, videos, contToken, err := ParsePlaylistPage(data, playlistURL)
	if err != nil {
		return err
	}

	it := NewInnerTube(c)
	total := 0
	pages := 0

	emitPair := func(edge PlaylistVideo, v Video) error {
		if err := emit(edge, v); err != nil {
			return err
		}
		total++
		return nil
	}

	// Build a map from videoID to Video for the first page.
	videoMap := make(map[string]Video, len(videos))
	for _, v := range videos {
		videoMap[v.VideoID] = v
	}

	for _, edge := range edges {
		if opt.Max > 0 && total >= opt.Max {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		v := videoMap[edge.VideoID]
		if err := emitPair(edge, v); err != nil {
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
			return fmt.Errorf("playlist continuation: %w", err)
		}
		pageVideos, pageEdges, nextToken := ParseContinuationPlaylistVideos(resp, playlist.PlaylistID)
		pageMap := make(map[string]Video, len(pageVideos))
		for _, v := range pageVideos {
			pageMap[v.VideoID] = v
		}
		for _, edge := range pageEdges {
			if opt.Max > 0 && total >= opt.Max {
				return nil
			}
			v := pageMap[edge.VideoID]
			if err := emitPair(edge, v); err != nil {
				if err == ErrStop {
					return nil
				}
				return err
			}
		}
		pages++
		contToken = nextToken
		if len(pageEdges) == 0 {
			break
		}
	}
	return nil
}
