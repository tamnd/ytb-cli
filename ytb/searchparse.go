package ytb

import (
	"strings"
)

// searchparse.go reads a search response. Doc 05 section 3, ytb search.
//
// A search is the one read where order is data. The site ranked these rows and
// the rank is most of what the user asked for, so this file walks the response
// structurally, section by section and item by item, and returns one mixed slice
// in draw order. It does not use walkJSON: a map walk in Go visits keys in
// whatever order the runtime feels like, which turns rank 1 into rank 14 for no
// reason the caller can see.
//
// The shapes a search serves, counted on three live responses:
//
//	plain "rick astley"       officialCardViewModel 1, videoRenderer 19, shelfRenderer 1, gridShelfViewModel 3
//	--type channel            channelRenderer 20
//	--type playlist           lockupViewModel 19, channelRenderer 1
//
// Two things fall out of that. A plain search serves no channelRenderer at all
// any more, so the top channel arrives as an officialCardViewModel and a reader
// that only knows channelRenderer drops the single most relevant row on the
// page. And a shelf is not a row: shelfRenderer and gridShelfViewModel are boxes
// holding rows, so they are opened and their contents take the shelf's place in
// the order.

// ParseSearchResults walks a search response in the order the page draws it and
// returns the rows found, as Video, Channel and Playlist values, with the token
// for the next page.
//
// The first page and a continuation are the same items under two different
// roots, so both roots are tried and the item walk is shared.
func ParseSearchResults(root map[string]any) ([]any, string) {
	var out []any
	sections := searchSections(root)
	for _, entry := range sections {
		out = append(out, searchItems(entry)...)
	}
	return out, extractContinuationToken(root)
}

// searchSections returns the section entries of a search response, whichever
// root they arrived under.
func searchSections(root map[string]any) []map[string]any {
	primary := mapValue(mapValue(mapValue(root, "contents"), "twoColumnSearchResultsRenderer"), "primaryContents")
	if list := listOfMaps(mapValue(primary, "sectionListRenderer")["contents"]); len(list) > 0 {
		return unwrapItemSections(list)
	}
	// A continuation puts the same items into an append action, wrapped in the
	// same item section the first page wraps them in.
	var out []map[string]any
	for _, cmd := range listOfMaps(root["onResponseReceivedCommands"]) {
		action := mapValue(cmd, "appendContinuationItemsAction")
		out = append(out, unwrapItemSections(listOfMaps(action["continuationItems"]))...)
	}
	return out
}

// unwrapItemSections opens the itemSectionRenderer each page of results arrives
// in and leaves everything else alone.
func unwrapItemSections(entries []map[string]any) []map[string]any {
	var out []map[string]any
	for _, entry := range entries {
		if sec := mapValue(entry, "itemSectionRenderer"); sec != nil {
			out = append(out, listOfMaps(sec["contents"])...)
			continue
		}
		out = append(out, entry)
	}
	return out
}

// searchItems turns one item of a section into the rows it holds.
//
// Most items hold one row. A shelf holds several and is opened in place, so
// "Latest from Rick Astley" contributes its videos at the rank the shelf sat at
// rather than being dropped or emitted as a thing with no id.
func searchItems(item map[string]any) []any {
	var out []any
	switch {
	case mapValue(item, "videoRenderer") != nil:
		if v := parseVideoRenderer(mapValue(item, "videoRenderer")); v.VideoID != "" {
			out = append(out, v)
		}
	case mapValue(item, "channelRenderer") != nil:
		if c := parseChannelRenderer(mapValue(item, "channelRenderer")); c.ChannelID != "" {
			out = append(out, c)
		}
	case mapValue(item, "officialCardViewModel") != nil:
		if c := parseOfficialCard(mapValue(item, "officialCardViewModel")); c.ChannelID != "" {
			out = append(out, c)
		}
	case mapValue(item, "playlistRenderer") != nil:
		if p := parsePlaylistRenderer(mapValue(item, "playlistRenderer")); p.PlaylistID != "" {
			out = append(out, p)
		}
	case mapValue(item, "lockupViewModel") != nil:
		r := mapValue(item, "lockupViewModel")
		switch stringValue(r["contentType"]) {
		case "LOCKUP_CONTENT_TYPE_PLAYLIST", "LOCKUP_CONTENT_TYPE_PODCAST", "LOCKUP_CONTENT_TYPE_ALBUM":
			if p := parseLockupPlaylist(r); p.PlaylistID != "" {
				out = append(out, p)
			}
		default:
			if v := parseLockupViewModel(r); v.VideoID != "" {
				out = append(out, v)
			}
		}
	case mapValue(item, "shortsLockupViewModel") != nil:
		if v := parseShortsLockupViewModel(mapValue(item, "shortsLockupViewModel")); v.VideoID != "" {
			out = append(out, v)
		}
	// The rest are boxes. Each holds its rows under a different key, because
	// nothing about this API is uniform, and each is opened in place.
	case mapValue(item, "shelfRenderer") != nil:
		content := mapValue(mapValue(item, "shelfRenderer"), "content")
		for _, key := range []string{"verticalListRenderer", "horizontalListRenderer", "expandedShelfContentsRenderer"} {
			for _, sub := range listOfMaps(mapValue(content, key)["items"]) {
				out = append(out, searchItems(sub)...)
			}
		}
	case mapValue(item, "gridShelfViewModel") != nil:
		for _, sub := range listOfMaps(mapValue(item, "gridShelfViewModel")["contents"]) {
			out = append(out, searchItems(sub)...)
		}
	case mapValue(item, "reelShelfRenderer") != nil:
		for _, sub := range listOfMaps(mapValue(item, "reelShelfRenderer")["items"]) {
			out = append(out, searchItems(sub)...)
		}
	case mapValue(item, "horizontalShelfViewModel") != nil:
		for _, sub := range listOfMaps(mapValue(item, "horizontalShelfViewModel")["items"]) {
			out = append(out, searchItems(sub)...)
		}
	}
	// Everything else on a search page is chrome: the continuation item, the
	// "did you mean" line, the ad slots, the "people also watched" card of
	// buttons. None of them is a result and none is skipped by accident.
	return out
}

// parseOfficialCard reads the card a search draws above the results for the
// channel the query names.
//
// It is a channel row and it is the best one on the page: it carries the handle,
// the subscriber count, the video count and a description, where channelRenderer
// carries a truncated snippet. It has to be read because a plain search now
// serves no channelRenderer at all, so this card is the only channel row a
// search for a channel's name returns.
func parseOfficialCard(card map[string]any) Channel {
	vm := mapValue(mapValue(card, "header"), "pageHeaderViewModel")
	c := Channel{Envelope: newEnvelope("channel", SurfaceInnerTube)}
	title := mapValue(mapValue(vm, "title"), "dynamicTextViewModel")
	c.Title = stringValue(mapValue(title, "text")["content"])
	c.ChannelID = stringValue(mapValue(mapValue(mapValue(mapValue(mapValue(title, "rendererContext"), "commandContext"), "onTap"), "innertubeCommand"), "browseEndpoint")["browseId"])
	c.Description = stringValue(mapValue(mapValue(mapValue(vm, "description"), "descriptionPreviewViewModel"), "description")["content"])
	c.Avatar = ParseThumbnails(mapValue(mapValue(mapValue(vm, "image"), "contentPreviewImageViewModel"), "image")["sources"])
	if c.ChannelID != "" {
		c.URL = BaseURL + "/channel/" + c.ChannelID
	}

	// The metadata rows are the lockup shape again: fragments in draw order with
	// nothing naming them, so each is classified on what it says.
	for _, row := range listOfMaps(mapValue(mapValue(vm, "metadata"), "contentMetadataViewModel")["metadataRows"]) {
		for _, part := range listOfMaps(row["metadataParts"]) {
			text := stringValue(mapValue(part, "text")["content"])
			switch {
			case text == "":
			case strings.HasPrefix(text, "@"):
				c.Handle = text
				c.URL = BaseURL + "/" + text
			case strings.Contains(text, "subscriber"):
				c.SubscriberCountText = text
				c.SubscriberCount = parseCountText(text)
				c.SubscriberCountIsApproximate = true
			case strings.Contains(text, "video"):
				c.VideoCountText = text
				c.VideoCount = parseCountText(text)
			}
		}
	}
	c.setVia("channel", "s2 officialCardViewModel, the card a search draws above its results")
	c.miss("a search result card: no keywords, no links, no tab strip, no join date and no lifetime view count")
	return c
}

// parsePlaylistRenderer reads the legacy playlist row, which a search still
// serves to an older client context.
func parsePlaylistRenderer(r map[string]any) Playlist {
	p := newPlaylist(stringValue(r["playlistId"]), SurfaceInnerTube)
	p.Title = extractText(r["title"])
	p.ChannelTitle = extractText(r["longBylineText"])
	p.ChannelID = ownerChannelID(r)
	if txt := extractText(r["videoCountText"]); txt != "" {
		p.VideoCountText = txt
		p.VideoCount = parseCountText(txt)
	}
	if u := joinURL(endpointURL(r["navigationEndpoint"])); u != "" {
		p.URL = u
	}
	p.Thumbnails = ParseThumbnails(mapValue(r, "thumbnail")["thumbnails"])
	p.miss("a listing row: no description, and the item list is a separate read")
	return p
}

// listOfMaps is the [] any to []map[string]any conversion this file does on
// every nesting level.
func listOfMaps(v any) []map[string]any {
	list, _ := v.([]any)
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}
