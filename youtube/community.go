package youtube

import (
	"context"
	"fmt"
)

// StreamCommunity streams community posts for a channel.
// channel may be a channel ID (UC...), handle (@name), vanity name, or URL.
// Returning ErrStop from emit halts iteration cleanly.
func (c *Client) StreamCommunity(ctx context.Context, channel string, opt PageOptions, emit func(CommunityPost) error) error {
	it := NewInnerTube(c)

	// Resolve to a UC-style channel ID (the Browse API requires it).
	channelID, err := c.ResolveChannelID(ctx, channel)
	if err != nil {
		return fmt.Errorf("community resolve channel: %w", err)
	}

	// Discover the community/posts tab params dynamically.
	tabParams, err := it.DiscoverCommunityTabParams(ctx, channelID)
	if err != nil {
		return fmt.Errorf("community discover tab params: %w", err)
	}
	if tabParams == "" {
		return nil // channel has no community tab
	}

	// Fetch first page.
	resp, err := it.Community(ctx, channelID, tabParams, "")
	if err != nil {
		return fmt.Errorf("community first page: %w", err)
	}

	total := 0
	pages := 0

	processCommunityPage := func(resp map[string]any) (int, string) {
		var count int
		var nextToken string
		walkJSON(resp, func(m map[string]any) {
			if p := ParseCommunityPost(m, channelID); p != nil {
				if opt.Max > 0 && total+count >= opt.Max {
					return
				}
				if emitErr := emit(*p); emitErr != nil {
					return
				}
				count++
			}
			if cir, ok := m["continuationItemRenderer"].(map[string]any); ok {
				if ep := mapValue(cir, "continuationEndpoint"); ep != nil {
					if cmd := mapValue(ep, "continuationCommand"); cmd != nil {
						if tok := stringValue(cmd["token"]); tok != "" && nextToken == "" {
							nextToken = tok
						}
					}
				}
			}
		})
		return count, nextToken
	}

	batchCount, contToken := processCommunityPage(resp)
	total += batchCount
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

		resp, err = it.Community(ctx, channelID, tabParams, contToken)
		if err != nil {
			return fmt.Errorf("community page %d: %w", pages+1, err)
		}
		batchCount, contToken = processCommunityPage(resp)
		total += batchCount
		pages++
		if batchCount == 0 {
			break
		}
	}
	return nil
}
