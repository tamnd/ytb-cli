package youtube

import (
	"context"
	"fmt"
)

// StreamHashtag streams videos tagged with a YouTube hashtag.
// tag may include or omit the leading "#".
// Returning ErrStop from emit halts iteration cleanly.
func (c *Client) StreamHashtag(ctx context.Context, tag string, opt PageOptions, emit func(Video) error) error {
	it := NewInnerTube(c)

	browseID, params, err := it.ResolveHashtag(ctx, tag)
	if err != nil {
		return fmt.Errorf("resolve hashtag %q: %w", tag, err)
	}

	total := 0
	pages := 0
	contToken := ""

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if opt.Max > 0 && total >= opt.Max {
			return nil
		}
		if opt.MaxPages > 0 && pages >= opt.MaxPages {
			return nil
		}

		var data map[string]any
		if contToken == "" {
			data, err = it.Browse(ctx, browseID, params, "")
		} else {
			data, err = it.BrowseContinuation(ctx, contToken)
		}
		if err != nil {
			return fmt.Errorf("hashtag page %d: %w", pages+1, err)
		}

		videos, nextToken := parseHashtagPage(data)
		for _, v := range videos {
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
		if contToken == "" || len(videos) == 0 {
			break
		}
	}
	return nil
}
