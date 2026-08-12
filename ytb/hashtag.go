package ytb

import (
	"context"
	"fmt"
	"strings"
)

// StreamHashtag streams videos tagged with a YouTube hashtag.
// tag may include or omit the leading "#".
// Returning ErrStop from emit halts iteration cleanly.
func (c *Client) StreamHashtag(ctx context.Context, tag string, opt PageOptions, emit func(Video) error) error {
	_, err := c.StreamHashtagWithHeader(ctx, tag, opt, emit)
	return err
}

// StreamHashtagWithHeader is the same read and also hands back the feed's own
// record: what YouTube calls the tag and how many videos it says carry it.
//
// It is a second entry point rather than an extra return on the first because
// most callers want the videos, and the header is one object at the top of the
// first page that a continuation never repeats.
func (c *Client) StreamHashtagWithHeader(ctx context.Context, tag string, opt PageOptions, emit func(Video) error) (*HashtagRecord, error) {
	// The feed page is where these rows were read, and a row that named no source
	// was the one kind in the tool you could not trace back to a URL.
	source := BaseURL + "/hashtag/" + strings.ToLower(strings.TrimPrefix(tag, "#"))
	inner := stampEmit(c, emit)
	emit = func(v Video) error {
		v.addSource(source)
		return inner(v)
	}
	it := NewInnerTube(c)

	browseID, params, err := it.ResolveHashtag(ctx, tag)
	if err != nil {
		return nil, fmt.Errorf("resolve hashtag %q: %w", tag, err)
	}

	total := 0
	pages := 0
	contToken := ""
	var header *HashtagRecord

	for {
		if ctx.Err() != nil {
			return header, ctx.Err()
		}
		if opt.Max > 0 && total >= opt.Max {
			return header, nil
		}
		if opt.MaxPages > 0 && pages >= opt.MaxPages {
			return header, nil
		}

		var data map[string]any
		if contToken == "" {
			data, err = it.Browse(ctx, browseID, params, "")
		} else {
			data, err = it.BrowseContinuation(ctx, contToken)
		}
		if err != nil {
			return header, fmt.Errorf("hashtag page %d: %w", pages+1, err)
		}
		if header == nil {
			header = ParseHashtagHeader(data, tag)
			if header != nil {
				c.stamp(header)
			}
		}

		videos, nextToken := parseHashtagPage(data)
		for _, v := range videos {
			if opt.Max > 0 && total >= opt.Max {
				return header, nil
			}
			if err := emit(v); err != nil {
				if err == ErrStop {
					return header, nil
				}
				return header, err
			}
			total++
		}
		pages++
		contToken = nextToken
		if contToken == "" || len(videos) == 0 {
			break
		}
	}
	return header, nil
}

// ParseHashtagHeader reads the feed's own record off a hashtag browse response.
// Doc 03 section 11.
//
// The header is a pageHeaderViewModel, which is the newer shape and carries the
// counts as one rendered line: "6.7M videos • 1.3M channels". Both halves are
// kept as text and neither is parsed into a number, because 6.7M is a rounding of
// something between 6.65 and 6.75 million and undoing it would invent five digits.
func ParseHashtagHeader(data map[string]any, tag string) *HashtagRecord {
	name := strings.TrimPrefix(strings.TrimSpace(tag), "#")
	if name == "" {
		return nil
	}
	rec := &HashtagRecord{
		Tag:      name,
		URL:      "https://www.youtube.com/hashtag/" + name,
		Envelope: newEnvelope("hashtag", SurfaceInnerTube),
	}
	rec.addClient("WEB")

	header := mapValue(mapValue(data, "header"), "pageHeaderRenderer")
	view := mapValue(mapValue(header, "content"), "pageHeaderViewModel")
	if view == nil {
		// The older shape put the title straight on the renderer, and a
		// continuation carries no header at all.
		rec.Title = stringValue(header["pageTitle"])
		if rec.Title == "" {
			return nil
		}
		rec.miss("the header carried no counts, only the title")
		return rec
	}

	rec.Title = extractText(mapValue(mapValue(view, "title"), "dynamicTextViewModel")["text"])
	if rec.Title == "" {
		rec.Title = stringValue(header["pageTitle"])
	}

	// The counts are one line with a bullet in the middle, and the two halves are
	// taken by position rather than by looking for the word "videos". Under --hl vi
	// that line reads "6,7Tr video • 1,3Tr kênh", so a keyword match would find
	// nothing and quietly report a hashtag with no counts. Doc 01 section 1.3.
	rows := mapValue(mapValue(view, "metadata"), "contentMetadataViewModel")
	for _, row := range arrayValue(rows["metadataRows"]) {
		for _, part := range arrayValue(mapValue(row, "")["metadataParts"]) {
			fields := strings.Split(extractText(mapValue(part, "")["text"]), "•")
			if len(fields) > 0 && rec.VideoCountText == "" {
				rec.VideoCountText = strings.TrimSpace(fields[0])
			}
			if len(fields) > 1 && rec.ChannelCountText == "" {
				rec.ChannelCountText = strings.TrimSpace(fields[1])
			}
		}
	}
	if rec.VideoCountText == "" {
		rec.miss("the header stated no video count")
	}
	return rec
}
