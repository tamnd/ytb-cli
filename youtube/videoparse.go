package youtube

import (
	"strings"
)

// videoparse.go reads a player response and a watch page into a Video. Doc 03
// section 2.
//
// The two census maps below are the reason this file is not a guess. A player
// response carries sixteen keys under videoDetails and twenty under microformat,
// and a parser that reads eight of them looks exactly like a parser that read all
// of them. Every key is listed with what it becomes, and the ones deliberately
// dropped say why they were dropped. A test walks a real capture and fails on a key
// that is not in the map, so a field YouTube adds shows up as a red test rather
// than as data nobody noticed was missing.

// videoDetailsFields is every key seen under videoDetails, with its destination.
var videoDetailsFields = map[string]string{
	"videoId":                "VideoID",
	"title":                  "Title",
	"lengthSeconds":          "DurationSeconds, unless microformat disagrees; see durationOf",
	"keywords":               "Keywords",
	"channelId":              "ChannelID",
	"shortDescription":       "Description",
	"viewCount":              "ViewCount",
	"author":                 "ChannelTitle",
	"thumbnail":              "Thumbnails",
	"allowRatings":           "AllowRatings",
	"isPrivate":              "IsPrivate",
	"isCrawlable":            "IsCrawlable",
	"isLiveContent":          "IsLiveContent",
	"isOwnerViewing":         "dropped: a property of the caller's session, not of the video",
	"isUnpluggedCorpus":      "dropped: YouTube TV licensing, never true on a public upload",
	"isTvfilmVideo":          "dropped: same, and it is false on every video read so far",
	"isLowLatencyLiveStream": "dropped: a streaming detail, and LiveState already says live",
	"latencyClass":           "dropped: same",
	"isLiveDvrEnabled":       "dropped: same",
	"liveChunkReadahead":     "dropped: same",
	"musicVideoType":         "dropped: only music.youtube.com sets it, and the music plane reads it there",
}

// microformatFields is every key seen under playerMicroformatRenderer.
var microformatFields = map[string]string{
	"externalVideoId":      "VideoID",
	"title":                "Title, when videoDetails did not answer",
	"description":          "Description, when videoDetails did not answer",
	"lengthSeconds":        "DurationSeconds; this block wins, see durationOf",
	"category":             "Category",
	"publishDate":          "PublishedAt",
	"uploadDate":           "UploadDate",
	"viewCount":            "ViewCount, when videoDetails did not answer",
	"likeCount":            "LikeCount, the only tier 0 source of it",
	"externalChannelId":    "ChannelID",
	"ownerChannelName":     "ChannelTitle",
	"ownerProfileUrl":      "ChannelURL and ChannelHandle",
	"canonicalUrl":         "CanonicalURL, and IsShort when it is a /shorts/ path",
	"embed":                "EmbedURL",
	"thumbnail":            "Thumbnails",
	"availableCountries":   "AvailableCountries",
	"isFamilySafe":         "IsFamilySafe",
	"isUnlisted":           "IsUnlisted",
	"isShortsEligible":     "dropped: eligibility is not the same claim as IsShort",
	"hasYpcMetadata":       "dropped: paid-content plumbing, false on every public upload",
	"liveBroadcastDetails": "LiveState",
	"locationDescription":  "LocationDescription",
	"publishedTimeText":    "PublishedText",
}

// NewVideo starts a record for a video id with its derived URLs already set, so
// every read produces the same three URLs whatever surface answered.
func NewVideo(videoID string, surfaces ...string) *Video {
	return &Video{
		VideoID:  videoID,
		URL:      NormalizeVideoURL(videoID),
		ShortURL: "https://youtu.be/" + videoID,
		EmbedURL: BaseURL + "/embed/" + videoID,
		Envelope: newEnvelope("video", surfaces...),
	}
}

// ParsePlayerResponse fills a Video from a /player response or from the watch
// page's ytInitialPlayerResponse, which are the same shape. surface is the surface
// id the response came from, so via can name the block that won a disagreement.
func ParsePlayerResponse(pr map[string]any, v *Video, surface string) {
	if pr == nil || v == nil {
		return
	}
	v.addSurface(surface)

	details := mapValue(pr, "videoDetails")
	micro := mapValue(mapValue(pr, "microformat"), "playerMicroformatRenderer")

	if details != nil {
		v.VideoID = firstNonEmpty(v.VideoID, stringValue(details["videoId"]))
		v.Title = firstNonEmpty(v.Title, stringValue(details["title"]))
		v.Description = firstNonEmpty(v.Description, stringValue(details["shortDescription"]))
		v.ChannelID = firstNonEmpty(v.ChannelID, stringValue(details["channelId"]))
		v.ChannelTitle = firstNonEmpty(v.ChannelTitle, stringValue(details["author"]))
		if n := int64Value(details["viewCount"]); n > 0 {
			v.ViewCount = n
		}
		if kws := stringSlice(details["keywords"]); len(kws) > 0 {
			v.Keywords = kws
		}
		v.IsLiveContent = firstFlag(v.IsLiveContent, details["isLiveContent"])
		v.IsPrivate = firstFlag(v.IsPrivate, details["isPrivate"])
		v.IsCrawlable = firstFlag(v.IsCrawlable, details["isCrawlable"])
		v.AllowRatings = firstFlag(v.AllowRatings, details["allowRatings"])
		v.Thumbnails = mergeThumbnails(v.Thumbnails, ParseThumbnails(mapValue(details, "thumbnail")["thumbnails"]))
	}

	if micro != nil {
		v.VideoID = firstNonEmpty(v.VideoID, stringValue(micro["externalVideoId"]))
		v.Title = firstNonEmpty(v.Title, extractText(micro["title"]))
		v.Description = firstNonEmpty(v.Description, extractText(micro["description"]))
		v.Category = firstNonEmpty(v.Category, stringValue(micro["category"]))
		v.CanonicalURL = firstNonEmpty(v.CanonicalURL, stringValue(micro["canonicalUrl"]))
		v.ChannelID = firstNonEmpty(v.ChannelID, stringValue(micro["externalChannelId"]))
		v.ChannelTitle = firstNonEmpty(v.ChannelTitle, stringValue(micro["ownerChannelName"]))
		v.LocationDescription = firstNonEmpty(v.LocationDescription, stringValue(micro["locationDescription"]))
		v.PublishedText = firstNonEmpty(v.PublishedText, extractText(micro["publishedTimeText"]))
		if u := stringValue(micro["ownerProfileUrl"]); u != "" {
			// microformat spells this http:// where every other block says https://,
			// and two spellings of one channel URL is two nodes in a graph.
			v.ChannelURL = firstNonEmpty(v.ChannelURL, httpsScheme(u))
			v.ChannelHandle = firstNonEmpty(v.ChannelHandle, handleFromURL(v.ChannelURL))
		}
		if u := stringValue(mapValue(micro, "embed")["iframeUrl"]); u != "" {
			v.EmbedURL = u
		}
		if n := int64Value(micro["viewCount"]); n > 0 && v.ViewCount == 0 {
			v.ViewCount = n
		}
		if n := int64Value(micro["likeCount"]); n > 0 {
			v.LikeCount = n
			v.setVia("like_count", surface+" microformat.likeCount")
		}
		if t := parseDate(stringValue(micro["publishDate"])); !t.IsZero() {
			v.PublishedAt = t
		}
		if t := parseDate(stringValue(micro["uploadDate"])); !t.IsZero() {
			v.UploadDate = t
		}
		if arr := stringSlice(micro["availableCountries"]); len(arr) > 0 {
			v.AvailableCountries = arr
		}
		v.IsFamilySafe = firstFlag(v.IsFamilySafe, micro["isFamilySafe"])
		v.IsUnlisted = firstFlag(v.IsUnlisted, micro["isUnlisted"])
		v.Thumbnails = mergeThumbnails(v.Thumbnails, ParseThumbnails(mapValue(micro, "thumbnail")["thumbnails"]))
		if state := liveStateOf(micro); state != "" {
			v.LiveState = state
		}
	}

	// A short is a short because YouTube's own canonical URL says /shorts/. The
	// microformat's isShortsEligible is a different question with the same shape,
	// and reading it here is how a nine minute video gets reported as a short.
	if v.CanonicalURL != "" {
		v.IsShort = boolPtr(strings.Contains(v.CanonicalURL, "/shorts/"))
	}

	if secs, block := durationOf(details, micro); secs > 0 {
		v.DurationSeconds = secs
		v.DurationText = formatDurationClock(secs)
		v.setVia("duration_seconds", surface+" "+block)
	}

	if ps := mapValue(pr, "playabilityStatus"); ps != nil {
		v.Playability = parsePlayability(ps)
		v.AgeRestricted = ageRestrictedOf(v.Playability)
	}

	if tracks := ParseCaptionTracks(pr, v.VideoID); len(tracks) > 0 {
		v.CaptionTracks = tracks
	}

	v.ThumbnailURL = largestThumbnail(v.Thumbnails)
}

// durationOf picks a length and names the block it came from.
//
// The two blocks disagree, measured on dQw4w9WgXcQ: videoDetails says 213 and
// microformat says 214, and the page's own schema.org duration is PT3M34S, which
// is 214. So microformat wins and the microdata is what broke the tie. The block
// name goes in via, because "s1" alone would not say which of the two answered.
func durationOf(details, micro map[string]any) (int, string) {
	if micro != nil {
		if n := int(int64Value(micro["lengthSeconds"])); n > 0 {
			return n, "microformat.lengthSeconds"
		}
	}
	if details != nil {
		if n := int(int64Value(details["lengthSeconds"])); n > 0 {
			return n, "videoDetails.lengthSeconds"
		}
	}
	return 0, ""
}

// liveStateOf reads liveBroadcastDetails into the one word that answers "is this
// live now". IsLiveContent cannot: it stays true forever on a finished stream.
func liveStateOf(micro map[string]any) string {
	lbd := mapValue(micro, "liveBroadcastDetails")
	if lbd == nil {
		return ""
	}
	switch {
	case boolValue(lbd["isLiveNow"]):
		return "live"
	case stringValue(lbd["endTimestamp"]) != "":
		return "ended"
	case stringValue(lbd["startTimestamp"]) != "":
		// A start with no end and not live now is a stream that has not begun,
		// which is a premiere or a scheduled broadcast.
		return "upcoming"
	}
	return ""
}

// parsePlayability reads playabilityStatus, including the error screen, which is
// where the sentence explaining a refusal actually lives.
func parsePlayability(ps map[string]any) *Playability {
	p := &Playability{
		Status:          stringValue(ps["status"]),
		Reason:          stringValue(ps["reason"]),
		PlayableInEmbed: flagOf(ps["playableInEmbed"]),
	}
	if es := mapValue(mapValue(ps, "errorScreen"), "playerErrorMessageRenderer"); es != nil {
		p.Reason = firstNonEmpty(p.Reason, extractText(es["reason"]))
		p.ReasonDetail = extractText(es["subreason"])
	}
	if p.Status == "" && p.Reason == "" {
		return nil
	}
	return p
}

// ageRestrictedOf answers from the playability status rather than guessing.
//
// YouTube never says "age restricted" in a field. It says LOGIN_REQUIRED with a
// reason sentence, and the same status covers a private video, so the reason is
// what separates them. OK is a real no: the video played without a sign in.
func ageRestrictedOf(p *Playability) *bool {
	if p == nil {
		return nil
	}
	switch p.Status {
	case "OK":
		return boolPtr(false)
	case "LOGIN_REQUIRED", "AGE_VERIFICATION_REQUIRED", "CONTENT_CHECK_REQUIRED":
		reason := strings.ToLower(p.Reason + " " + p.ReasonDetail)
		if strings.Contains(reason, "age") || strings.Contains(reason, "inappropriate") ||
			strings.Contains(reason, "confirm your age") || p.Status == "AGE_VERIFICATION_REQUIRED" {
			return boolPtr(true)
		}
	}
	return nil
}

// ParseCaptionTracks reads the caption track list off a player response. The text
// is a separate read; this is the index.
func ParseCaptionTracks(pr map[string]any, videoID string) []CaptionTrack {
	renderer := mapValue(mapValue(pr, "captions"), "playerCaptionsTracklistRenderer")
	if renderer == nil {
		return nil
	}
	var out []CaptionTrack
	for _, item := range arrayValue(renderer["captionTracks"]) {
		m := mapValue(item, "")
		if m == nil {
			continue
		}
		kind := stringValue(m["kind"])
		ct := CaptionTrack{
			VideoID:         videoID,
			LanguageCode:    stringValue(m["languageCode"]),
			Name:            extractText(m["name"]),
			BaseURL:         stringValue(m["baseUrl"]),
			Kind:            kind,
			IsAutoGenerated: kind == "asr",
		}
		if ct.LanguageCode == "" || ct.BaseURL == "" {
			continue
		}
		out = append(out, ct)
	}
	return out
}

// ParseWatchData fills the fields that only ytInitialData has: the linked
// description, the rendered counts and dates, and the comment count.
//
// None of these duplicate the player response. The player response has the
// description as flat text and no links in it, has the exact view count but not
// the "1.7B views" a page renders, and has no comment count at all.
func ParseWatchData(root any, v *Video, surface string) {
	if root == nil || v == nil {
		return
	}
	v.addSurface(surface)

	if runs := watchDescriptionRuns(root); len(runs) > 0 {
		v.DescriptionRuns = runs
		v.Links = RunLinks(runs)
		v.Mentions = RunMentions(runs)
		v.Hashtags = RunHashtags(runs)
		v.setVia("description_runs", surface+" videoSecondaryInfoRenderer.attributedDescription")
		if v.Description == "" {
			v.Description = runsText(runs)
		}
	}

	walkJSON(root, func(m map[string]any) {
		if r, ok := m["videoPrimaryInfoRenderer"].(map[string]any); ok {
			v.Title = firstNonEmpty(v.Title, extractText(r["title"]))
			if vc := mapValue(mapValue(r, "viewCount"), "videoViewCountRenderer"); vc != nil {
				// The long form is what a person reading a row wants and the short
				// form is what a narrow terminal wants. Keep the long one: it is the
				// one that still says the exact number.
				v.ViewCountText = firstNonEmpty(v.ViewCountText, extractText(vc["viewCount"]), extractText(vc["shortViewCount"]))
				if boolValue(vc["isLive"]) && v.LiveState == "" {
					v.LiveState = "live"
				}
			}
			// dateText is the absolute "Oct 24, 2009" and relativeDateText the
			// "16 years ago". published_text takes the relative one, because that is
			// what every listing surface puts there and mixing the two spellings in
			// one field makes the field unusable.
			v.PublishedText = firstNonEmpty(v.PublishedText, extractText(r["relativeDateText"]), extractText(r["dateText"]))
		}
		if r, ok := m["videoOwnerRenderer"].(map[string]any); ok {
			v.ChannelTitle = firstNonEmpty(v.ChannelTitle, extractText(r["title"]))
			if nav := mapValue(r, "navigationEndpoint"); nav != nil {
				if be := mapValue(nav, "browseEndpoint"); be != nil {
					v.ChannelID = firstNonEmpty(v.ChannelID, stringValue(be["browseId"]))
					if base := stringValue(be["canonicalBaseUrl"]); strings.HasPrefix(base, "/@") {
						v.ChannelHandle = firstNonEmpty(v.ChannelHandle, strings.TrimPrefix(base, "/"))
					}
				}
			}
		}
	})

	if v.ChannelID != "" && v.ChannelURL == "" {
		v.ChannelURL = BaseURL + "/channel/" + v.ChannelID
	}
	if txt := parseCommentCountText(root); txt != "" {
		v.CommentCount = parseCountText(txt)
		v.setVia("comment_count", surface+" commentsEntryPointHeaderRenderer")
	} else {
		v.miss("comment count not on the page, ytb comments reads it")
	}
}

// watchDescriptionRuns finds the attributed description. It is on the page twice,
// under videoSecondaryInfoRenderer and again in the description engagement panel,
// and the two carry the same content with the same offsets, so the first one wins.
func watchDescriptionRuns(root any) []TextRun {
	var runs []TextRun
	walkJSON(root, func(m map[string]any) {
		if len(runs) > 0 {
			return
		}
		for _, key := range []string{"attributedDescription", "attributedDescriptionBodyText"} {
			if node, ok := m[key].(map[string]any); ok {
				if parsed := ParseAttributedRuns(node); len(parsed) > 0 {
					runs = parsed
					return
				}
			}
		}
	})
	return runs
}

// runsText reassembles the plain text of a set of runs. It is the original
// content string, because ParseAttributedRuns keeps the gaps between commands.
func runsText(runs []TextRun) string {
	var b strings.Builder
	for _, r := range runs {
		b.WriteString(r.Text)
	}
	return b.String()
}

// ParseChapters reads a video's chapters, preferring the ones YouTube itself
// recognised. See the Chapter doc comment for why origin is a field.
func ParseChapters(nextResp map[string]any, videoID string, runs []TextRun, description string) []Chapter {
	if chapters := chaptersFromMarkers(nextResp, videoID); len(chapters) > 0 {
		return chapters
	}
	if chapters := chaptersFromRuns(runs, videoID); len(chapters) > 0 {
		return chapters
	}
	return chaptersFromDescription(description, videoID)
}

// chaptersFromMarkers reads a macroMarkersListRenderer, which is what the player
// draws on the scrubber.
func chaptersFromMarkers(nextResp map[string]any, videoID string) []Chapter {
	var chapters []Chapter
	var found bool
	walkJSON(nextResp, func(m map[string]any) {
		if found {
			return
		}
		mlr, ok := m["macroMarkersListRenderer"].(map[string]any)
		if !ok {
			return
		}
		contents := arrayValue(mlr["contents"])
		if len(contents) == 0 {
			return
		}
		found = true
		for _, item := range contents {
			r := mapValue(mapValue(item, ""), "macroMarkersListItemRenderer")
			if r == nil {
				continue
			}
			chapters = append(chapters, Chapter{
				VideoID:      videoID,
				Title:        extractText(r["title"]),
				StartSeconds: int(int64Value(mapValue(mapValue(r, "onTap"), "watchEndpoint")["startTimeSeconds"])),
				ThumbnailURL: bestThumbnail(mapValue(r, "thumbnail")["thumbnails"]),
				Position:     len(chapters) + 1,
				Origin:       ChapterFromMarkers,
			})
		}
	})
	return chapters
}

// chaptersFromRuns reads chapters off the timestamp links in the description.
//
// This beats a regex over the text for one reason: YouTube linkified these itself,
// so the offset is a number it supplied rather than one parsed out of "1:23", and
// "1:23" written in a sentence is not linked and does not become a chapter here.
// The title is the rest of the line the timestamp sits on. See runChapterTitle for
// which end of the line that is.
func chaptersFromRuns(runs []TextRun, videoID string) []Chapter {
	var chapters []Chapter
	for i, r := range runs {
		if r.Kind != RunTimestamp {
			continue
		}
		chapters = append(chapters, Chapter{
			VideoID:      videoID,
			Title:        runChapterTitle(runs, i),
			StartSeconds: r.StartSeconds,
			Position:     len(chapters) + 1,
			Origin:       ChapterFromDescription,
		})
	}
	// One timestamp is a link to a moment, not a chapter list. YouTube's own rule
	// is that chapters need a marker at 0:00 and at least three of them, and a
	// single "watch the bit at 2:10" should not produce a one chapter video.
	if len(chapters) < 2 {
		return nil
	}
	return chapters
}

// runChapterTitle names the chapter a timestamp run belongs to.
//
// Both layouts are real and neither is rare. 3blue1brown writes "0:00 Introduction
// example" with the timestamp first, and koi64's guitar mix writes "Eyes To The Sky
// - 0:00:00" with the title first. Reading only the run after the timestamp gets
// the first layout and gives the second a chapter list with every title blank,
// which is exactly what it did on 3b-1N526pFM.
//
// So: whatever is left on the line after the timestamp, and when that is nothing,
// whatever was on it before.
func runChapterTitle(runs []TextRun, i int) string {
	if i+1 < len(runs) {
		after, _, _ := strings.Cut(runs[i+1].Text, "\n")
		if t := trimChapterTitle(after); t != "" {
			return t
		}
	}
	if i > 0 {
		lines := strings.Split(runs[i-1].Text, "\n")
		if t := trimChapterTitle(lines[len(lines)-1]); t != "" {
			return t
		}
	}
	return ""
}

// trimChapterTitle strips the whitespace and the dash, colon or bullet an uploader
// puts between a title and its timestamp. The no-break space is in the cutset
// because YouTube's own description runs are full of them.
func trimChapterTitle(s string) string {
	return strings.Trim(s, " \t -–—:|·•")
}

// chaptersFromDescription is the last resort, for a read that has the description
// as flat text and never saw the linked version: /player answers with
// shortDescription and no runs at all.
//
// Both line layouts get a pass, in that order, and the timestamp-first one wins a
// tie. A tracklist line reads "Eyes To The Sky - 0:02:20", and matching only
// "0:02:20 Eyes To The Sky" would report that video as having no chapters.
func chaptersFromDescription(description, videoID string) []Chapter {
	matches := reTimestamp.FindAllStringSubmatch(description, -1)
	if len(matches) >= 2 {
		return descriptionChapters(matches, videoID, 1, 2)
	}
	if trailing := reTimestampTrailing.FindAllStringSubmatch(description, -1); len(trailing) >= 2 {
		return descriptionChapters(trailing, videoID, 2, 1)
	}
	return nil
}

// descriptionChapters turns regex matches into chapters. stamp and title are the
// submatch indexes, which swap between the two line layouts.
func descriptionChapters(matches [][]string, videoID string, stamp, title int) []Chapter {
	chapters := make([]Chapter, 0, len(matches))
	for i, m := range matches {
		chapters = append(chapters, Chapter{
			VideoID:      videoID,
			Title:        trimChapterTitle(m[title]),
			StartSeconds: parseDurationSeconds(m[stamp]),
			Position:     i + 1,
			Origin:       ChapterFromDescription,
		})
	}
	return chapters
}

// mergeThumbnails adds renditions that are not already there, keyed on URL.
func mergeThumbnails(into, add []Thumbnail) []Thumbnail {
	for _, t := range add {
		dup := false
		for _, have := range into {
			if have.URL == t.URL {
				dup = true
				break
			}
		}
		if !dup {
			into = append(into, t)
		}
	}
	return into
}

// largestThumbnail returns the widest rendition's URL, for the one-URL callers:
// a table column, a Markdown export, a SQL row.
func largestThumbnail(thumbs []Thumbnail) string {
	best := ""
	widest := -1
	for _, t := range thumbs {
		if t.Width > widest {
			widest = t.Width
			best = t.URL
		}
	}
	return best
}

// handleFromURL pulls the @handle out of a channel URL, or returns "" for the
// /channel/UC... form, which carries no handle.
func handleFromURL(raw string) string {
	_, after, ok := strings.Cut(raw, "/@")
	if !ok {
		return ""
	}
	handle, _, _ := strings.Cut(after, "/")
	if handle == "" {
		return ""
	}
	return "@" + handle
}

// flagOf reads a JSON bool into a *bool: set when the key was present, nil when it
// was not. Every flag on the record goes through this or firstFlag, because the
// whole point of *bool is that absent and false are different answers.
func flagOf(v any) *bool {
	b, ok := v.(bool)
	if !ok {
		return nil
	}
	return &b
}

// firstFlag keeps a flag an earlier surface already answered. A later read that
// does not carry the key must not erase one that did.
func firstFlag(have *bool, v any) *bool {
	if have != nil {
		return have
	}
	return flagOf(v)
}

// boolPtr is for the flags this package decides rather than reads.
func boolPtr(b bool) *bool {
	return &b
}

// flagTrue collapses a flag to a plain bool for the callers that have to branch:
// a Markdown export cannot write "we do not know". Absent reads as false here, and
// the difference stays visible in the JSON, which is where it belongs.
func flagTrue(b *bool) bool {
	return b != nil && *b
}
