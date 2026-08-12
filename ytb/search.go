package ytb

import (
	"context"
	"fmt"
	"net/url"

	"github.com/tamnd/any-cli/kit/errs"
)

// Search runs a search and streams the rows to emit in the order the site
// ranked them. Each call to emit receives a Video, Channel or Playlist.
//
// The rows are emitted mixed rather than grouped by type. A search for a
// channel's name answers with that channel's card first and its videos after,
// and reordering that into "all videos, then all channels" throws away the one
// thing a search knows that a listing does not.
//
// Returning ErrStop from emit halts iteration cleanly.
func (c *Client) Search(ctx context.Context, query string, f SearchFilters, opt PageOptions, emit func(any) error) error {
	emit = c.stampEmitAny(emit)
	it := NewInnerTube(c)
	resp, err := it.Search(ctx, query, f, "")
	if err != nil {
		return fmt.Errorf("search %q: %w", query, err)
	}

	total := 0
	pages := 0
	empties := 0
	seen := map[string]struct{}{}
	token := ""

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		items, next := ParseSearchResults(resp)
		if pages == 0 && len(items) == 0 {
			return errs.NoResults("no search results found for %q", query)
		}
		for _, item := range items {
			key := searchKey(item)
			if key == "" {
				continue
			}
			// A row repeating across pages is normal: a shelf on page 1 draws the
			// same video the ranked list draws on page 2, and the site is happy to
			// show it twice.
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			if opt.Max > 0 && total >= opt.Max {
				return nil
			}
			if err := emit(searchWithClient(item)); err != nil {
				if err == ErrStop {
					return nil
				}
				return err
			}
			total++
		}
		pages++
		token = next
		if token == "" {
			return nil
		}
		// A page that carries a token and no rows is a thing YouTube does, and it
		// is not the end of the results: the page after it has twenty. So an empty
		// page is followed once more and only two in a row are read as the end.
		if len(items) == 0 {
			empties++
			if empties >= 2 {
				return nil
			}
		} else {
			empties = 0
		}
		if opt.MaxPages > 0 && pages >= opt.MaxPages {
			return nil
		}
		resp, err = it.Search(ctx, query, f, token)
		if err != nil {
			return fmt.Errorf("search continuation: %w", err)
		}
	}
}

// searchWithClient stamps the client that answered onto a row, which the parsers
// cannot do because they do not know which client was asked.
func searchWithClient(item any) any {
	switch v := item.(type) {
	case Video:
		v.addClient("WEB")
		return v
	case Channel:
		v.addClient("WEB")
		return v
	case Playlist:
		v.addClient("WEB")
		return v
	}
	return item
}

// searchKey identifies a row for the duplicate check. The kind is part of it
// because a channel and a video can hold the same id string in principle and
// there is no reason to make that a collision.
func searchKey(item any) string {
	switch v := item.(type) {
	case Video:
		return "video:" + v.VideoID
	case Channel:
		return "channel:" + v.ChannelID
	case Playlist:
		return "playlist:" + v.PlaylistID
	}
	return ""
}

// Trending streams trending/popular videos for the given category.
// category may be "music", "gaming", "news", "movies", or "" for general trending.
// Returning ErrStop from emit halts iteration cleanly.
func (c *Client) Trending(ctx context.Context, category string, opt PageOptions, emit func(Video) error) error {
	emit = stampEmit(c, emit)
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
