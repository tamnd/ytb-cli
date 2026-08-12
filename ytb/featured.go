package ytb

import (
	"context"
	"fmt"
	"strings"
)

// featured.go reads the featured tab's channel shelves.
//
// It exists for one predicate. A channel's home page can carry a shelf of other
// channels, "Brady Haran's other channels" on @Computerphile is seven of them,
// and that shelf is the only place YouTube states a channel-to-channel link at
// tier 0. Nothing else in this tool reads it, so features would be a predicate
// in the table with no way to produce one.
//
// The shelf is a gridChannelRenderer, which is the same row a search result
// gives a channel, so parseChannelRenderer already knows how to read it.

// FeaturedChannels reads a channel's home tab and returns the channel itself
// and the channels its shelves feature, in page order.
//
// The channel comes back too because the claim needs both ends and the caller
// usually passed a handle, which is not an identity and cannot be a key.
func (c *Client) FeaturedChannels(ctx context.Context, idOrURL string) (*Channel, []Channel, error) {
	pageURL := strings.Replace(NormalizeChannelURL(idOrURL), "/videos", "/featured", 1)

	data, code, err := c.FetchPageData(ctx, pageURL)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch featured tab %q: %w", pageURL, err)
	}
	if code == 404 || data == nil {
		return nil, nil, fmt.Errorf("channel not found: %s", idOrURL)
	}

	ch, _, _, err := ParseChannelPage(data, pageURL)
	if err != nil {
		return nil, nil, err
	}

	var featured []Channel
	seen := map[string]bool{}
	walkJSON(data.InitialData, func(m map[string]any) {
		for _, key := range []string{"gridChannelRenderer", "channelRenderer"} {
			r, ok := m[key].(map[string]any)
			if !ok {
				continue
			}
			got := parseChannelRenderer(r)
			// The owner appears in its own header, and a shelf can list the same
			// channel twice when two shelves overlap.
			if got.ChannelID == "" || got.ChannelID == ch.ChannelID || seen[got.ChannelID] {
				continue
			}
			seen[got.ChannelID] = true
			got.addSource(pageURL)
			featured = append(featured, got)
		}
	})
	c.stamp(ch)
	stampAll(c, featured)
	return ch, featured, nil
}
