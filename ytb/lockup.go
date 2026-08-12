package ytb

import (
	"regexp"
	"strings"
)

// lockup.go reads the lockupViewModel, which is the item shape the site now uses
// everywhere a list of things is drawn. Doc 02 section 4.2.
//
// The old renderers named their fields: videoRenderer had viewCountText and
// publishedTimeText and a parser looked them up. A lockup has neither. It has a
// list of rendered fragments in the order the UI draws them, with a delimiter,
// and nothing saying what any of them means:
//
//	metadataRows[0].metadataParts[0].text.content = "Arcangel"
//	metadataRows[1].metadataParts[0].text.content = "32M views"
//	metadataRows[1].metadataParts[1].text.content = "1 month ago"
//
// So this parser cannot look fields up. It classifies each fragment on what the
// fragment says, and anything it does not recognise goes into metadata_parts
// verbatim rather than being dropped. That last rule is the point of the design:
// a fragment YouTube adds tomorrow, or one that arrives in another language
// because the caller passed --hl, shows up in the output as an unclassified
// string that a census can count, instead of vanishing without trace.

// reLockupDuration matches a lockup's duration badge, "3:51" or "1:02:04". It is
// anchored because a fragment like "12:00 PM" is not a duration.
var reLockupDuration = regexp.MustCompile(`^\d+(:\d\d)+$`)

// lockupPart is one rendered fragment with the tap command that decorates it.
//
// The command matters: it is the only thing distinguishing a channel name from
// any other short string. "Arcangel" is a channel because its whole run taps
// through to a browseEndpoint, not because it failed every other test.
type lockupPart struct {
	text    string
	browse  string
	isVideo bool
}

// lockupParts flattens a lockup's metadata rows into fragments in draw order.
func lockupParts(r map[string]any) []lockupPart {
	rows, _ := mapValue(mapValue(mapValue(r, "metadata"), "lockupMetadataViewModel"), "metadata")["contentMetadataViewModel"].(map[string]any)
	list, _ := rows["metadataRows"].([]any)
	var out []lockupPart
	for _, row := range list {
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
			txt := mapValue(mpm, "text")
			content := stringValue(txt["content"])
			if content == "" {
				continue
			}
			p := lockupPart{text: content}
			if runs, ok := txt["commandRuns"].([]any); ok {
				for _, run := range runs {
					rm, ok := run.(map[string]any)
					if !ok {
						continue
					}
					cmd := mapValue(mapValue(rm, "onTap"), "innertubeCommand")
					if id := stringValue(mapValue(cmd, "browseEndpoint")["browseId"]); id != "" {
						p.browse = id
					}
					if mapValue(cmd, "watchEndpoint") != nil {
						p.isVideo = true
					}
				}
			}
			out = append(out, p)
		}
	}
	return out
}

// applyLockupParts runs doc 02 section 4.2's classification table over a video's
// fragments.
//
// The order of the cases is the table's order and it matters. "Streamed 2 years
// ago" has to be tested before the plain relative time, and a part carrying a
// channel browse id is a channel whatever its text looks like.
func applyLockupParts(v *Video, parts []lockupPart) {
	for _, p := range parts {
		text := p.text
		switch {
		case strings.HasPrefix(p.browse, "UC"):
			if v.ChannelTitle == "" {
				v.ChannelTitle = strings.TrimSpace(strings.TrimPrefix(text, "by "))
			}
			if v.ChannelID == "" {
				v.ChannelID = p.browse
				v.setVia("channel_id", "s2 the owner fragment's own tap command")
			}
		case strings.Contains(text, " watching"):
			// "1.2K watching" is a live viewer count and not a view count, but it is
			// the only number the row carries, so it is kept as one and live_state
			// says what it really counts.
			v.ViewCountText = text
			v.ViewCount = parseCountText(text)
			v.LiveState = "live"
		case strings.HasPrefix(text, "Streamed "):
			v.PublishedText = text
			v.IsLiveContent = boolPtr(true)
		case strings.HasSuffix(text, " views"), strings.HasSuffix(text, " view"):
			v.ViewCountText = text
			v.ViewCount = parseCountText(text)
		case isRelativeTimeText(text):
			v.PublishedText = text
		case reLockupDuration.MatchString(text):
			v.DurationText = text
			v.DurationSeconds = parseDurationSeconds(text)
		case text == "LIVE", text == "Live":
			v.LiveState = "live"
		case strings.HasPrefix(text, "Scheduled for "), strings.HasPrefix(text, "Premieres "):
			v.LiveState = "upcoming"
			v.MetadataParts = append(v.MetadataParts, text)
		case looksLikeCount(text):
			// A continuation page drops the word and sends a bare "101K". It is a view
			// count there and nowhere else, so it only fills the field the full text
			// would have filled and never overwrites one.
			if v.ViewCount == 0 {
				v.ViewCountText = text
				v.ViewCount = parseCountText(text)
			}
		default:
			v.MetadataParts = append(v.MetadataParts, text)
		}
	}
}

// parseLockupViewModel reads a lockup that holds a video.
func parseLockupViewModel(r map[string]any) Video {
	contentType := stringValue(r["contentType"])
	if contentType != "" && contentType != "LOCKUP_CONTENT_TYPE_VIDEO" {
		return Video{}
	}
	videoID := stringValue(r["contentId"])
	if videoID == "" {
		return Video{}
	}
	v := *NewVideo(videoID, SurfaceInnerTube)
	v.Title = stringValue(mapValue(mapValue(mapValue(r, "metadata"), "lockupMetadataViewModel"), "title")["content"])
	applyLockupParts(&v, lockupParts(r))
	if v.ViewCountText != "" {
		v.setVia("view_count", "s2 lockup metadata, rounded as the site rendered it")
	}
	if v.ChannelID == "" {
		v.ChannelID = lockupChannelID(r)
		if v.ChannelID != "" {
			v.setVia("channel_id", "s2 the first channel link on the row, since no fragment named the owner")
		}
	}
	applyLockupImage(&v, r)
	lockupMisses(&v)
	return v
}

// applyLockupImage reads the thumbnail and its overlay badges.
//
// The duration is not in the metadata rows. It is a badge drawn on the thumbnail,
// in the same list as LIVE, New, 4K, CC and the members-only marker, so the badge
// that matches a clock is the duration and every other badge is kept as one.
func applyLockupImage(v *Video, r map[string]any) {
	tm := mapValue(mapValue(r, "contentImage"), "thumbnailViewModel")
	if tm == nil {
		return
	}
	if image := mapValue(tm, "image"); image != nil {
		v.Thumbnails = ParseThumbnails(image["sources"])
		v.ThumbnailURL = largestThumbnail(v.Thumbnails)
	}
	for _, text := range lockupBadges(tm) {
		switch {
		case reLockupDuration.MatchString(text):
			v.DurationText = text
			v.DurationSeconds = parseDurationSeconds(text)
			v.setVia("duration_seconds", "s2 the badge drawn on the thumbnail")
		case strings.EqualFold(text, "LIVE"):
			v.LiveState = "live"
			v.Badges = appendOnce(v.Badges, text)
		default:
			v.Badges = appendOnce(v.Badges, text)
		}
	}
}

// badgeContainers are the two overlay shapes that hold badges, with the key
// their badge list is under. A video lockup draws its duration in the first and a
// playlist lockup draws "10 videos" in the second, so a parser that knows only
// one of them silently loses a field on half the site.
var badgeContainers = [][2]string{
	{"thumbnailBottomOverlayViewModel", "badges"},
	{"thumbnailOverlayBadgeViewModel", "thumbnailBadges"},
}

// lockupBadges collects the badge texts drawn on a lockup's thumbnail.
//
// Only the badge overlays are read. The rest of the overlay tree is hover states
// and buttons that add the video to a playlist or a queue, and walking it would
// collect "Play all" and "Watch later" as if they described the video.
func lockupBadges(tm map[string]any) []string {
	overlays, _ := tm["overlays"].([]any)
	var out []string
	for _, o := range overlays {
		om, ok := o.(map[string]any)
		if !ok {
			continue
		}
		for _, container := range badgeContainers {
			holder := mapValue(om, container[0])
			if holder == nil {
				continue
			}
			badges, _ := holder[container[1]].([]any)
			for _, b := range badges {
				bm, ok := b.(map[string]any)
				if !ok {
					continue
				}
				if text := stringValue(mapValue(bm, "thumbnailBadgeViewModel")["text"]); text != "" {
					out = append(out, text)
				}
			}
		}
	}
	return out
}

// lockupChannelID digs the uploader's channel id out of a lockupViewModel.
//
// This is the fallback for a lockup whose metadata carries no channel run at all,
// such as the rows on a channel's own uploads page where the owner is implied.
// The id is then only present in the avatar stack or a tap command further down,
// so the whole subtree is searched for the first UC browse id.
func lockupChannelID(r map[string]any) string {
	var found string
	walkJSON(r, func(m map[string]any) {
		if found != "" {
			return
		}
		if id := stringValue(mapValue(m, "browseEndpoint")["browseId"]); strings.HasPrefix(id, "UC") {
			found = id
		}
	})
	return found
}

// parseLockupPlaylist reads a lockup that holds a playlist.
//
// A playlist lockup's fragments are its own set: the word "Playlist", a video
// count, a view count, an "Updated 4 days ago", and an owner whose text is
// localized and sometimes prefixed. YouTube renders that owner as "Rick Astley"
// on one playlist and "by Music" on another, and the command run covers the whole
// string including the "by ", so the prefix is trimmed by text and not by offset.
func parseLockupPlaylist(r map[string]any) Playlist {
	if stringValue(r["contentType"]) != "LOCKUP_CONTENT_TYPE_PLAYLIST" {
		return Playlist{}
	}
	id := stringValue(r["contentId"])
	if id == "" {
		return Playlist{}
	}
	p := newPlaylist(id, SurfaceInnerTube)
	p.Title = stringValue(mapValue(mapValue(mapValue(r, "metadata"), "lockupMetadataViewModel"), "title")["content"])
	for _, part := range lockupParts(r) {
		text := part.text
		switch {
		case strings.HasPrefix(part.browse, "VL"):
			// "View full playlist", a link back to the thing being described. It says
			// nothing the contentId did not already say.
		case strings.HasPrefix(part.browse, "UC"):
			// The owner run is first and the word "Playlist" that follows carries the
			// same browse id, so the first one wins and the label does not overwrite it.
			if p.ChannelID == "" {
				p.ChannelID = part.browse
				p.ChannelTitle = strings.TrimSpace(strings.TrimPrefix(text, "by "))
				break
			}
			if text == "Playlist" || text == "Podcast" || text == "Album" {
				break
			}
			p.MetadataParts = append(p.MetadataParts, text)
		case strings.HasSuffix(text, " views"), strings.HasSuffix(text, " view"):
			p.ViewCountText = text
			p.ViewCount = parseCountText(text)
		case strings.HasSuffix(text, " videos"), strings.HasSuffix(text, " video"):
			p.VideoCountText = text
			p.VideoCount = parseCountText(text)
		case strings.HasSuffix(text, " episodes"), strings.HasSuffix(text, " episode"):
			p.VideoCountText = text
			p.VideoCount = parseCountText(text)
		case strings.Contains(strings.ToLower(text), "updated"):
			p.UpdatedText = text
		case text == "Playlist", text == "Podcast", text == "Album":
			p.MetadataParts = appendOnce(p.MetadataParts, text)
		default:
			p.MetadataParts = append(p.MetadataParts, text)
		}
	}
	// A playlist lockup's image is a stack rather than a single thumbnail, so it
	// nests one level deeper than a video's, and the video count rides on it as an
	// overlay badge. On a search result that badge is the only place the count is
	// stated at all.
	tm := mapValue(mapValue(mapValue(mapValue(r, "contentImage"), "collectionThumbnailViewModel"), "primaryThumbnail"), "thumbnailViewModel")
	p.Thumbnails = ParseThumbnails(mapValue(tm, "image")["sources"])
	if p.VideoCount == 0 {
		for _, text := range lockupBadges(tm) {
			if n := parseCountText(text); n > 0 {
				p.VideoCount = n
				p.VideoCountText = text
				break
			}
		}
	}
	p.miss("a listing row: no description, and the item list is a separate read")
	return p
}
