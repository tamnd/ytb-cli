package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	innertubeURL        = "https://www.youtube.com/youtubei/v1"
	musicInnertubeURL   = "https://music.youtube.com/youtubei/v1"
	defaultClientVer    = "2.20260114.08.00"
	musicClientVer      = "1.20260114.03.00"
	webClientName       = "1"
	androidVRClientName = "28"
)

// InnerTubeClient calls YouTube's internal InnerTube API. No API key or auth required.
// It routes every POST through the parent Client so requests are rate-limited and retried.
type InnerTubeClient struct {
	c             *Client
	clientVersion string
	hl, gl        string
}

// NewInnerTube creates an InnerTube client bound to c.
func NewInnerTube(c *Client) *InnerTubeClient {
	return &InnerTubeClient{c: c, clientVersion: defaultClientVer, hl: c.hl, gl: c.gl}
}

func (it *InnerTubeClient) context() map[string]any {
	return map[string]any{
		"client": map[string]any{
			"clientName":    "WEB",
			"clientVersion": it.clientVersion,
			"hl":            it.hl,
			"gl":            it.gl,
		},
	}
}

func (it *InnerTubeClient) mwebContext() map[string]any {
	return map[string]any{
		"client": map[string]any{
			"clientName":    "MWEB",
			"clientVersion": it.clientVersion,
			"hl":            it.hl,
			"gl":            it.gl,
		},
	}
}

func (it *InnerTubeClient) musicContext() map[string]any {
	return map[string]any{
		"client": map[string]any{
			"clientName":    "WEB_REMIX",
			"clientVersion": musicClientVer,
			"hl":            it.hl,
			"gl":            it.gl,
		},
	}
}

// Search calls /search.
func (it *InnerTubeClient) Search(ctx context.Context, query string, filters SearchFilters, continuation string) (map[string]any, error) {
	body := map[string]any{"context": it.context(), "query": query}
	if sp := filters.Encode(); sp != "" {
		body["params"] = sp
	}
	if continuation != "" {
		body["continuation"] = continuation
	}
	return it.c.postJSON(ctx, innertubeURL+"/search", body)
}

// Browse calls /browse.
func (it *InnerTubeClient) Browse(ctx context.Context, browseID, params, continuation string) (map[string]any, error) {
	body := map[string]any{"context": it.context()}
	if browseID != "" {
		body["browseId"] = browseID
	}
	if params != "" {
		body["params"] = params
	}
	if continuation != "" {
		body["continuation"] = continuation
	}
	return it.c.postJSON(ctx, innertubeURL+"/browse", body)
}

// BrowseContinuation pages /browse with only a continuation token.
func (it *InnerTubeClient) BrowseContinuation(ctx context.Context, continuation string) (map[string]any, error) {
	return it.c.postJSON(ctx, innertubeURL+"/browse", map[string]any{
		"context": it.context(), "continuation": continuation,
	})
}

// Player calls /player.
func (it *InnerTubeClient) Player(ctx context.Context, videoID string) (map[string]any, error) {
	return it.player(ctx, videoID, it.context(), webClientName, it.c.userAgents[0], 0)
}

const webSafariUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/15.5 Safari/605.1.15,gzip(gfe)"

func (it *InnerTubeClient) webSafariContext() map[string]any {
	return map[string]any{
		"client": map[string]any{
			"clientName":    "WEB",
			"clientVersion": defaultClientVer,
			"userAgent":     webSafariUA,
			"hl":            it.hl,
			"gl":            it.gl,
		},
	}
}

// WebSafariPlayer calls /player with yt-dlp's WEB Safari fallback client. This
// client often retains progressive/HLS entries when the plain WEB player response
// is SABR-only.
func (it *InnerTubeClient) WebSafariPlayer(ctx context.Context, videoID, visitorData string, signatureTimestamp int) (map[string]any, error) {
	ytctx := it.webSafariContext()
	if visitorData != "" {
		if client, ok := ytctx["client"].(map[string]any); ok {
			client["visitorData"] = visitorData
		}
	}
	return it.player(ctx, videoID, ytctx, webClientName, webSafariUA, signatureTimestamp)
}

const androidVRClientVer = "1.65.10"

// androidVRUA is the Oculus YouTube VR app user agent. The ANDROID_VR client is
// the one anonymous context that still returns complete, directly-fetchable
// stream URLs (no signature cipher) and needs no proof-of-origin token, so the
// native downloader leads with it. Its UA must match or YouTube strips formats.
const androidVRUA = "com.google.android.apps.youtube.vr.oculus/" + androidVRClientVer +
	" (Linux; U; Android 12L; eureka-user Build/SQ3A.220605.009.A1) gzip"

// AndroidVRPlayer calls /player with the ANDROID_VR client. Unlike the WEB
// /player call, the formats it returns carry plain `url` fields rather than a
// signatureCipher, so only the `n` throttling parameter still needs solving.
func (it *InnerTubeClient) AndroidVRPlayer(ctx context.Context, videoID, visitorData string, signatureTimestamp int) (map[string]any, error) {
	ytctx := map[string]any{
		"client": map[string]any{
			"clientName":        "ANDROID_VR",
			"clientVersion":     androidVRClientVer,
			"deviceMake":        "Oculus",
			"deviceModel":       "Quest 3",
			"androidSdkVersion": 32,
			"userAgent":         androidVRUA,
			"osName":            "Android",
			"osVersion":         "12L",
			"hl":                it.hl,
			"timeZone":          "UTC",
			"utcOffsetMinutes":  0,
		},
	}
	if visitorData != "" {
		if client, ok := ytctx["client"].(map[string]any); ok {
			client["visitorData"] = visitorData
		}
	}
	return it.player(ctx, videoID, ytctx, androidVRClientName, androidVRUA, signatureTimestamp)
}

func (it *InnerTubeClient) player(ctx context.Context, videoID string, ytctx map[string]any, clientName, ua string, signatureTimestamp int) (map[string]any, error) {
	clientVersion := defaultClientVer
	if client, ok := ytctx["client"].(map[string]any); ok {
		if v := stringValue(client["clientVersion"]); v != "" {
			clientVersion = v
		}
	}
	contentPlaybackContext := map[string]any{
		"html5Preference": "HTML5_PREF_WANTS",
	}
	if signatureTimestamp > 0 {
		contentPlaybackContext["signatureTimestamp"] = signatureTimestamp
	}
	body := map[string]any{
		"context": ytctx,
		"videoId": videoID,
		"playbackContext": map[string]any{
			"contentPlaybackContext": contentPlaybackContext,
		},
		"contentCheckOk": true,
		"racyCheckOk":    true,
	}
	headers := map[string]string{
		"User-Agent":               ua,
		"Origin":                   BaseURL,
		"X-YouTube-Client-Name":    clientName,
		"X-YouTube-Client-Version": clientVersion,
	}
	if client, ok := ytctx["client"].(map[string]any); ok {
		headers["X-Goog-Visitor-Id"] = stringValue(client["visitorData"])
	}
	return it.c.postJSONHeaders(ctx, innertubeURL+"/player", body, headers)
}

// Next calls /next.
func (it *InnerTubeClient) Next(ctx context.Context, videoID, continuation string) (map[string]any, error) {
	body := map[string]any{"context": it.context(), "videoId": videoID}
	if continuation != "" {
		body["continuation"] = continuation
	}
	return it.c.postJSON(ctx, innertubeURL+"/next", body)
}

// NextMWEB calls /next with the MWEB client (returns classic commentRenderer).
func (it *InnerTubeClient) NextMWEB(ctx context.Context, videoID string) (map[string]any, error) {
	return it.c.postJSON(ctx, innertubeURL+"/next", map[string]any{
		"context": it.mwebContext(), "videoId": videoID,
	})
}

// CommentContinuation pages comments/replies via the MWEB client.
func (it *InnerTubeClient) CommentContinuation(ctx context.Context, continuation string) (map[string]any, error) {
	return it.c.postJSON(ctx, innertubeURL+"/next", map[string]any{
		"context": it.mwebContext(), "continuation": continuation,
	})
}

// webContextWithVisitor returns the WEB context, carrying visitorData when known.
// Comment continuations minted by the watch page expect the same visitor session.
func (it *InnerTubeClient) webContextWithVisitor(visitor string) map[string]any {
	ctx := it.context()
	if visitor != "" {
		if client, ok := ctx["client"].(map[string]any); ok {
			client["visitorData"] = visitor
		}
	}
	return ctx
}

// CommentContinuationWEB pages comments/replies via the WEB client. Modern
// YouTube serves comment bodies as entity payloads under this client; the token
// comes from the watch page's ytInitialData and is bound to its visitor session.
func (it *InnerTubeClient) CommentContinuationWEB(ctx context.Context, continuation, visitor string) (map[string]any, error) {
	return it.c.postJSON(ctx, innertubeURL+"/next", map[string]any{
		"context": it.webContextWithVisitor(visitor), "continuation": continuation,
	})
}

// Community fetches a channel's community/posts tab.
func (it *InnerTubeClient) Community(ctx context.Context, browseID, params, continuation string) (map[string]any, error) {
	if params == "" {
		params = "Egljb21tdW5pdHk%3D"
	}
	return it.Browse(ctx, browseID, params, continuation)
}

// DiscoverCommunityTabParams finds the community/posts tab params for a channel.
func (it *InnerTubeClient) DiscoverCommunityTabParams(ctx context.Context, browseID string) (string, error) {
	data, err := it.Browse(ctx, browseID, "", "")
	if err != nil {
		return "", err
	}
	var params string
	walkJSON(data, func(m map[string]any) {
		if params != "" {
			return
		}
		tr, ok := m["tabRenderer"].(map[string]any)
		if !ok {
			return
		}
		tl := strings.ToLower(stringValue(tr["title"]))
		if tl != "community" && tl != "posts" {
			return
		}
		ep, ok := tr["endpoint"].(map[string]any)
		if !ok {
			return
		}
		be, ok := ep["browseEndpoint"].(map[string]any)
		if !ok {
			return
		}
		params = stringValue(be["params"])
	})
	return params, nil
}

// ResolveHashtag resolves a hashtag to its browseId and params.
func (it *InnerTubeClient) ResolveHashtag(ctx context.Context, hashtag string) (string, string, error) {
	tag := strings.TrimPrefix(hashtag, "#")
	hashURL := "https://www.youtube.com/hashtag/" + strings.ToLower(tag)
	data, err := it.c.postJSON(ctx, innertubeURL+"/navigation/resolve_url", map[string]any{
		"context": it.context(), "url": hashURL,
	})
	if err != nil {
		return "", "", err
	}
	var browseID, params string
	walkJSON(data, func(m map[string]any) {
		if browseID != "" {
			return
		}
		if be, ok := m["browseEndpoint"].(map[string]any); ok {
			if id := stringValue(be["browseId"]); id != "" {
				browseID = id
				params = stringValue(be["params"])
			}
		}
	})
	if browseID == "" {
		return "", "", fmt.Errorf("hashtag %q: could not resolve browseId", hashtag)
	}
	return browseID, params, nil
}

// MusicSearch calls music.youtube.com /search with the WEB_REMIX client.
func (it *InnerTubeClient) MusicSearch(ctx context.Context, query, params, continuation string) (map[string]any, error) {
	body := map[string]any{"context": it.musicContext(), "query": query}
	if params != "" {
		body["params"] = params
	}
	if continuation != "" {
		body["continuation"] = continuation
	}
	return it.c.postJSON(ctx, musicInnertubeURL+"/search", body)
}

// MusicBrowse calls music.youtube.com /browse with the WEB_REMIX client.
func (it *InnerTubeClient) MusicBrowse(ctx context.Context, browseID, params, continuation string) (map[string]any, error) {
	body := map[string]any{"context": it.musicContext()}
	if browseID != "" {
		body["browseId"] = browseID
	}
	if params != "" {
		body["params"] = params
	}
	if continuation != "" {
		body["continuation"] = continuation
	}
	return it.c.postJSON(ctx, musicInnertubeURL+"/browse", body)
}

// MusicPlayer calls music.youtube.com /player for a song's details.
func (it *InnerTubeClient) MusicPlayer(ctx context.Context, videoID string) (map[string]any, error) {
	return it.c.postJSON(ctx, musicInnertubeURL+"/player", map[string]any{
		"context": it.musicContext(), "videoId": videoID,
	})
}

// Suggest fetches autocomplete suggestions from the public suggestqueries endpoint.
func (it *InnerTubeClient) Suggest(ctx context.Context, input string) ([]string, error) {
	url := "https://suggestqueries-clients6.youtube.com/complete/search?client=youtube&ds=yt&q=" +
		strings.ReplaceAll(input, " ", "+")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := it.c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseSuggestionsJSON(string(body)), nil
}

func parseSuggestionsJSON(s string) []string {
	const prefix = "window.google.ac.h("
	if idx := strings.Index(s, prefix); idx >= 0 {
		s = s[idx+len(prefix):]
		if last := strings.LastIndex(s, ")"); last >= 0 {
			s = s[:last]
		}
	}
	var data []any
	if err := json.Unmarshal([]byte(s), &data); err != nil {
		return nil
	}
	if len(data) < 2 {
		return nil
	}
	items, ok := data[1].([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range items {
		arr, ok := item.([]any)
		if !ok || len(arr) == 0 {
			continue
		}
		if sug, ok := arr[0].(string); ok {
			out = append(out, sug)
		}
	}
	return out
}
