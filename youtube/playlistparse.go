package youtube

import (
	"strings"
)

// playlistparse.go reads a playlist's header and its items. Doc 03 section 4.
//
// A playlist page serves one of two header shapes and which one you get depends
// on how the playlist was made:
//
//	PL...        a person's playlist        header.pageHeaderRenderer
//	PL... auto   an auto-generated one      header.pageHeaderRenderer
//	UU...        a channel's uploads        header.playlistHeaderRenderer
//	OLAK5uy_...  an album                   header.playlistHeaderRenderer
//
// They share almost no key names. playlistHeaderRenderer has numVideosText,
// viewCountText, byline and ownerText, all named. pageHeaderViewModel has a list
// of rendered fragments with nothing named at all, an owner hidden inside an
// avatar stack, and no "Updated" anywhere on it.
//
// So both are read by name and neither is guessed at. The old parser walked the
// whole tree with a blind firstNonEmpty merge, which cannot tell which shape
// answered and so cannot say why a field is missing. Here the shape that answered
// is recorded in via, and a field the shape does not carry is named in missed.

// ParsePlaylistRecord reads a playlist header out of a browse response or a page's
// ytInitialData.
//
// It returns nil when neither header shape is present, which is what a mix does:
// a mix answers with alerts, microformat and nothing else, and the caller turns
// that into the refusal rather than into an empty playlist.
func ParsePlaylistRecord(root map[string]any, playlistID, surface string) *Playlist {
	p := newPlaylist(playlistID, surface)
	header := mapValue(root, "header")
	switch {
	case mapValue(header, "playlistHeaderRenderer") != nil:
		applyPlaylistHeaderRenderer(&p, mapValue(header, "playlistHeaderRenderer"))
		p.setVia("header", "s2 header.playlistHeaderRenderer, the named shape")
	case mapValue(mapValue(header, "pageHeaderRenderer"), "content") != nil:
		vm := mapValue(mapValue(mapValue(header, "pageHeaderRenderer"), "content"), "pageHeaderViewModel")
		if vm == nil {
			return nil
		}
		applyPlaylistPageHeader(&p, vm)
		p.setVia("header", "s2 header.pageHeaderRenderer, the view model shape")
		p.miss("this header shape carries no updated_text; only playlistHeaderRenderer states when a playlist last changed")
	default:
		return nil
	}
	// The microformat block is served under both shapes and is the only place the
	// description survives when the header truncated it.
	if mf := mapValue(mapValue(root, "microformat"), "microformatDataRenderer"); mf != nil {
		if d := stringValue(mf["description"]); d != "" && len(d) > len(p.Description) {
			p.Description = d
			p.setVia("description", "s2 microformat.microformatDataRenderer, which is not truncated")
		}
		if u := stringValue(mf["unlisted"]); u == "true" {
			p.Visibility = "unlisted"
		}
		if b, ok := mf["unlisted"].(bool); ok {
			p.Visibility = map[bool]string{true: "unlisted", false: "public"}[b]
		}
	}
	for _, w := range alertWarnings(root) {
		p.miss("YouTube said: %s", w)
	}
	return &p
}

// applyPlaylistHeaderRenderer reads the named header shape.
//
// Every field here is a key with a name on it, which is why this shape is the
// easy one. byline is the exception: it is an array of blocks whose text carries
// "Updated 4 days ago" and there is no key saying so, so it is classified like a
// lockup fragment.
func applyPlaylistHeaderRenderer(p *Playlist, r map[string]any) {
	p.Title = extractText(r["title"])
	p.Description = extractText(r["descriptionText"])
	p.ChannelTitle = extractText(r["ownerText"])
	p.ChannelID = stringValue(mapValue(mapValue(r, "ownerEndpoint"), "browseEndpoint")["browseId"])
	if p.ChannelID == "" {
		p.ChannelID = ownerChannelID(r)
	}
	if txt := extractText(r["numVideosText"]); txt != "" {
		p.VideoCountText = txt
		p.VideoCount = parseCountText(txt)
	}
	if txt := extractText(r["viewCountText"]); txt != "" {
		p.ViewCountText = txt
		p.ViewCount = parseCountText(txt)
	}
	if txt := extractText(r["lastUpdatedText"]); txt != "" {
		p.UpdatedText = txt
	}
	applyPlaylistSubtitle(p, extractText(r["subtitle"]))
	// The byline restates what the named keys already said and sometimes carries a
	// line none of them do. Each fragment is classified by what it is, and it only
	// fills a field that is still empty; a fragment this read recognises is never
	// also kept as an unrecognised one, which is what put "435 videos" in
	// metadata_parts next to the video count it had already been read into.
	for _, txt := range playlistBylineTexts(r) {
		switch {
		case strings.Contains(strings.ToLower(txt), "updated"):
			if p.UpdatedText == "" {
				p.UpdatedText = txt
			}
		case strings.HasSuffix(txt, " videos"), strings.HasSuffix(txt, " video"):
			if p.VideoCountText == "" {
				p.VideoCountText = txt
				p.VideoCount = parseCountText(txt)
			}
		case strings.HasSuffix(txt, " views"), strings.HasSuffix(txt, " view"):
			if p.ViewCountText == "" {
				p.ViewCountText = txt
				p.ViewCount = parseCountText(txt)
			}
		case txt == p.ChannelTitle, txt == p.Title:
		default:
			p.MetadataParts = appendOnce(p.MetadataParts, txt)
		}
	}
	for _, b := range renderersUnder(r, "metadataBadgeRenderer") {
		switch strings.ToLower(extractText(b["label"])) {
		case "unlisted":
			p.Visibility = "unlisted"
		case "public":
			p.Visibility = "public"
		case "private":
			p.Visibility = "private"
		}
	}
	p.Thumbnails = ParseThumbnails(mapValue(mapValue(mapValue(r, "playlistHeaderBanner"), "heroPlaylistThumbnailRenderer"), "thumbnail")["thumbnails"])
}

// applyPlaylistSubtitle reads the artist off an album header.
//
// An album is the one playlist that names its owner nowhere else. A channel's
// uploads carries ownerText with a link on it and no subtitle at all; the album
// OLAK5uy_lGQfnMNGvYCRdDq9ZLzJV2BJL2aHQsz9Y carries no ownerText, no
// ownerEndpoint, and the single line "Oasis • Album". So the artist is read off
// that line and the missing id is stated rather than left to look like an album
// nobody made.
//
// The pieces are classified one by one because their order is not promised and
// the type word is localized while the artist's name is not. A piece this read
// does not recognise is kept verbatim.
func applyPlaylistSubtitle(p *Playlist, subtitle string) {
	if subtitle == "" {
		return
	}
	for _, part := range strings.Split(subtitle, "•") {
		part = strings.TrimSpace(part)
		switch {
		case part == "":
		case part == "Album", part == "Single", part == "EP", part == "Playlist", part == "Podcast":
		case p.ChannelTitle == "":
			p.ChannelTitle = part
			p.setVia("channel_title", "s2 header.subtitle, which names the artist but does not link it")
			p.miss("this playlist states its owner as text only, so there is no channel_id for %q", part)
		default:
			p.MetadataParts = appendOnce(p.MetadataParts, part)
		}
	}
}

// playlistBylineTexts flattens the byline blocks of a playlistHeaderRenderer.
func playlistBylineTexts(r map[string]any) []string {
	list, _ := r["byline"].([]any)
	var out []string
	for _, b := range list {
		bm, ok := b.(map[string]any)
		if !ok {
			continue
		}
		if txt := extractText(mapValue(bm, "playlistBylineRenderer")["text"]); txt != "" {
			out = append(out, txt)
		}
	}
	return out
}

// applyPlaylistPageHeader reads the view model header shape.
//
// Row 0 is an avatar stack holding the owner and row 1 is a list of fragments
// reading "Playlist", "10 videos", "435 views". The owner's rendered text is
// localized and sometimes carries a prefix: it is "Rick Astley" on one playlist,
// "by Music" on another and "của Âm nhạc" under --hl vi. The command run covers
// the whole string including the "by ", so the prefix cannot be stripped by
// offset and is trimmed as text, in the one language this read can be sure of.
func applyPlaylistPageHeader(p *Playlist, vm map[string]any) {
	p.Title = stringValue(mapValue(mapValue(mapValue(vm, "title"), "dynamicTextViewModel"), "text")["content"])
	p.Description = stringValue(mapValue(mapValue(mapValue(vm, "description"), "descriptionPreviewViewModel"), "description")["content"])
	rows, _ := mapValue(mapValue(vm, "metadata"), "contentMetadataViewModel")["metadataRows"].([]any)
	for _, row := range rows {
		rm, ok := row.(map[string]any)
		if !ok {
			continue
		}
		parts, _ := rm["metadataParts"].([]any)
		for _, mp := range parts {
			mpm, ok := mp.(map[string]any)
			if !ok {
				continue
			}
			if stack := mapValue(mpm, "avatarStack"); stack != nil {
				applyPlaylistOwner(p, stack)
				continue
			}
			txt := stringValue(mapValue(mpm, "text")["content"])
			switch {
			case txt == "":
			case strings.HasSuffix(txt, " videos"), strings.HasSuffix(txt, " video"):
				p.VideoCountText = txt
				p.VideoCount = parseCountText(txt)
			case strings.HasSuffix(txt, " views"), strings.HasSuffix(txt, " view"):
				p.ViewCountText = txt
				p.ViewCount = parseCountText(txt)
			case strings.Contains(strings.ToLower(txt), "updated"):
				p.UpdatedText = txt
			case txt == "Playlist", txt == "Podcast", txt == "Album", txt == "Mix":
			case strings.EqualFold(txt, "unlisted"), strings.EqualFold(txt, "private"), strings.EqualFold(txt, "public"):
				p.Visibility = strings.ToLower(txt)
			default:
				p.MetadataParts = append(p.MetadataParts, txt)
			}
		}
	}
	p.Thumbnails = ParseThumbnails(mapValue(mapValue(mapValue(vm, "heroImage"), "contentPreviewImageViewModel"), "image")["sources"])
}

// applyPlaylistOwner reads the owner out of an avatar stack.
func applyPlaylistOwner(p *Playlist, stack map[string]any) {
	vm := mapValue(stack, "avatarStackViewModel")
	if vm == nil {
		return
	}
	txt := mapValue(vm, "text")
	name := stringValue(txt["content"])
	p.ChannelTitle = strings.TrimSpace(strings.TrimPrefix(name, "by "))
	if p.ChannelTitle != name {
		p.setVia("channel_title", "s2 avatarStack, with YouTube's own \"by \" prefix trimmed")
	}
	runs, _ := txt["commandRuns"].([]any)
	for _, run := range runs {
		rm, ok := run.(map[string]any)
		if !ok {
			continue
		}
		cmd := mapValue(mapValue(rm, "onTap"), "innertubeCommand")
		if id := stringValue(mapValue(cmd, "browseEndpoint")["browseId"]); strings.HasPrefix(id, "UC") {
			p.ChannelID = id
		}
	}
}

// renderersUnder collects every renderer of the given key under a subtree.
func renderersUnder(root any, key string) []map[string]any {
	var out []map[string]any
	walkJSON(root, func(m map[string]any) {
		if r, ok := m[key].(map[string]any); ok {
			out = append(out, r)
		}
	})
	return out
}

// ParsePlaylistItems reads a page of playlist items and the token for the next.
//
// Position is the yield order and nothing else. The old playlistVideoRenderer
// carried an index and a setVideoId and it is gone: a census over four live
// playlist pages found 100, 98, 64 and 37 lockupViewModel on them and zero
// playlistVideoRenderer. A lockup states neither, so position is counted here
// from a base the caller carries across pages, and set_video_id is simply not
// available at tier 0 any more.
func ParsePlaylistItems(root any, base int) ([]Video, string) {
	var out []Video
	seen := map[string]struct{}{}
	pos := base
	add := func(v Video) {
		if v.VideoID == "" {
			return
		}
		if _, ok := seen[v.VideoID]; ok {
			return
		}
		seen[v.VideoID] = struct{}{}
		pos++
		v.Position = pos
		v.miss("set_video_id is not available: the lockupViewModel that replaced playlistVideoRenderer does not carry one")
		out = append(out, v)
	}
	walkJSON(root, func(m map[string]any) {
		// playlistVideoRenderer is kept because a response from an older client
		// context still serves it, not because a web response does.
		if r, ok := m["playlistVideoRenderer"].(map[string]any); ok {
			if videoID := stringValue(r["videoId"]); videoID != "" {
				v := *NewVideo(videoID, SurfaceInnerTube)
				v.Title = extractText(r["title"])
				v.ChannelTitle = extractText(r["shortBylineText"])
				v.ChannelID = ownerChannelID(r)
				v.DurationText = extractText(r["lengthText"])
				v.DurationSeconds = parseDurationSeconds(v.DurationText)
				v.Thumbnails = ParseThumbnails(mapValue(r, "thumbnail")["thumbnails"])
				v.ThumbnailURL = largestThumbnail(v.Thumbnails)
				v.SetVideoID = stringValue(r["setVideoId"])
				lockupMisses(&v)
				add(v)
			}
		}
		if r, ok := m["lockupViewModel"].(map[string]any); ok {
			add(parseLockupViewModel(r))
		}
		// A shorts playlist renders none of the above. UUSHuAXFkgsw1L7xaCfnd5JJOw
		// says 294 videos in its header and its page carries 98 shortsLockupViewModel
		// and not one playlistVideoRenderer, so a reader that knows only the first
		// two returns an empty playlist for a channel with 294 shorts in it.
		if r, ok := m["shortsLockupViewModel"].(map[string]any); ok {
			add(parseShortsLockupViewModel(r))
		}
	})
	return out, extractContinuationToken(root)
}
