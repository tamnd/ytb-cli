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

// extractJSONCall finds the JSON argument passed to a function call marker,
// e.g. marker = "ytcfg.set(".
//
// It tries every occurrence and returns the largest object it finds, because the
// marker is not unique and the first hit is usually not the one wanted. A watch
// page calls ytcfg.set five times: the first is ytcfg.set('EMERGENCY_BASE_URL',
// '/error...'), a two-argument string call with no object in it at all, and the
// configuration block with the API key and the player build in it is the second.
// Stopping at the first hit is why PLAYER_JS_URL read as empty on every page.
func extractJSONCall(html, marker string) string {
	best := ""
	for at := 0; ; {
		idx := strings.Index(html[at:], marker)
		if idx < 0 {
			return best
		}
		start := at + idx + len(marker)
		for start < len(html) && (html[start] == ' ' || html[start] == '\n' || html[start] == '\r' || html[start] == '\t') {
			start++
		}
		if obj := extractJSONObject(html, start); len(obj) > len(best) {
			best = obj
		}
		at += idx + len(marker)
	}
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

// compactCountRe matches a bare display count like "101K", "1.2M", "423", "1,234".
var compactCountRe = regexp.MustCompile(`^\d[\d.,]*\s*[KMB]?$`)

// looksLikeCount reports whether s is a bare compact count with no unit word, as
// served in /browse continuation lockups (e.g. "101K" instead of "101K views").
func looksLikeCount(s string) bool {
	return compactCountRe.MatchString(strings.TrimSpace(s))
}

// isRelativeTimeText reports whether s is a published/scheduled time string such
// as "1 month ago", "1mo ago", "Streamed 2 days ago", or "Premiered 5 hours ago".
func isRelativeTimeText(s string) bool {
	return strings.Contains(s, " ago") ||
		strings.Contains(s, "Streamed") ||
		strings.Contains(s, "Premiered") ||
		strings.Contains(s, "Scheduled")
}

// formatDurationClock renders seconds as "M:SS" or "H:MM:SS", the inverse of
// parseDurationSeconds. It returns "" for non-positive input.
func formatDurationClock(total int) string {
	if total <= 0 {
		return ""
	}
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
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

// ParseVideoPage parses a watch page into a Video with its related videos and the
// continuation token for the rest of them. The record itself is built in
// videoparse.go; this is the page-shaped wrapper around it.
func ParseVideoPage(data *PageData, pageURL string) (*Video, []Video, string, error) {
	videoID := ExtractVideoID(pageURL)
	if videoID == "" {
		return nil, nil, "", fmt.Errorf("cannot extract video id")
	}
	v := NewVideo(videoID, SurfaceWatchHTML)
	v.addSource(pageURL)
	if pr, ok := data.PlayerResp.(map[string]any); ok {
		ParsePlayerResponse(pr, v, SurfaceWatchHTML)
	}
	ParseWatchData(data.InitialData, v, SurfaceWatchHTML)
	if data.ClientVersion != "" {
		v.addClient("WEB")
	}
	return v, ParseRelatedShelf(data.InitialData, v.VideoID), extractRelatedContinuationToken(data.InitialData), nil
}

// pageHeaderMetadataParts returns the text.content of every metadataPart under a
// pageHeaderViewModel's contentMetadataViewModel rows, in document order. Modern
// channel and playlist pages carry their subscriber/video/view counts here rather
// than in the older subscriberCountText / videoCountText renderers.
func pageHeaderMetadataParts(ph map[string]any) []string {
	var parts []string
	cmv := mapValue(mapValue(ph, "metadata"), "contentMetadataViewModel")
	if cmv == nil {
		return parts
	}
	for _, row := range arrayValue(cmv["metadataRows"]) {
		rm := mapValue(row, "")
		for _, mp := range arrayValue(rm["metadataParts"]) {
			if txt := mapValue(mapValue(mp, ""), "text"); txt != nil {
				if s := stringValue(txt["content"]); s != "" {
					parts = append(parts, s)
				}
			}
		}
	}
	return parts
}

// pageHeaderAvatarName returns the channel name from the metadata part backed by
// the owner's avatar stack, the reliable owner signal in a pageHeaderViewModel
// (the bare "Playlist"/"5 videos" labels carry no owner).
func pageHeaderAvatarName(ph map[string]any) string {
	cmv := mapValue(mapValue(ph, "metadata"), "contentMetadataViewModel")
	if cmv == nil {
		return ""
	}
	for _, row := range arrayValue(cmv["metadataRows"]) {
		for _, mp := range arrayValue(mapValue(row, "")["metadataParts"]) {
			mpm := mapValue(mp, "")
			if mpm["avatarStack"] == nil {
				continue
			}
			if txt := mapValue(mpm, "text"); txt != nil {
				if s := stringValue(txt["content"]); s != "" {
					return s
				}
			}
		}
	}
	return ""
}

// ParseChannelPage parses a channel HTML page into the channel and whatever
// listing was on it. The record itself is built by ParseChannelRecord; this adds
// the videos and the continuation token, which is what a tab read needs.
func ParseChannelPage(data *PageData, pageURL string) (*Channel, []Video, string, error) {
	ch := ParseChannelRecord(data, pageURL)
	if ch == nil {
		return nil, nil, "", fmt.Errorf("channel metadata not found")
	}
	videos := parseVideosFromTree(data.InitialData)
	for i := range videos {
		if videos[i].ChannelID == "" {
			videos[i].ChannelID = ch.ChannelID
		}
		if videos[i].ChannelTitle == "" {
			videos[i].ChannelTitle = ch.Title
		}
	}
	contToken := extractContinuationToken(data.InitialData)
	return ch, dedupeVideos(videos), contToken, nil
}

// ParseContinuationVideos extracts videos and next continuation token from a /browse continuation.
func ParseContinuationVideos(data map[string]any) ([]Video, string) {
	videos := parseVideosFromTree(data)
	contToken := extractContinuationToken(data)
	return dedupeVideos(videos), contToken
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
			c := parseChannelRenderer(r)
			if c.ChannelID != "" {
				channels = append(channels, c)
				results = append(results, SearchResult{EntityType: EntityChannel, ID: c.ChannelID, Title: c.Title, URL: c.URL})
			}
		}
		if r, ok := m["playlistRenderer"].(map[string]any); ok {
			p := newPlaylist(stringValue(r["playlistId"]), SurfaceInnerTube)
			p.Title = extractText(r["title"])
			p.ChannelTitle = extractText(r["longBylineText"])
			p.ChannelID = ownerChannelID(r)
			p.VideoCountText = extractText(r["videoCountText"])
			p.VideoCount = parseCountText(p.VideoCountText)
			if u := joinURL(endpointURL(r["navigationEndpoint"])); u != "" {
				p.URL = u
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
			c := parseChannelRenderer(r)
			if c.ChannelID != "" {
				channels = append(channels, c)
			}
		}
		if r, ok := m["playlistRenderer"].(map[string]any); ok {
			p := newPlaylist(stringValue(r["playlistId"]), SurfaceInnerTube)
			p.Title = extractText(r["title"])
			p.ChannelTitle = extractText(r["longBylineText"])
			p.ChannelID = ownerChannelID(r)
			p.VideoCountText = extractText(r["videoCountText"])
			p.VideoCount = parseCountText(p.VideoCountText)
			if u := joinURL(endpointURL(r["navigationEndpoint"])); u != "" {
				p.URL = u
			}
			if p.PlaylistID != "" {
				playlists = append(playlists, p)
			}
		}
	})
	contToken := extractContinuationToken(data)
	return dedupeVideos(videos), dedupeChannels(channels), dedupePlaylists(playlists), contToken
}

// reTimestamp matches a "1:23 Title" line and reTimestampTrailing a
// "Title - 1:23" one, for the description fallback in videoparse.go. Both layouts
// are common enough that handling one is handling half of them.
var (
	reTimestamp = regexp.MustCompile(`(?m)^(\d{1,2}:\d{2}(?::\d{2})?)\s+(.+)$`)
	// The title group is lazy on purpose. Greedy, it swallowed the hour off
	// "Hit The Hay - 0:02:20" and reported the title as "Hit The Hay - 0", because
	// the colon inside the timestamp is also a legal separator.
	reTimestampTrailing = regexp.MustCompile(`(?m)^(.*?\S)\s*[-–—:|]\s*(\d{1,2}:\d{2}(?::\d{2})?)\s*$`)
)

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
	mime := stringValue(m["mimeType"])
	f := &VideoFormat{
		VideoID:          videoID,
		ITag:             itag,
		MimeType:         mime,
		Kind:             formatKind(mime, adaptive),
		Container:        formatContainer(mime),
		Codec:            formatCodec(mime),
		Quality:          stringValue(m["quality"]),
		QualityLabel:     stringValue(m["qualityLabel"]),
		Width:            int(int64Value(m["width"])),
		Height:           int(int64Value(m["height"])),
		FPS:              int(int64Value(m["fps"])),
		Bitrate:          int64Value(m["bitrate"]),
		AverageBitrate:   int64Value(m["averageBitrate"]),
		AudioChannels:    int(int64Value(m["audioChannels"])),
		AudioSampleRate:  int(numericString(m["audioSampleRate"])),
		ContentLength:    numericString(m["contentLength"]),
		IsAdaptive:       adaptive,
		AudioQuality:     stringValue(m["audioQuality"]),
		ApproxDurationMS: numericString(m["approxDurationMs"]),
		InitRange:        parseByteRange(m["initRange"]),
		IndexRange:       parseByteRange(m["indexRange"]),
		LastModified:     microTime(numericString(m["lastModified"])),
		URL:              stringValue(m["url"]),
		// True whatever this format is and wherever it came from. It is a fact about
		// googlevideo and not about the format, and it is on the record because a
		// consumer cannot find it out without measuring it. Doc 01 section 8.
		IsThrottledUnranged: true,
	}
	f.ExpiresAt = urlExpiry(f.URL)
	return f
}

// numericString reads the integers YouTube spells as strings: contentLength,
// audioSampleRate, approxDurationMs and lastModified are all quoted in the
// response, and int64Value does not unquote.
func numericString(v any) int64 {
	if n := int64Value(v); n != 0 {
		return n
	}
	n, _ := strconv.ParseInt(stringValue(v), 10, 64)
	return n
}

// parseByteRange reads an initRange or indexRange, whose bounds are strings.
func parseByteRange(v any) *ByteRange {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	start, hasStart := m["start"]
	end, hasEnd := m["end"]
	if !hasStart && !hasEnd {
		return nil
	}
	return &ByteRange{Start: numericString(start), End: numericString(end)}
}

// microTime reads lastModified, which is microseconds since the epoch. Seconds
// would put it in 57 million AD and milliseconds in 57000 AD, so the unit matters.
func microTime(micro int64) time.Time {
	if micro <= 0 {
		return time.Time{}
	}
	return time.UnixMicro(micro)
}

// urlExpiry reads the expire parameter off a stream URL, which is the deadline the
// CDN actually enforces. It is a unix timestamp sitting in plain sight, so a long
// download knows when it will be cut off before it starts rather than at the 403.
func urlExpiry(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return time.Time{}
	}
	if sec, err := strconv.ParseInt(u.Query().Get("expire"), 10, 64); err == nil && sec > 0 {
		return time.Unix(sec, 0)
	}
	// Some googlevideo URLs carry it as a path segment instead: /expire/1786013780/.
	// Same number, same meaning, and missing it would mean the downloader never
	// refreshes on those hosts.
	const marker = "/expire/"
	i := strings.Index(u.Path, marker)
	if i < 0 {
		return time.Time{}
	}
	rest := u.Path[i+len(marker):]
	if end := strings.Index(rest, "/"); end >= 0 {
		rest = rest[:end]
	}
	sec, err := strconv.ParseInt(rest, 10, 64)
	if err != nil || sec <= 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0)
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

// parseVideoRenderer parses the classic videoRenderer and its gridVideoRenderer
// and compactVideoRenderer twins, which carry the same keys.
//
// A lockup is a listing row, not a read of the video, and the envelope says so:
// the view count here is the rounded "1.7B views" text, and missed names the
// fields a full read would add rather than leaving them looking absent by choice.
func parseVideoRenderer(r map[string]any) Video {
	videoID := firstNonEmpty(
		stringValue(r["videoId"]),
		stringValue(mapValue(mapValue(r, "navigationEndpoint"), "watchEndpoint")["videoId"]),
	)
	if videoID == "" {
		return Video{}
	}
	v := *NewVideo(videoID, SurfaceInnerTube)
	v.Title = extractText(r["title"])
	v.Description = extractText(r["descriptionSnippet"])
	v.DurationText = extractText(r["lengthText"])
	v.DurationSeconds = parseDurationSeconds(v.DurationText)
	v.PublishedText = extractText(r["publishedTimeText"])
	v.ChannelTitle = extractText(r["ownerText"])
	v.ChannelID = ownerChannelID(r)
	v.Thumbnails = ParseThumbnails(mapValue(r, "thumbnail")["thumbnails"])
	v.ThumbnailURL = largestThumbnail(v.Thumbnails)
	if txt := extractText(r["viewCountText"]); txt != "" {
		v.ViewCountText = txt
		v.ViewCount = parseCountText(txt)
		v.setVia("view_count", "s2 viewCountText, rounded")
	}
	if state := lockupLiveState(r); state != "" {
		v.LiveState = state
	}
	lockupMisses(&v)
	return v
}

// ownerChannelID reads the channel id off a lockup's owner link. Doc 00 records
// this as a real defect in the old parser: a search result with no channel_id is
// a video with no edge to its uploader, which is most of what the graph is for.
func ownerChannelID(r map[string]any) string {
	for _, key := range []string{"ownerText", "longBylineText", "shortBylineText"} {
		for _, run := range arrayValue(mapValue(r, key)["runs"]) {
			nav := mapValue(mapValue(run, ""), "navigationEndpoint")
			if id := stringValue(mapValue(nav, "browseEndpoint")["browseId"]); strings.HasPrefix(id, "UC") {
				return id
			}
		}
	}
	return ""
}

// lockupLiveState reads the LIVE badge a listing row carries instead of the
// liveBroadcastDetails a player response has.
func lockupLiveState(r map[string]any) string {
	for _, b := range arrayValue(r["badges"]) {
		label := stringValue(mapValue(mapValue(b, ""), "metadataBadgeRenderer")["label"])
		if strings.EqualFold(label, "LIVE") || strings.EqualFold(label, "LIVE NOW") {
			return "live"
		}
	}
	if boolValue(mapValue(r, "viewCountText")["isLive"]) {
		return "live"
	}
	return ""
}

// lockupMisses records what a listing row does not carry, so a Video from
// ytb search and a Video from ytb video are not mistaken for the same read.
func lockupMisses(v *Video) {
	v.miss("listing row: no keywords, category, like count, available countries or playability")
	if v.ViewCount > 0 {
		v.miss("view count is the rounded lockup text, ytb video reads the exact one")
	}
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
		if r, ok := m["shortsLockupViewModel"].(map[string]any); ok {
			v := parseShortsLockupViewModel(r)
			if v.VideoID != "" {
				out = append(out, v)
			}
		}
	})
	return out
}

// parseShortsLockupViewModel parses the shortsLockupViewModel, which is how a
// short is rendered on the Shorts tab and in the UUSH shorts playlist.
//
// The same view model on those two pages is not the same object, and every
// difference is a trap.
//
// entityId is "shorts-shelf-item-GdbjNGtWPe4" on the Shorts tab and the opaque
// hash "A4C99DA633F4D7CD" in the playlist. So the id is read out of the
// reelWatchEndpoint, which both pages carry and which states it outright, and the
// entityId is only trusted when the prefix was really there.
//
// overlayMetadata.secondaryText is "22K views" on the Shorts tab and absent in
// the playlist. What the playlist does carry is accessibilityText, which ends
// "..., 22 thousand views - play Short", and that is where the count comes from
// when the lockup itself does not state one.
func parseShortsLockupViewModel(r map[string]any) Video {
	videoID := reelVideoID(r)
	if videoID == "" {
		if id, ok := strings.CutPrefix(stringValue(r["entityId"]), "shorts-shelf-item-"); ok {
			videoID = id
		}
	}
	if videoID == "" {
		return Video{}
	}
	v := *NewVideo(videoID, SurfaceInnerTube)
	// A shorts lockup is the one listing surface that states the thing is a short
	// by existing: nothing but a short is ever rendered as one.
	v.URL = BaseURL + "/shorts/" + videoID
	v.IsShort = boolPtr(true)
	if om := mapValue(r, "overlayMetadata"); om != nil {
		v.Title = stringValue(mapValue(om, "primaryText")["content"])
		if txt := stringValue(mapValue(om, "secondaryText")["content"]); txt != "" {
			v.ViewCountText = txt
			v.ViewCount = parseCountText(txt)
			v.setVia("view_count", "s2 overlayMetadata.secondaryText, rounded")
		}
	}
	if v.ViewCount == 0 {
		if txt, n := a11yViewCount(stringValue(r["accessibilityText"])); n > 0 {
			v.ViewCountText = txt
			v.ViewCount = n
			v.setVia("view_count", "s2 accessibilityText, rounded and spelled out")
		}
	}
	v.Thumbnails = ParseThumbnails(mapValue(mapValue(mapValue(r, "thumbnailViewModel"), "thumbnailViewModel"), "image")["sources"])
	v.ThumbnailURL = largestThumbnail(v.Thumbnails)
	lockupMisses(&v)
	// A shorts row names nobody. There is no byline, no avatar and no browse
	// endpoint anywhere in the subtree, on the Shorts tab or in a search shelf, so
	// the owner is only known when the surrounding page states it. That is a fact
	// about the shape and it is said rather than left to look like a video with no
	// uploader.
	v.miss("a shorts row carries no owner: no byline, no avatar and no channel link anywhere on it")
	return v
}

// reelVideoID reads the id a shorts lockup navigates to. The endpoint states it
// twice, as a field and as the last segment of the url, and the field is
// preferred because it needs no string surgery.
func reelVideoID(r map[string]any) string {
	cmd := mapValue(mapValue(r, "onTap"), "innertubeCommand")
	if id := stringValue(mapValue(cmd, "reelWatchEndpoint")["videoId"]); id != "" {
		return id
	}
	u := stringValue(mapValue(mapValue(cmd, "commandMetadata"), "webCommandMetadata")["url"])
	if id, ok := strings.CutPrefix(u, "/shorts/"); ok {
		return id
	}
	return ""
}

// a11yViewCountRe pulls the count out of a shorts lockup's accessibility label,
// which reads "<title>, 22 thousand views - play Short". The scale is a word
// rather than the K/M suffix the visible text uses.
var a11yViewCountRe = regexp.MustCompile(`([\d,.]+)\s*(thousand|million|billion)?\s+views`)

// a11yViewCount returns the phrase it matched and the number behind it, so a
// caller can record both what was said and what it read.
func a11yViewCount(label string) (string, int64) {
	m := a11yViewCountRe.FindStringSubmatch(label)
	if m == nil {
		return "", 0
	}
	scale := map[string]float64{"thousand": 1_000, "million": 1_000_000, "billion": 1_000_000_000}[m[2]]
	if scale == 0 {
		return strings.TrimSpace(m[0]), parseCountText(m[1])
	}
	// The number in front of the scale word can be fractional, "1.4 million", so
	// it is parsed as one rather than run through the integer count reader that
	// would drop the .4 and lose four hundred thousand views.
	f, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64)
	if err != nil {
		return "", 0
	}
	return strings.TrimSpace(m[0]), int64(f * scale)
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
			p := newPlaylist(stringValue(r["playlistId"]), SurfaceInnerTube)
			p.Title = extractText(r["title"])
			p.VideoCountText = extractText(r["videoCountText"])
			p.VideoCount = parseCountText(p.VideoCountText)
			if u := joinURL(endpointURL(r["navigationEndpoint"])); u != "" {
				p.URL = u
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

// ParseRelatedShelf reads the secondaryResults shelf as the rows it is.
//
// Each row is a lockup with a title and a byline on it, and reducing the shelf
// to a list of ids throws away twenty video titles and twenty channel ids that
// the response already paid for. RelatedVideos below is the join the store
// holds, derived from these.
func ParseRelatedShelf(root any, videoID string) []Video {
	var out []Video
	seen := map[string]struct{}{}
	for _, v := range dedupeVideos(parseVideosFromTree(root)) {
		if v.VideoID == "" || v.VideoID == videoID {
			continue
		}
		if _, ok := seen[v.VideoID]; ok {
			continue
		}
		seen[v.VideoID] = struct{}{}
		out = append(out, v)
	}
	return out
}

// RelatedVideos turns a shelf into the join rows the store holds. Position is
// the shelf's own order, one based.
func RelatedVideos(videoID string, shelf []Video) []RelatedVideo {
	out := make([]RelatedVideo, 0, len(shelf))
	for i, v := range shelf {
		out = append(out, RelatedVideo{VideoID: videoID, RelatedVideoID: v.VideoID, Position: i + 1})
	}
	return out
}

// ParseContinuationRelatedVideos extracts related videos and next token from a /next continuation.
func ParseContinuationRelatedVideos(data map[string]any, videoID string) ([]Video, string) {
	return ParseRelatedShelf(data, videoID), extractContinuationToken(data)
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

// extractContinuationToken finds the token that pages the list in root.
//
// The search lives in continuation.go, which knows all four token shapes and
// which markers mean "more of this list" rather than "start a different one".
// This used to walk continuationItemRenderer.continuationEndpoint by hand, which
// found one shape of four and, on a channel page, picked between the grid's token
// and the about panel's by map iteration order.
func extractContinuationToken(root any) string {
	return FindContinuationToken(root)
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
	if a.ChannelTitle == "" {
		a.ChannelTitle = b.ChannelTitle
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
