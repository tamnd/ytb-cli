package youtube

import (
	"context"
	"fmt"
	"net/url"
)

// Search performs a YouTube search and streams results to emit.
// Each call to emit receives a Video, Channel, or Playlist value.
// Returning ErrStop from emit halts iteration cleanly.
func (c *Client) Search(ctx context.Context, query string, f SearchFilters, opt PageOptions, emit func(any) error) error {
	searchURL := BaseURL + "/results?search_query=" + url.QueryEscape(query)
	if sp := f.Encode(); sp != "" {
		searchURL += "&sp=" + url.QueryEscape(sp)
	}

	data, _, err := c.FetchPageData(ctx, searchURL)
	if err != nil {
		return fmt.Errorf("search %q: %w", query, err)
	}

	results, videos, channels, playlists, contToken, err := ParseSearchPage(data, query)
	_ = results
	if err != nil {
		return err
	}

	it := NewInnerTube(c)
	total := 0
	pages := 0

	emitItems := func(vs []Video, cs []Channel, ps []Playlist) error {
		for _, v := range vs {
			if opt.Max > 0 && total >= opt.Max {
				return ErrStop
			}
			if err := emit(v); err != nil {
				return err
			}
			total++
		}
		for _, ch := range cs {
			if opt.Max > 0 && total >= opt.Max {
				return ErrStop
			}
			if err := emit(ch); err != nil {
				return err
			}
			total++
		}
		for _, p := range ps {
			if opt.Max > 0 && total >= opt.Max {
				return ErrStop
			}
			if err := emit(p); err != nil {
				return err
			}
			total++
		}
		return nil
	}

	if err := emitItems(videos, channels, playlists); err != nil {
		if err == ErrStop {
			return nil
		}
		return err
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

		resp, err := it.Search(ctx, query, f, contToken)
		if err != nil {
			return fmt.Errorf("search continuation: %w", err)
		}
		pv, pc, pp, nextToken := ParseInnerTubeSearchResults(resp)
		if err := emitItems(pv, pc, pp); err != nil {
			if err == ErrStop {
				return nil
			}
			return err
		}
		pages++
		contToken = nextToken
		if len(pv)+len(pc)+len(pp) == 0 {
			break
		}
	}
	return nil
}

// Trending streams trending/popular videos for the given category.
// category may be "music", "gaming", "news", "movies", or "" for general trending.
// Returning ErrStop from emit halts iteration cleanly.
func (c *Client) Trending(ctx context.Context, category string, opt PageOptions, emit func(Video) error) error {
	query := trendingQuery(category)
	filters := SearchFilters{
		Sort:       "views",
		Type:       "video",
		UploadDate: "today",
	}
	sp := filters.Encode()
	searchURL := BaseURL + "/results?search_query=" + url.QueryEscape(query)
	if sp != "" {
		searchURL += "&sp=" + url.QueryEscape(sp)
	}

	data, _, err := c.FetchPageData(ctx, searchURL)
	if err != nil {
		return fmt.Errorf("trending fetch: %w", err)
	}

	_, videos, _, _, contToken, _ := ParseSearchPage(data, query)
	videos = dedupeVideos(videos)

	it := NewInnerTube(c)
	total := 0
	pages := 0

	for _, v := range videos {
		if opt.Max > 0 && total >= opt.Max {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := emit(v); err != nil {
			if err == ErrStop {
				return nil
			}
			return err
		}
		total++
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

		resp, err := it.Search(ctx, query, filters, contToken)
		if err != nil {
			return fmt.Errorf("trending continuation: %w", err)
		}
		pageVideos, _, _, nextToken := ParseInnerTubeSearchResults(resp)
		pageVideos = dedupeVideos(pageVideos)
		for _, v := range pageVideos {
			if opt.Max > 0 && total >= opt.Max {
				return nil
			}
			if err := emit(v); err != nil {
				if err == ErrStop {
					return nil
				}
				return err
			}
			total++
		}
		pages++
		contToken = nextToken
		if len(pageVideos) == 0 {
			break
		}
	}
	return nil
}

// Suggest returns autocomplete suggestions for query from YouTube's suggestion endpoint.
func (c *Client) Suggest(ctx context.Context, query string) ([]string, error) {
	it := NewInnerTube(c)
	return it.Suggest(ctx, query)
}
