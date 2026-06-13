package youtube

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// --- JSON extraction helpers ---

// extractJSONVar finds the JSON value assigned to a JS variable marker.
// e.g. marker = "var ytInitialData = "
func extractJSONVar(html, marker string) string {
	idx := strings.Index(html, marker)
	if idx < 0 {
		return ""
	}
	start := idx + len(marker)
	for start < len(html) && (html[start] == ' ' || html[start] == '\n') {
		start++
	}
	return extractJSONObject(html, start)
}

// extractJSONCall finds the JSON argument passed to a function call marker.
// e.g. marker = "ytcfg.set("
func extractJSONCall(html, marker string) string {
	idx := strings.Index(html, marker)
	if idx < 0 {
		return ""
	}
	start := idx + len(marker)
	for start < len(html) && (html[start] == ' ' || html[start] == '\n') {
		start++
	}
	return extractJSONObject(html, start)
}

// extractJSONObject scans forward from start to find a balanced JSON object.
func extractJSONObject(s string, start int) string {
	if start >= len(s) || s[start] != '{' {
		return ""
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		ch := s[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

// extractQuotedConfig finds a quoted string value for a JSON key in an HTML page.
func extractQuotedConfig(html, key string) string {
	patterns := []string{
		`"` + key + `":"`,
		`"` + key + `": "`,
	}
	for _, pattern := range patterns {
		idx := strings.Index(html, pattern)
		if idx < 0 {
			continue
		}
		start := idx + len(pattern)
		end := start
		escaped := false
		for end < len(html) {
			ch := html[end]
			if escaped {
				escaped = false
				end++
				continue
			}
			if ch == '\\' {
				escaped = true
				end++
				continue
			}
			if ch == '"' {
				raw := html[start:end]
				decoded, err := strconv.Unquote(`"` + raw + `"`)
				if err == nil {
					return decoded
				}
				return raw
			}
			end++
		}
	}
	return ""
}

// --- Tree walking and primitive helpers ---

// walkJSON visits every map node in v depth-first, calling fn on each one.
func walkJSON(v any, fn func(map[string]any)) {
	switch x := v.(type) {
	case map[string]any:
		fn(x)
		for _, val := range x {
			walkJSON(val, fn)
		}
	case []any:
		for _, val := range x {
			walkJSON(val, fn)
		}
	}
}

// stringValue coerces any JSON scalar to a string.
func stringValue(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case fmt.Stringer:
		return x.String()
	default:
		return ""
	}
}

func int64Value(v any) int64 {
	switch x := v.(type) {
	case string:
		n, _ := strconv.ParseInt(strings.ReplaceAll(x, ",", ""), 10, 64)
		return n
	case float64:
		return int64(x)
	case int64:
		return x
	case json.Number:
		n, _ := x.Int64()
		return n
	default:
		return 0
	}
}

func boolValue(v any) bool {
	b, _ := v.(bool)
	return b
}

func mapValue(v any, key string) map[string]any {
	if key == "" {
		if m, ok := v.(map[string]any); ok {
			return m
		}
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		if child, ok := m[key].(map[string]any); ok {
			return child
		}
	}
	return nil
}

func arrayValue(v any) []any {
	if arr, ok := v.([]any); ok {
		return arr
	}
	return nil
}

func stringSlice(v any) []string {
	if arr, ok := v.([]any); ok {
		out := make([]string, 0, len(arr))
		for _, item := range arr {
			if s := stringValue(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// --- Text / count / duration helpers ---

// extractText extracts display text from YouTube's simpleText / runs / content format.
func extractText(v any) string {
	if v == nil {
		return ""
	}
	if m, ok := v.(map[string]any); ok {
		if s := stringValue(m["simpleText"]); s != "" {
			return cleanWhitespace(s)
		}
		if runs, ok := m["runs"].([]any); ok {
			var parts []string
			for _, item := range runs {
				if rm, ok := item.(map[string]any); ok {
					if txt := stringValue(rm["text"]); txt != "" {
						parts = append(parts, txt)
					}
				}
			}
			return cleanWhitespace(strings.Join(parts, ""))
		}
		if content := stringValue(m["content"]); content != "" {
			return cleanWhitespace(content)
		}
	}
	if s, ok := v.(string); ok {
		return cleanWhitespace(s)
	}
	return ""
}

// cleanWhitespace collapses runs of whitespace and strips non-breaking spaces.
func cleanWhitespace(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, " ", " ")), " ")
}

// parseCountText converts display strings like "1.2M views", "5K", "3,400" to int64.
func parseCountText(s string) int64 {
	s = strings.TrimSpace(strings.ToLower(strings.ReplaceAll(s, ",", "")))
	if s == "" {
		return 0
	}
	for _, suffix := range []string{
		" views", " view", " subscribers", " subscriber",
		" videos", " video", " comments", " comment",
		" lessons", " lesson",
	} {
		s = strings.TrimSuffix(s, suffix)
	}
	mult := float64(1)
	switch {
	case strings.HasSuffix(s, "k"):
		mult = 1_000
		s = strings.TrimSuffix(s, "k")
	case strings.HasSuffix(s, "m"):
		mult = 1_000_000
		s = strings.TrimSuffix(s, "m")
	case strings.HasSuffix(s, "b"):
		mult = 1_000_000_000
		s = strings.TrimSuffix(s, "b")
	}
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return int64(f * mult)
}

// parseDurationSeconds converts "H:MM:SS" or "M:SS" to integer seconds.
func parseDurationSeconds(s string) int {
	if s == "" {
		return 0
	}
	parts := strings.Split(s, ":")
	total := 0
	for _, part := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return 0
		}
		total = total*60 + n
	}
	return total
}

// parseDate parses an RFC3339 or YYYY-MM-DD date string.
func parseDate(s string) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// bestThumbnail returns the URL of the last (largest) thumbnail in an array.
func bestThumbnail(v any) string {
	arr := arrayValue(v)
	if len(arr) == 0 {
		return ""
	}
	best := ""
	for _, item := range arr {
		if m := mapValue(item, ""); m != nil {
			if u := stringValue(m["url"]); u != "" {
				best = u
			}
		}
	}
	return best
}

// endpointURL resolves a navigationEndpoint's webCommandMetadata URL.
func endpointURL(v any) string {
	m := mapValue(v, "")
	if m == nil {
		return ""
	}
	if wm := mapValue(m, "commandMetadata"); wm != nil {
		if web := mapValue(wm, "webCommandMetadata"); web != nil {
			return stringValue(web["url"])
		}
	}
	return ""
}

// joinURL prepends BaseURL to a relative path.
func joinURL(path string) string {
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	return BaseURL + path
}

// --- ExtractHashtags ---

// ExtractHashtags finds all #word patterns in a string and returns unique hashtags.
func ExtractHashtags(text string) []string {
	re := regexp.MustCompile(`#(\w+)`)
	matches := re.FindAllStringSubmatch(text, -1)
	seen := make(map[string]bool)
	var out []string
	for _, m := range matches {
		tag := m[1]
		if !seen[tag] {
			seen[tag] = true
			out = append(out, tag)
		}
	}
	return out
}

// --- Page parsers ---

// ParseVideoPage parses ytInitialData + ytInitialPlayerResponse from a video HTML page.
func ParseVideoPage(data *PageData, pageURL string) (*Video, []CaptionTrack, []RelatedVideo, string, error) {
	videoID := ExtractVideoID(pageURL)
	if videoID == "" {
		return nil, nil, nil, "", fmt.Errorf("cannot extract video id")
	}
	v := &Video{
		VideoID:   videoID,
		URL:       NormalizeVideoURL(videoID),
		EmbedURL:  BaseURL + "/embed/" + videoID,
		FetchedAt: time.Now(),
	}
	if pr, ok := data.PlayerResp.(map[string]any); ok {
		if details := mapValue(pr, "videoDetails"); details != nil {
			v.Title = stringValue(details["title"])
			v.Description = stringValue(details["shortDescription"])
			v.ChannelID = stringValue(details["channelId"])
			v.ChannelName = stringValue(details["author"])
			v.DurationSeconds = int(int64Value(details["lengthSeconds"]))
			v.ViewCount = int64Value(details["viewCount"])
			v.IsLive = boolValue(details["isLiveContent"])
			if arr := stringSlice(details["keywords"]); len(arr) > 0 {
				v.Tags = arr
			}
			if thumbs := mapValue(details, "thumbnail"); thumbs != nil {
				v.ThumbnailURL = bestThumbnail(thumbs["thumbnails"])
			}
		}
		if micro := mapValue(pr, "microformat"); micro != nil {
			if pm := mapValue(micro, "playerMicroformatRenderer"); pm != nil {
				if v.Description == "" {
					v.Description = stringValue(mapValue(pm, "description")["simpleText"])
				}
				v.Category = stringValue(pm["category"])
				v.UploadDate = stringValue(pm["uploadDate"])
				if published := stringValue(pm["publishDate"]); published != "" {
					v.PublishedText = published
					v.PublishedAt = parseDate(published)
				}
				v.ChannelID = firstNonEmpty(v.ChannelID, stringValue(pm["externalChannelId"]))
				v.ChannelName = firstNonEmpty(v.ChannelName, stringValue(pm["ownerChannelName"]))
				v.LikeCount = int64Value(pm["likeCount"])
				if thumb := mapValue(pm, "thumbnail"); thumb != nil && v.ThumbnailURL == "" {
					v.ThumbnailURL = bestThumbnail(thumb["thumbnails"])
				}
			}
		}
	}

	var tracks []CaptionTrack
	if pr, ok := data.PlayerResp.(map[string]any); ok {
		if caps := mapValue(pr, "captions"); caps != nil {
			if renderer := mapValue(caps, "playerCaptionsTracklistRenderer"); renderer != nil {
				for _, item := range arrayValue(renderer["captionTracks"]) {
					m := mapValue(item, "")
					ct := CaptionTrack{
						VideoID:         v.VideoID,
						LanguageCode:    stringValue(m["languageCode"]),
						Name:            extractText(m["name"]),
						BaseURL:         stringValue(m["baseUrl"]),
						Kind:            stringValue(m["kind"]),
						IsAutoGenerated: stringValue(m["kind"]) == "asr",
						FetchedAt:       time.Now(),
					}
					if ct.LanguageCode != "" && ct.BaseURL != "" {
						tracks = append(tracks, ct)
					}
				}
			}
		}
	}

	related := parseRelatedVideos(data.InitialData, v.VideoID)
	contToken := extractRelatedContinuationToken(data.InitialData)
	if txt := parseCommentCountText(data.InitialData); txt != "" {
		v.CommentCount = parseCountText(txt)
	}
	if pt := parsePublishedText(data.InitialData); pt != "" && v.PublishedText == "" {
		v.PublishedText = pt
	}
	return v, tracks, related, contToken, nil
}

// ParsePlayerDetails extracts video details from an InnerTube /player response.
// Only populates fields that have values; callers should merge into existing data.
func ParsePlayerDetails(data map[string]any, videoID string) *Video {
	v := &Video{VideoID: videoID}
	if details := mapValue(data, "videoDetails"); details != nil {
		v.Description = stringValue(details["shortDescription"])
		v.DurationSeconds = int(int64Value(details["lengthSeconds"]))
		v.ViewCount = int64Value(details["viewCount"])
		if arr := stringSlice(details["keywords"]); len(arr) > 0 {
			v.Tags = arr
		}
	}
	if micro := mapValue(data, "microformat"); micro != nil {
		if pm := mapValue(micro, "playerMicroformatRenderer"); pm != nil {
			v.Category = stringValue(pm["category"])
			v.UploadDate = stringValue(pm["uploadDate"])
			if published := stringValue(pm["publishDate"]); published != "" {
				v.PublishedAt = parseDate(published)
			}
			v.LikeCount = int64Value(pm["likeCount"])
		}
	}
	return v
}

// ParseChannelPage parses ytInitialData from a channel HTML page.
func ParseChannelPage(data *PageData, pageURL string) (*Channel, []Video, string, error) {
	ch := &Channel{URL: pageURL, FetchedAt: time.Now()}
	walkJSON(data.InitialData, func(m map[string]any) {
		if r, ok := m["channelMetadataRenderer"].(map[string]any); ok {
			ch.ChannelID = firstNonEmpty(ch.ChannelID, stringValue(r["externalId"]))
			ch.Title = firstNonEmpty(ch.Title, stringValue(r["title"]))
			ch.Description = firstNonEmpty(ch.Description, stringValue(r["description"]))
			ch.Handle = firstNonEmpty(ch.Handle, strings.TrimPrefix(stringValue(r["vanityChannelUrl"]), BaseURL+"/"))
			if ch.URL == "" {
				ch.URL = stringValue(r["channelUrl"])
			}
			if thumbs := mapValue(r, "avatar"); thumbs != nil {
				ch.AvatarURL = bestThumbnail(thumbs["thumbnails"])
			}
		}
		if r, ok := m["pageHeaderViewModel"].(map[string]any); ok {
			if banner := mapValue(r, "banner"); banner != nil {
				ch.BannerURL = bestThumbnail(mapValue(banner, "image")["sources"])
			}
		}
		if r, ok := m["videoCountText"].(map[string]any); ok && ch.VideosText == "" {
			ch.VideosText = extractText(r)
		}
		if r, ok := m["subscriberCountText"].(map[string]any); ok && ch.SubscribersText == "" {
			ch.SubscribersText = extractText(r)
		}
	})
	if ch.ChannelID != "" && strings.HasPrefix(ch.ChannelID, "UC") {
		ch.UploadsPlaylistID = "UU" + ch.ChannelID[2:]
	}
	videos := parseVideosFromTree(data.InitialData)
	for i := range videos {
		if videos[i].ChannelID == "" {
			videos[i].ChannelID = ch.ChannelID
		}
		if videos[i].ChannelName == "" {
			videos[i].ChannelName = ch.Title
		}
	}
	contToken := extractContinuationToken(data.InitialData)
	if ch.ChannelID == "" && ch.Title == "" {
		return nil, nil, "", fmt.Errorf("channel metadata not found")
	}
	return ch, dedupeVideos(videos), contToken, nil
}

// ParseContinuationVideos extracts videos and next continuation token from a /browse continuation.
func ParseContinuationVideos(data map[string]any) ([]Video, string) {
	videos := parseVideosFromTree(data)
	contToken := extractContinuationToken(data)
	return dedupeVideos(videos), contToken
}

// ParsePlaylistPage parses ytInitialData from a playlist HTML page.
func ParsePlaylistPage(data *PageData, pageURL string) (*Playlist, []PlaylistVideo, []Video, string, error) {
	playlistID := extractPlaylistID(pageURL)
	if playlistID == "" {
		return nil, nil, nil, "", fmt.Errorf("cannot extract playlist id")
	}
	p := &Playlist{
		PlaylistID: playlistID,
		URL:        NormalizePlaylistURL(playlistID),
		FetchedAt:  time.Now(),
	}
	walkJSON(data.InitialData, func(m map[string]any) {
		if r, ok := m["playlistHeaderRenderer"].(map[string]any); ok {
			p.Title = firstNonEmpty(p.Title, extractText(r["title"]))
			p.Description = firstNonEmpty(p.Description, extractText(r["descriptionText"]))
			p.ChannelName = firstNonEmpty(p.ChannelName, extractText(r["ownerText"]))
			p.ViewCountText = firstNonEmpty(p.ViewCountText, extractText(r["viewCountText"]))
			p.LastUpdatedText = firstNonEmpty(p.LastUpdatedText, extractText(r["lastUpdatedText"]))
			p.VideoCount = int(parseCountText(extractText(r["numVideosText"])))
		}
		if r, ok := m["playlistSidebarPrimaryInfoRenderer"].(map[string]any); ok {
			p.Title = firstNonEmpty(p.Title, extractText(r["title"]))
		}
		if r, ok := m["playlistSidebarSecondaryInfoRenderer"].(map[string]any); ok {
			p.ChannelName = firstNonEmpty(p.ChannelName, extractText(r["videoOwner"]))
		}
	})
	videos, edges := parsePlaylistVideos(data.InitialData, playlistID)
	contToken := extractContinuationToken(data.InitialData)
	if p.Title == "" && len(videos) == 0 {
		return nil, nil, nil, "", fmt.Errorf("playlist metadata not found")
	}
	return p, edges, dedupeVideos(videos), contToken, nil
}

// ParseContinuationPlaylistVideos extracts playlist videos from a /browse continuation.
func ParseContinuationPlaylistVideos(data map[string]any, playlistID string) ([]Video, []PlaylistVideo, string) {
	videos, edges := parsePlaylistVideos(data, playlistID)
	contToken := extractContinuationToken(data)
	return dedupeVideos(videos), edges, contToken
}

// ParseSearchPage parses a search results HTML page.
func ParseSearchPage(data *PageData, query string) ([]SearchResult, []Video, []Channel, []Playlist, string, error) {
	var (
		results   []SearchResult
		videos    []Video
		channels  []Channel
		playlists []Playlist
	)
	walkJSON(data.InitialData, func(m map[string]any) {
		if r, ok := m["videoRenderer"].(map[string]any); ok {
			v := parseVideoRenderer(r)
			if v.VideoID != "" {
				videos = append(videos, v)
				results = append(results, SearchResult{EntityType: EntityVideo, ID: v.VideoID, Title: v.Title, URL: v.URL})
			}
		}
		if r, ok := m["channelRenderer"].(map[string]any); ok {
			c := Channel{
				ChannelID:       stringValue(r["channelId"]),
				Title:           extractText(r["title"]),
				Description:     extractText(r["descriptionSnippet"]),
				SubscribersText: extractText(r["subscriberCountText"]),
				URL:             joinURL(endpointURL(r["navigationEndpoint"])),
				FetchedAt:       time.Now(),
			}
			if c.ChannelID != "" {
				channels = append(channels, c)
				results = append(results, SearchResult{EntityType: EntityChannel, ID: c.ChannelID, Title: c.Title, URL: c.URL})
			}
		}
		if r, ok := m["playlistRenderer"].(map[string]any); ok {
			p := Playlist{
				PlaylistID:  stringValue(r["playlistId"]),
				Title:       extractText(r["title"]),
				ChannelName: extractText(r["longBylineText"]),
				VideoCount:  int(parseCountText(extractText(r["videoCountText"]))),
				URL:         joinURL(endpointURL(r["navigationEndpoint"])),
				FetchedAt:   time.Now(),
			}
			if p.PlaylistID != "" {
				playlists = append(playlists, p)
				results = append(results, SearchResult{EntityType: EntityPlaylist, ID: p.PlaylistID, Title: p.Title, URL: p.URL})
			}
		}
		// Newer search results render playlists (and some videos) as
		// lockupViewModel rather than the legacy *Renderer shapes.
		if r, ok := m["lockupViewModel"].(map[string]any); ok {
			switch stringValue(r["contentType"]) {
			case "LOCKUP_CONTENT_TYPE_PLAYLIST":
				if p := parseLockupPlaylist(r); p.PlaylistID != "" {
					playlists = append(playlists, p)
					results = append(results, SearchResult{EntityType: EntityPlaylist, ID: p.PlaylistID, Title: p.Title, URL: p.URL})
				}
			case "LOCKUP_CONTENT_TYPE_VIDEO":
				if v := parseLockupViewModel(r); v.VideoID != "" {
					videos = append(videos, v)
					results = append(results, SearchResult{EntityType: EntityVideo, ID: v.VideoID, Title: v.Title, URL: v.URL})
				}
			}
		}
	})
	contToken := extractContinuationToken(data.InitialData)
	if len(results) == 0 {
		return nil, nil, nil, nil, "", fmt.Errorf("no search results found for %q", query)
	}
	return results, dedupeVideos(videos), dedupeChannels(channels), dedupePlaylists(playlists), contToken, nil
}

// ParseInnerTubeSearchResults extracts videos, channels, playlists and the next
// continuation token from an InnerTube /search continuation response.
func ParseInnerTubeSearchResults(data map[string]any) ([]Video, []Channel, []Playlist, string) {
	var videos []Video
	var channels []Channel
	var playlists []Playlist
	walkJSON(data, func(m map[string]any) {
		if r, ok := m["videoRenderer"].(map[string]any); ok {
			v := parseVideoRenderer(r)
			if v.VideoID != "" {
				videos = append(videos, v)
			}
		}
		if r, ok := m["channelRenderer"].(map[string]any); ok {
			c := Channel{
				ChannelID:       stringValue(r["channelId"]),
				Title:           extractText(r["title"]),
				Description:     extractText(r["descriptionSnippet"]),
				SubscribersText: extractText(r["subscriberCountText"]),
				URL:             joinURL(endpointURL(r["navigationEndpoint"])),
				FetchedAt:       time.Now(),
			}
			if c.ChannelID != "" {
				channels = append(channels, c)
			}
		}
		if r, ok := m["playlistRenderer"].(map[string]any); ok {
			p := Playlist{
				PlaylistID:  stringValue(r["playlistId"]),
				Title:       extractText(r["title"]),
				ChannelName: extractText(r["longBylineText"]),
				VideoCount:  int(parseCountText(extractText(r["videoCountText"]))),
				URL:         joinURL(endpointURL(r["navigationEndpoint"])),
				FetchedAt:   time.Now(),
			}
			if p.PlaylistID != "" {
				playlists = append(playlists, p)
			}
		}
	})
	contToken := extractContinuationToken(data)
	return dedupeVideos(videos), dedupeChannels(channels), dedupePlaylists(playlists), contToken
}

// ParseChapters extracts chapter data from an InnerTube /next response.
// Falls back to parsing timestamps from the video description.
func ParseChapters(nextResp map[string]any, videoID string, description string) []Chapter {
	var chapters []Chapter
	var found bool

	walkJSON(nextResp, func(m map[string]any) {
		if found {
			return
		}
		if mlr, ok := m["macroMarkersListRenderer"].(map[string]any); ok {
			contents := arrayValue(mlr["contents"])
			if len(contents) == 0 {
				return
			}
			found = true
			for i, item := range contents {
				if im, ok := item.(map[string]any); ok {
					if r, ok := im["macroMarkersListItemRenderer"].(map[string]any); ok {
						title := extractText(r["title"])
						startSecs := 0
						if onTap, ok := r["onTap"].(map[string]any); ok {
							if we := mapValue(onTap, "watchEndpoint"); we != nil {
								startSecs = int(int64Value(we["startTimeSeconds"]))
							}
						}
						thumbURL := ""
						if thumb, ok := r["thumbnail"].(map[string]any); ok {
							thumbURL = bestThumbnail(thumb["thumbnails"])
						}
						chapters = append(chapters, Chapter{
							VideoID:      videoID,
							Title:        title,
							StartSeconds: startSecs,
							ThumbnailURL: thumbURL,
							Position:     i + 1,
						})
					}
				}
			}
		}
	})

	if !found && description != "" {
		chapters = parseChaptersFromDescription(description, videoID)
	}
	return chapters
}

var reTimestamp = regexp.MustCompile(`(?m)^(\d{1,2}:\d{2}(?::\d{2})?)\s+(.+)$`)

func parseChaptersFromDescription(description, videoID string) []Chapter {
	matches := reTimestamp.FindAllStringSubmatch(description, -1)
	if len(matches) < 2 {
		return nil
	}
	var chapters []Chapter
	for i, m := range matches {
		secs := parseDurationSeconds(m[1])
		title := strings.TrimSpace(m[2])
		chapters = append(chapters, Chapter{
			VideoID:      videoID,
			Title:        title,
			StartSeconds: secs,
			Position:     i + 1,
		})
	}
	return chapters
}

// ParseVideoFormats extracts streaming format info from an InnerTube /player response.
// Deduplicates by itag, keeping the entry with the largest bitrate.
func ParseVideoFormats(playerResp map[string]any, videoID string) []VideoFormat {
	sd := mapValue(playerResp, "streamingData")
	if sd == nil {
		return nil
	}
	seen := map[int]*VideoFormat{}
	add := func(f *VideoFormat) {
		if f == nil {
			return
		}
		if prev, ok := seen[f.ITag]; !ok || f.Bitrate > prev.Bitrate {
			seen[f.ITag] = f
		}
	}
	for _, item := range arrayValue(sd["formats"]) {
		if f := parseFormat(item, videoID, false); f != nil {
			add(f)
		}
	}
	for _, item := range arrayValue(sd["adaptiveFormats"]) {
		if f := parseFormat(item, videoID, true); f != nil {
			add(f)
		}
	}
	formats := make([]VideoFormat, 0, len(seen))
	for _, f := range seen {
		formats = append(formats, *f)
	}
	return formats
}

func parseFormat(item any, videoID string, adaptive bool) *VideoFormat {
	m, ok := item.(map[string]any)
	if !ok {
		return nil
	}
	itag := int(int64Value(m["itag"]))
	if itag == 0 {
		return nil
	}
	var contentLen int64
	if s := stringValue(m["contentLength"]); s != "" {
		contentLen, _ = strconv.ParseInt(s, 10, 64)
	}
	return &VideoFormat{
		VideoID:       videoID,
		ITag:          itag,
		MimeType:      stringValue(m["mimeType"]),
		Quality:       stringValue(m["quality"]),
		QualityLabel:  stringValue(m["qualityLabel"]),
		Width:         int(int64Value(m["width"])),
		Height:        int(int64Value(m["height"])),
		FPS:           int(int64Value(m["fps"])),
		Bitrate:       int64Value(m["bitrate"]),
		ContentLength: contentLen,
		IsAdaptive:    adaptive,
		AudioQuality:  stringValue(m["audioQuality"]),
	}
}

// ParseMicroformat enriches a Video struct from microformat and playabilityStatus data.
func ParseMicroformat(playerResp map[string]any, v *Video) {
	if v == nil {
		return
	}
	if micro := mapValue(playerResp, "microformat"); micro != nil {
		if pm := mapValue(micro, "playerMicroformatRenderer"); pm != nil {
			if countries, ok := pm["availableCountries"].([]any); ok {
				for _, c := range countries {
					if s := stringValue(c); s != "" {
						v.AvailableCountries = append(v.AvailableCountries, s)
					}
				}
			}
			if fs, ok := pm["isFamilySafe"].(bool); ok {
				v.IsFamilySafe = fs
			} else {
				v.IsFamilySafe = true
			}
			if loc, ok := pm["locationDescription"].(string); ok && loc != "" {
				v.LocationDescription = loc
			}
		}
	}
	if details := mapValue(playerResp, "videoDetails"); details != nil {
		if ar, ok := details["allowRatings"].(bool); ok {
			v.AllowRatings = ar
		} else {
			v.AllowRatings = true
		}
		if kws := stringSlice(details["keywords"]); len(kws) > 0 && len(v.Tags) == 0 {
			v.Tags = kws
		}
	}
	if ps := mapValue(playerResp, "playabilityStatus"); ps != nil {
		status := stringValue(ps["status"])
		reason := strings.ToLower(stringValue(ps["reason"]))
		if status == "LOGIN_REQUIRED" || strings.Contains(reason, "age") {
			v.AgeRestricted = true
		}
	}
	if v.Description != "" {
		v.Hashtags = ExtractHashtags(v.Description)
	}
}

// ParseChannelNumericCounts enriches a Channel with numeric counts from InnerTube data.
func ParseChannelNumericCounts(data map[string]any, ch *Channel) {
	if ch == nil {
		return
	}
	walkJSON(data, func(m map[string]any) {
		if ch.SubscriberCount == 0 {
			if subs, ok := m["subscriberCountText"].(map[string]any); ok {
				txt := extractText(subs)
				if txt != "" {
					ch.SubscriberCount = parseCountText(txt)
				}
			}
		}
		if ch.VideoCount == 0 {
			if vids, ok := m["videosCountText"].(map[string]any); ok {
				txt := extractText(vids)
				if txt != "" {
					ch.VideoCount = parseCountText(txt)
				}
			}
		}
		if ch.ViewCount == 0 {
			if views, ok := m["viewCountText"].(map[string]any); ok {
				txt := extractText(views)
				if txt != "" {
					ch.ViewCount = parseCountText(txt)
				}
			}
		}
		if kw, ok := m["keywords"].(string); ok && kw != "" && len(ch.Keywords) == 0 {
			ch.Keywords = strings.Fields(kw)
		}
		if tv, ok := m["channelTrailerVideo"].(map[string]any); ok && ch.TrailerVideoID == "" {
			if vd := mapValue(tv, "videoRenderer"); vd != nil {
				ch.TrailerVideoID = stringValue(vd["videoId"])
			}
		}
		if !ch.IsVerified {
			if badges, ok := m["badges"].([]any); ok {
				for _, b := range badges {
					if bm, ok := b.(map[string]any); ok {
						if mbr := mapValue(bm, "metadataBadgeRenderer"); mbr != nil {
							if stringValue(mbr["style"]) == "BADGE_STYLE_TYPE_VERIFIED" {
								ch.IsVerified = true
							}
						}
					}
				}
			}
		}
	})
}

// ParseCommentRenderer parses a single commentRenderer or replyRenderer map.
func ParseCommentRenderer(m map[string]any, videoID, parentID string) *Comment {
	r, ok := m["commentRenderer"].(map[string]any)
	if !ok {
		r, ok = m["replyRenderer"].(map[string]any)
		if !ok {
			return nil
		}
	}
	id := stringValue(r["commentId"])
	if id == "" {
		return nil
	}
	c := &Comment{
		ID:        id,
		VideoID:   videoID,
		ParentID:  parentID,
		FetchedAt: time.Now(),
	}
	if author, ok := r["authorText"].(map[string]any); ok {
		c.AuthorDisplayName = extractText(author)
	}
	if endpoint, ok := r["authorEndpoint"].(map[string]any); ok {
		if browse := mapValue(endpoint, "browseEndpoint"); browse != nil {
			c.AuthorChannelID = stringValue(browse["browseId"])
		}
	}
	if avatar, ok := r["authorThumbnail"].(map[string]any); ok {
		c.AuthorProfileImage = bestThumbnail(avatar["thumbnails"])
	}
	if content, ok := r["contentText"].(map[string]any); ok {
		c.TextDisplay = extractText(content)
	}
	c.LikeCount = int64Value(r["voteCount"])
	if owner, ok := r["authorIsChannelOwner"].(bool); ok {
		c.IsOwnerComment = owner
	}
	if pt := extractText(r["publishedTimeText"]); pt != "" {
		c.PublishedText = pt
	}
	if replyCount, ok := r["replyCount"].(float64); ok {
		c.ReplyCount = int(replyCount)
	}
	return c
}

// CommentsRestricted reports whether a watch page's comment section was hidden
// by Restricted Mode. YouTube applies this to some server and datacenter
// requests; the section then carries only a messageRenderer to that effect.
func CommentsRestricted(root any) bool {
	restricted := false
	walkJSON(root, func(m map[string]any) {
		isr, ok := m["itemSectionRenderer"].(map[string]any)
		if !ok || stringValue(isr["sectionIdentifier"]) != "comment-item-section" {
			return
		}
		walkJSON(isr, func(mm map[string]any) {
			if mr, ok := mm["messageRenderer"].(map[string]any); ok {
				if strings.Contains(extractText(mr["text"]), "Restricted Mode") {
					restricted = true
				}
			}
		})
	})
	return restricted
}

// FindCommentsToken finds the comment-section continuation token in a watch
// page's ytInitialData (the reliable source: the /next API strips it for
// unauthenticated requests).
func FindCommentsToken(root any) string {
	var token string
	walkJSON(root, func(m map[string]any) {
		if token != "" {
			return
		}
		isr, ok := m["itemSectionRenderer"].(map[string]any)
		if !ok || stringValue(isr["sectionIdentifier"]) != "comment-item-section" {
			return
		}
		walkJSON(isr, func(mm map[string]any) {
			if cmd := mapValue(mm, "continuationCommand"); cmd != nil {
				if tok := stringValue(cmd["token"]); tok != "" && token == "" {
					token = tok
				}
			}
		})
	})
	return token
}

// collectCommentEntities gathers the commentEntityPayload entities in a
// continuation response, keyed by their entity key. Modern YouTube carries the
// comment body, author, and counts in these payloads; the rendered thread only
// references them by key.
func collectCommentEntities(root any) map[string]*Comment {
	out := map[string]*Comment{}
	walkJSON(root, func(m map[string]any) {
		p, ok := m["commentEntityPayload"].(map[string]any)
		if !ok {
			return
		}
		key, c := parseCommentEntityPayload(p)
		if key != "" && c != nil {
			out[key] = c
		}
	})
	return out
}

// parseCommentEntityPayload turns one commentEntityPayload into a Comment,
// returning the entity key the rendered thread uses to reference it.
func parseCommentEntityPayload(p map[string]any) (string, *Comment) {
	key := stringValue(p["key"])
	props := mapValue(p, "properties")
	author := mapValue(p, "author")
	toolbar := mapValue(p, "toolbar")
	commentID := stringValue(props["commentId"])
	if commentID == "" {
		return "", nil
	}
	c := &Comment{
		ID:                 commentID,
		AuthorDisplayName:  stringValue(author["displayName"]),
		AuthorChannelID:    stringValue(author["channelId"]),
		AuthorProfileImage: stringValue(author["avatarThumbnailUrl"]),
		TextDisplay:        stringValue(mapValue(props, "content")["content"]),
		PublishedText:      stringValue(props["publishedTime"]),
		IsOwnerComment:     boolValue(author["isCreator"]),
		FetchedAt:          time.Now(),
	}
	c.LikeCount = parseCountText(stringValue(toolbar["likeCountNotliked"]))
	c.ReplyCount = int(parseCountText(stringValue(toolbar["replyCount"])))
	return key, c
}

// ParseCommunityPost parses a backstagePostRenderer or sharedPostRenderer.
func ParseCommunityPost(m map[string]any, channelID string) *CommunityPost {
	r, ok := m["backstagePostRenderer"].(map[string]any)
	if !ok {
		r, ok = m["sharedPostRenderer"].(map[string]any)
		if !ok {
			return nil
		}
	}
	postID := stringValue(r["postId"])
	if postID == "" {
		return nil
	}
	p := &CommunityPost{
		PostID:    postID,
		ChannelID: channelID,
		FetchedAt: time.Now(),
	}
	if author, ok := r["authorText"].(map[string]any); ok {
		p.AuthorName = extractText(author)
	}
	if avatar, ok := r["authorThumbnail"].(map[string]any); ok {
		p.AuthorAvatar = bestThumbnail(avatar["thumbnails"])
	}
	if content, ok := r["contentText"].(map[string]any); ok {
		p.ContentText = extractText(content)
	}
	if votes, ok := r["voteCount"].(map[string]any); ok {
		p.VoteCount = extractText(votes)
	}
	walkJSON(r["likeButton"], func(m map[string]any) {
		if txt, ok := m["likeCountText"].(map[string]any); ok {
			p.LikeCount = parseCountText(extractText(txt))
		}
	})
	if pt := extractText(r["publishedTimeText"]); pt != "" {
		p.PublishedText = pt
	}
	if reply, ok := r["replyCount"].(float64); ok {
		p.ReplyCount = int(reply)
	}
	p.Attachments = parseCommunityAttachments(r)
	return p
}

func parseCommunityAttachments(r map[string]any) string {
	type attachment struct {
		Type string `json:"type"`
		URL  string `json:"url,omitempty"`
		ID   string `json:"id,omitempty"`
	}
	var atts []attachment
	if img, ok := r["backstageImageRenderer"].(map[string]any); ok {
		if images, ok := img["images"].([]any); ok {
			for _, item := range images {
				if im, ok := item.(map[string]any); ok {
					if ir, ok := im["backstageImageRenderer"].(map[string]any); ok {
						u := bestThumbnail(mapValue(ir, "image")["thumbnails"])
						if u != "" {
							atts = append(atts, attachment{Type: "image", URL: u})
						}
					}
				}
			}
		} else {
			u := bestThumbnail(mapValue(img, "image")["thumbnails"])
			if u != "" {
				atts = append(atts, attachment{Type: "image", URL: u})
			}
		}
	}
	if _, ok := r["pollRenderer"].(map[string]any); ok {
		atts = append(atts, attachment{Type: "poll"})
	}
	if vid, ok := r["videoRenderer"].(map[string]any); ok {
		atts = append(atts, attachment{Type: "video", ID: stringValue(vid["videoId"])})
	}
	if len(atts) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(atts)
	return string(b)
}

// --- Video renderer parsers ---

func parseVideoRenderer(r map[string]any) Video {
	v := Video{
		VideoID:       stringValue(r["videoId"]),
		Title:         extractText(r["title"]),
		Description:   extractText(r["descriptionSnippet"]),
		DurationText:  extractText(r["lengthText"]),
		PublishedText: extractText(r["publishedTimeText"]),
		ChannelName:   extractText(r["ownerText"]),
		ThumbnailURL:  bestThumbnail(mapValue(r, "thumbnail")["thumbnails"]),
		FetchedAt:     time.Now(),
	}
	if v.VideoID != "" {
		v.URL = BaseURL + "/watch?v=" + v.VideoID
		v.EmbedURL = BaseURL + "/embed/" + v.VideoID
	}
	v.ViewCount = parseCountText(extractText(r["viewCountText"]))
	v.DurationSeconds = parseDurationSeconds(v.DurationText)
	if nav := mapValue(r, "navigationEndpoint"); nav != nil {
		if watch := mapValue(nav, "watchEndpoint"); watch != nil && v.VideoID == "" {
			v.VideoID = stringValue(watch["videoId"])
		}
	}
	return v
}

func parseVideosFromTree(root any) []Video {
	var out []Video
	walkJSON(root, func(m map[string]any) {
		if r, ok := m["videoRenderer"].(map[string]any); ok {
			v := parseVideoRenderer(r)
			if v.VideoID != "" {
				out = append(out, v)
			}
		}
		if r, ok := m["gridVideoRenderer"].(map[string]any); ok {
			v := parseVideoRenderer(r)
			if v.VideoID != "" {
				out = append(out, v)
			}
		}
		if r, ok := m["compactVideoRenderer"].(map[string]any); ok {
			v := parseVideoRenderer(r)
			if v.VideoID != "" {
				out = append(out, v)
			}
		}
		if r, ok := m["lockupViewModel"].(map[string]any); ok {
			v := parseLockupViewModel(r)
			if v.VideoID != "" {
				out = append(out, v)
			}
		}
	})
	return out
}

// parseLockupViewModel parses YouTube's newer lockupViewModel format.
func parseLockupViewModel(r map[string]any) Video {
	contentType := stringValue(r["contentType"])
	if contentType != "" && contentType != "LOCKUP_CONTENT_TYPE_VIDEO" {
		return Video{}
	}
	videoID := stringValue(r["contentId"])
	if videoID == "" {
		return Video{}
	}
	v := Video{
		VideoID:  videoID,
		URL:      BaseURL + "/watch?v=" + videoID,
		EmbedURL: BaseURL + "/embed/" + videoID,
	}
	if meta := mapValue(r, "metadata"); meta != nil {
		if lm := mapValue(meta, "lockupMetadataViewModel"); lm != nil {
			if title := mapValue(lm, "title"); title != nil {
				v.Title = stringValue(title["content"])
			}
			if md := mapValue(lm, "metadata"); md != nil {
				if cr := mapValue(md, "contentMetadataViewModel"); cr != nil {
					if parts, ok := cr["metadataRows"].([]any); ok {
						for _, row := range parts {
							if rm, ok := row.(map[string]any); ok {
								if mps, ok := rm["metadataParts"].([]any); ok {
									for _, mp := range mps {
										if mpm, ok := mp.(map[string]any); ok {
											if txt := mapValue(mpm, "text"); txt != nil {
												content := stringValue(txt["content"])
												if strings.Contains(content, " views") {
													v.ViewCount = parseCountText(content)
												} else if strings.Contains(content, " ago") || strings.Contains(content, "Streamed") {
													v.PublishedText = content
												} else if v.ChannelName == "" {
													v.ChannelName = content
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
	if img := mapValue(r, "contentImage"); img != nil {
		if tm := mapValue(img, "thumbnailViewModel"); tm != nil {
			if image := mapValue(tm, "image"); image != nil {
				v.ThumbnailURL = bestThumbnail(image["sources"])
			}
			if overlays, ok := tm["overlays"].([]any); ok {
				for _, o := range overlays {
					if om, ok := o.(map[string]any); ok {
						if bov := mapValue(om, "thumbnailBottomOverlayViewModel"); bov != nil {
							if badges, ok := bov["badges"].([]any); ok {
								for _, b := range badges {
									if bm, ok := b.(map[string]any); ok {
										if tbvm := mapValue(bm, "thumbnailBadgeViewModel"); tbvm != nil {
											text := stringValue(tbvm["text"])
											if text != "" && strings.Contains(text, ":") {
												v.DurationText = text
												v.DurationSeconds = parseDurationSeconds(text)
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
	v.FetchedAt = time.Now()
	return v
}

// parseLockupPlaylist parses a lockupViewModel with LOCKUP_CONTENT_TYPE_PLAYLIST.
func parseLockupPlaylist(r map[string]any) Playlist {
	contentType := stringValue(r["contentType"])
	if contentType != "LOCKUP_CONTENT_TYPE_PLAYLIST" {
		return Playlist{}
	}
	p := Playlist{
		PlaylistID: stringValue(r["contentId"]),
		FetchedAt:  time.Now(),
	}
	if p.PlaylistID == "" {
		return Playlist{}
	}
	p.URL = BaseURL + "/playlist?list=" + p.PlaylistID

	if meta := mapValue(r, "metadata"); meta != nil {
		if lm := mapValue(meta, "lockupMetadataViewModel"); lm != nil {
			if title := mapValue(lm, "title"); title != nil {
				p.Title = stringValue(title["content"])
			}
			if md := mapValue(lm, "metadata"); md != nil {
				if cr := mapValue(md, "contentMetadataViewModel"); cr != nil {
					if parts, ok := cr["metadataRows"].([]any); ok {
						for _, row := range parts {
							if rm, ok := row.(map[string]any); ok {
								if mps, ok := rm["metadataParts"].([]any); ok {
									for _, mp := range mps {
										if mpm, ok := mp.(map[string]any); ok {
											if txt := mapValue(mpm, "text"); txt != nil {
												content := stringValue(txt["content"])
												if strings.Contains(strings.ToLower(content), "updated") {
													p.LastUpdatedText = content
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
	if img := mapValue(r, "contentImage"); img != nil {
		walkJSON(img, func(m map[string]any) {
			if tbvm := mapValue(m, "thumbnailBadgeViewModel"); tbvm != nil {
				text := stringValue(tbvm["text"])
				if text != "" {
					p.VideoCount = int(parseCountText(text))
				}
			}
		})
	}
	return p
}

// parsePlaylistsFromTree walks a JSON tree and extracts Playlist items.
func parsePlaylistsFromTree(root any) []Playlist {
	var out []Playlist
	walkJSON(root, func(m map[string]any) {
		if r, ok := m["lockupViewModel"].(map[string]any); ok {
			p := parseLockupPlaylist(r)
			if p.PlaylistID != "" {
				out = append(out, p)
			}
		}
		if r, ok := m["gridPlaylistRenderer"].(map[string]any); ok {
			p := Playlist{
				PlaylistID: stringValue(r["playlistId"]),
				Title:      extractText(r["title"]),
				VideoCount: int(parseCountText(extractText(r["videoCountText"]))),
				URL:        joinURL(endpointURL(r["navigationEndpoint"])),
				FetchedAt:  time.Now(),
			}
			if p.PlaylistID != "" {
				out = append(out, p)
			}
		}
	})
	return out
}

// ParseContinuationPlaylists extracts playlists and next continuation token from a /browse continuation.
func ParseContinuationPlaylists(data map[string]any) ([]Playlist, string) {
	playlists := parsePlaylistsFromTree(data)
	contToken := extractContinuationToken(data)
	return dedupePlaylists(playlists), contToken
}

func parsePlaylistVideos(root any, playlistID string) ([]Video, []PlaylistVideo) {
	var (
		videos []Video
		edges  []PlaylistVideo
		seen   = map[string]struct{}{}
		pos    = 0
	)
	walkJSON(root, func(m map[string]any) {
		if r, ok := m["playlistVideoRenderer"].(map[string]any); ok {
			videoID := stringValue(r["videoId"])
			if videoID == "" {
				return
			}
			if _, ok := seen[videoID]; ok {
				return
			}
			seen[videoID] = struct{}{}
			pos++
			v := Video{
				VideoID:         videoID,
				Title:           extractText(r["title"]),
				ChannelName:     extractText(r["shortBylineText"]),
				DurationText:    extractText(r["lengthText"]),
				ThumbnailURL:    bestThumbnail(mapValue(r, "thumbnail")["thumbnails"]),
				URL:             BaseURL + "/watch?v=" + videoID,
				EmbedURL:        BaseURL + "/embed/" + videoID,
				FetchedAt:       time.Now(),
				DurationSeconds: parseDurationSeconds(extractText(r["lengthText"])),
			}
			videos = append(videos, v)
			edges = append(edges, PlaylistVideo{PlaylistID: playlistID, VideoID: videoID, Position: pos})
		}
	})
	return videos, edges
}

func parseRelatedVideos(root any, videoID string) []RelatedVideo {
	var out []RelatedVideo
	seen := map[string]struct{}{}
	pos := 0
	for _, v := range dedupeVideos(parseVideosFromTree(root)) {
		if v.VideoID == "" || v.VideoID == videoID {
			continue
		}
		if _, ok := seen[v.VideoID]; ok {
			continue
		}
		seen[v.VideoID] = struct{}{}
		pos++
		out = append(out, RelatedVideo{VideoID: videoID, RelatedVideoID: v.VideoID, Position: pos})
	}
	return out
}

// ParseContinuationRelatedVideos extracts related videos and next token from a /next continuation.
func ParseContinuationRelatedVideos(data map[string]any, videoID string) ([]RelatedVideo, string) {
	videos := parseVideosFromTree(data)
	videos = dedupeVideos(videos)
	var out []RelatedVideo
	pos := 0
	for _, v := range videos {
		if v.VideoID == "" || v.VideoID == videoID {
			continue
		}
		pos++
		out = append(out, RelatedVideo{VideoID: videoID, RelatedVideoID: v.VideoID, Position: pos})
	}
	contToken := extractContinuationToken(data)
	return out, contToken
}

func parseCommentCountText(root any) string {
	var out string
	walkJSON(root, func(m map[string]any) {
		if out != "" {
			return
		}
		if r, ok := m["commentsEntryPointHeaderRenderer"].(map[string]any); ok {
			out = extractText(r["commentCount"])
		}
	})
	return out
}

func parsePublishedText(root any) string {
	var out string
	walkJSON(root, func(m map[string]any) {
		if out != "" {
			return
		}
		if r, ok := m["dateText"].(map[string]any); ok {
			out = extractText(r)
		}
	})
	return out
}

// --- Continuation token extractors ---

// extractContinuationToken finds the first continuation token in a JSON tree
// from continuationItemRenderer elements.
func extractContinuationToken(root any) string {
	var token string
	walkJSON(root, func(m map[string]any) {
		if token != "" {
			return
		}
		if cir, ok := m["continuationItemRenderer"].(map[string]any); ok {
			if ep := mapValue(cir, "continuationEndpoint"); ep != nil {
				if cmd := mapValue(ep, "continuationCommand"); cmd != nil {
					if t := stringValue(cmd["token"]); t != "" {
						token = t
					}
				}
			}
		}
	})
	return token
}

// extractRelatedContinuationToken finds the continuation token in the secondaryResults section.
func extractRelatedContinuationToken(root any) string {
	rootMap, ok := root.(map[string]any)
	if !ok {
		return ""
	}
	sr := mapValue(mapValue(mapValue(rootMap, "contents"), "twoColumnWatchNextResults"), "secondaryResults")
	if sr == nil {
		return ""
	}
	return extractContinuationToken(sr)
}

// extractCommentContinuationToken finds the comment continuation token in a /next response.
func extractCommentContinuationToken(root any) string {
	var token string
	walkJSON(root, func(m map[string]any) {
		if token != "" {
			return
		}
		isr, ok := m["itemSectionRenderer"].(map[string]any)
		if !ok {
			return
		}
		sid := stringValue(isr["sectionIdentifier"])
		if sid != "comment-item-section" {
			return
		}
		for _, ci := range arrayValue(isr["contents"]) {
			cim, ok := ci.(map[string]any)
			if !ok {
				continue
			}
			cir, ok := cim["continuationItemRenderer"].(map[string]any)
			if !ok {
				continue
			}
			if ep := mapValue(cir, "continuationEndpoint"); ep != nil {
				if cmd := mapValue(ep, "continuationCommand"); cmd != nil {
					if t := stringValue(cmd["token"]); t != "" {
						token = t
						return
					}
				}
			}
		}
	})
	return token
}

// extractCommentContFromNextResp is a fallback for finding comment continuation tokens.
func extractCommentContFromNextResp(root map[string]any) string {
	var token string
	walkJSON(root, func(m map[string]any) {
		if token != "" {
			return
		}
		if cir, ok := m["continuationItemRenderer"].(map[string]any); ok {
			if ep := mapValue(cir, "continuationEndpoint"); ep != nil {
				if cmd := mapValue(ep, "continuationCommand"); cmd != nil {
					if t := stringValue(cmd["token"]); t != "" {
						token = t
					}
				}
			}
		}
	})
	return token
}

// extractReplyToken finds the reply continuation token inside a commentThreadRenderer.
func extractReplyToken(ctr map[string]any) string {
	replies := mapValue(ctr, "replies")
	if replies == nil {
		return ""
	}
	crr := mapValue(replies, "commentRepliesRenderer")
	if crr == nil {
		return ""
	}
	contents := arrayValue(crr["contents"])
	for _, item := range contents {
		if im, ok := item.(map[string]any); ok {
			if cir, ok := im["continuationItemRenderer"].(map[string]any); ok {
				if ep := mapValue(cir, "continuationEndpoint"); ep != nil {
					if cmd := mapValue(ep, "continuationCommand"); cmd != nil {
						if tok := stringValue(cmd["token"]); tok != "" {
							return tok
						}
					}
				}
			}
		}
	}
	return ""
}

// --- Hashtag page parser ---

// parseHashtagPage extracts videos and continuation token from a Browse hashtag response.
func parseHashtagPage(data map[string]any) ([]Video, string) {
	var videos []Video
	var contToken string

	walkJSON(data, func(m map[string]any) {
		if rir, ok := m["richItemRenderer"].(map[string]any); ok {
			if content, ok := rir["content"].(map[string]any); ok {
				if vr, ok := content["videoRenderer"].(map[string]any); ok {
					if v := parseVideoRenderer(vr); v.VideoID != "" {
						videos = append(videos, v)
					}
				}
			}
		}
		if vr, ok := m["videoRenderer"].(map[string]any); ok {
			if v := parseVideoRenderer(vr); v.VideoID != "" {
				videos = append(videos, v)
			}
		}
		if gvr, ok := m["gridVideoRenderer"].(map[string]any); ok {
			if v := parseVideoRenderer(gvr); v.VideoID != "" {
				videos = append(videos, v)
			}
		}
		if cir, ok := m["continuationItemRenderer"].(map[string]any); ok {
			if ep := mapValue(cir, "continuationEndpoint"); ep != nil {
				if cmd := mapValue(ep, "continuationCommand"); cmd != nil {
					if t := stringValue(cmd["token"]); t != "" && contToken == "" {
						contToken = t
					}
				}
			}
		}
	})

	return dedupeVideos(videos), contToken
}

// --- Dedup helpers ---

func dedupeVideos(items []Video) []Video {
	seen := map[string]Video{}
	order := make([]string, 0, len(items))
	for _, item := range items {
		if item.VideoID == "" {
			continue
		}
		if _, ok := seen[item.VideoID]; !ok {
			order = append(order, item.VideoID)
		}
		prev := seen[item.VideoID]
		seen[item.VideoID] = mergeVideo(prev, item)
	}
	out := make([]Video, 0, len(order))
	for _, id := range order {
		out = append(out, seen[id])
	}
	return out
}

func dedupeChannels(items []Channel) []Channel {
	seen := map[string]Channel{}
	order := []string{}
	for _, item := range items {
		if item.ChannelID == "" {
			continue
		}
		if _, ok := seen[item.ChannelID]; !ok {
			order = append(order, item.ChannelID)
		}
		prev := seen[item.ChannelID]
		if prev.Title == "" {
			seen[item.ChannelID] = item
		}
	}
	out := make([]Channel, 0, len(order))
	for _, id := range order {
		out = append(out, seen[id])
	}
	return out
}

func dedupePlaylists(items []Playlist) []Playlist {
	seen := map[string]Playlist{}
	order := []string{}
	for _, item := range items {
		if item.PlaylistID == "" {
			continue
		}
		if _, ok := seen[item.PlaylistID]; !ok {
			order = append(order, item.PlaylistID)
		}
		prev := seen[item.PlaylistID]
		if prev.Title == "" {
			seen[item.PlaylistID] = item
		}
	}
	out := make([]Playlist, 0, len(order))
	for _, id := range order {
		out = append(out, seen[id])
	}
	return out
}

func mergeVideo(a, b Video) Video {
	if a.VideoID == "" {
		return b
	}
	if a.Title == "" {
		a.Title = b.Title
	}
	if a.Description == "" {
		a.Description = b.Description
	}
	if a.ChannelID == "" {
		a.ChannelID = b.ChannelID
	}
	if a.ChannelName == "" {
		a.ChannelName = b.ChannelName
	}
	if a.DurationSeconds == 0 {
		a.DurationSeconds = b.DurationSeconds
	}
	if a.DurationText == "" {
		a.DurationText = b.DurationText
	}
	if a.ViewCount == 0 {
		a.ViewCount = b.ViewCount
	}
	if a.ThumbnailURL == "" {
		a.ThumbnailURL = b.ThumbnailURL
	}
	if a.URL == "" {
		a.URL = b.URL
	}
	if a.EmbedURL == "" {
		a.EmbedURL = b.EmbedURL
	}
	return a
}

// trendingQuery maps a category name to a search query string.
func trendingQuery(category string) string {
	switch category {
	case "music":
		return "trending music"
	case "gaming":
		return "trending gaming"
	case "news":
		return "trending news today"
	case "movies":
		return "new movie trailers"
	default:
		return "trending"
	}
}

// Satisfy the url import.
var _ = url.QueryEscape
