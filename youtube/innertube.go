package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// innertube.go is the verb layer. Each method names one read, picks the client
// that answers it, and hands the body to Client.Call, which does the key, the
// headers, the context, the cache, the rate limit and the refusal check.
//
// Which client answers which verb is not a free choice, and the reasons are
// measured rather than assumed:
//
//   - browse, search and next as WEB, because those return what a person sees.
//   - player as ANDROID_VR then ANDROID, because only the mobile clients return
//     formats with a plain url, and only their caption baseUrl values return
//     bytes.
//   - comments as MWEB, because its comment section is still commentRenderer
//     rather than an entity payload keyed by id.
//   - music as WEB_REMIX against music.youtube.com with its own harvested key.
type InnerTubeClient struct {
	c      *Client
	hl, gl string
}

// NewInnerTube creates an InnerTube client bound to c.
func NewInnerTube(c *Client) *InnerTubeClient {
	return &InnerTubeClient{c: c, hl: c.hl, gl: c.gl}
}

// Search calls /search as WEB.
func (it *InnerTubeClient) Search(ctx context.Context, query string, filters SearchFilters, continuation string) (map[string]any, error) {
	body := map[string]any{"query": query}
	if sp := filters.Encode(); sp != "" {
		body["params"] = sp
	}
	if continuation != "" {
		body["continuation"] = continuation
	}
	return it.c.Call(ctx, ClientWEB(), "search", body, "search "+query)
}

// Browse calls /browse as WEB.
//
// A playlist browseId must carry its VL prefix. Without it YouTube answers 400
// with a 285-byte body that names no field, so the prefix is added by the caller
// that knows it is browsing a playlist and never guessed at here.
func (it *InnerTubeClient) Browse(ctx context.Context, browseID, params, continuation string) (map[string]any, error) {
	body := map[string]any{}
	if browseID != "" {
		body["browseId"] = browseID
	}
	if params != "" {
		body["params"] = params
	}
	if continuation != "" {
		body["continuation"] = continuation
	}
	subject := "browse " + browseID
	if browseID == "" {
		subject = "browse continuation"
	}
	return it.c.Call(ctx, ClientWEB(), "browse", body, subject)
}

// BrowseContinuation pages /browse with only a continuation token.
func (it *InnerTubeClient) BrowseContinuation(ctx context.Context, continuation string) (map[string]any, error) {
	return it.Browse(ctx, "", "", continuation)
}

// Player calls /player as WEB. This is the metadata read: it returns
// videoDetails and microformat and no stream URLs at all.
func (it *InnerTubeClient) Player(ctx context.Context, videoID string) (map[string]any, error) {
	return it.playerAs(ctx, ClientWEB(), videoID)
}

// AndroidPlayer calls /player as ANDROID. This is the read that answers with
// stream URLs and with caption baseUrl values that return bytes.
func (it *InnerTubeClient) AndroidPlayer(ctx context.Context, videoID string) (map[string]any, error) {
	return it.playerAs(ctx, ClientANDROID(), videoID)
}

// IOSPlayer calls /player as IOS, the second opinion on formats.
func (it *InnerTubeClient) IOSPlayer(ctx context.Context, videoID string) (map[string]any, error) {
	return it.playerAs(ctx, ClientIOS(), videoID)
}

// AndroidVRPlayer calls /player as ANDROID_VR. It needs no proof-of-origin
// token, which is why the downloader leads with it.
func (it *InnerTubeClient) AndroidVRPlayer(ctx context.Context, videoID, visitorData string, signatureTimestamp int) (map[string]any, error) {
	// visitorData and signatureTimestamp are ignored. The visitor id comes from
	// the harvest so every call in a run shares one session, and the signature
	// timestamp only ever told the server which base.js would solve a cipher.
	// This client solves none, and the mobile players answer with plain URLs
	// either way. The parameters stay in the signature so the download path reads
	// the same as it did before the cipher went.
	_, _ = visitorData, signatureTimestamp
	return it.playerAs(ctx, ClientANDROIDVR(), videoID)
}

// WebSafariPlayer calls /player as WEB with a Safari user agent.
func (it *InnerTubeClient) WebSafariPlayer(ctx context.Context, videoID, visitorData string, signatureTimestamp int) (map[string]any, error) {
	_, _ = visitorData, signatureTimestamp
	spec := ClientWEB()
	spec.UserAgent = webSafariUA
	return it.playerAs(ctx, spec, videoID)
}

const webSafariUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/15.5 Safari/605.1.15,gzip(gfe)"

// androidVRUA is exported through the ClientSpec now, but the download path
// labels each stream with the UA that produced it, so the constant stays.
var androidVRUA = ClientANDROIDVR().UserAgent

func (it *InnerTubeClient) playerAs(ctx context.Context, spec ClientSpec, videoID string) (map[string]any, error) {
	body := map[string]any{
		"videoId": videoID,
		"playbackContext": map[string]any{
			"contentPlaybackContext": map[string]any{
				"html5Preference": "HTML5_PREF_WANTS",
			},
		},
		// Both of these are assertions the web player makes on the user's behalf
		// when they click through a content warning. Sending them is how an
		// embeddable-but-flagged video returns its formats.
		"contentCheckOk": true,
		"racyCheckOk":    true,
	}
	return it.c.Call(ctx, spec, "player", body, "video "+videoID)
}

// Next calls /next as WEB, for related videos and the comment section token.
func (it *InnerTubeClient) Next(ctx context.Context, videoID, continuation string) (map[string]any, error) {
	body := map[string]any{}
	if videoID != "" {
		body["videoId"] = videoID
	}
	if continuation != "" {
		body["continuation"] = continuation
	}
	return it.c.Call(ctx, ClientWEB(), "next", body, "next "+videoID)
}

// NextMWEB calls /next as MWEB, which returns classic commentRenderer bodies.
func (it *InnerTubeClient) NextMWEB(ctx context.Context, videoID string) (map[string]any, error) {
	return it.c.Call(ctx, ClientMWEB(), "next", map[string]any{"videoId": videoID}, "comments for "+videoID)
}

// CommentContinuation pages comments and replies as MWEB.
func (it *InnerTubeClient) CommentContinuation(ctx context.Context, continuation string) (map[string]any, error) {
	return it.c.Call(ctx, ClientMWEB(), "next", map[string]any{"continuation": continuation}, "comment page")
}

// CommentContinuationWEB pages comments and replies as WEB, where bodies arrive
// as entity payloads. The token comes off a response and is never built here.
func (it *InnerTubeClient) CommentContinuationWEB(ctx context.Context, continuation, visitor string) (map[string]any, error) {
	// visitor is ignored: the harvested visitor id is used for every call in a
	// run, so a token minted under it stays valid.
	_ = visitor
	return it.c.Call(ctx, ClientWEB(), "next", map[string]any{"continuation": continuation}, "comment page")
}

// Community fetches a channel's community tab. params must come from the
// channel's own tab strip, which DiscoverCommunityTabParams reads.
func (it *InnerTubeClient) Community(ctx context.Context, browseID, params, continuation string) (map[string]any, error) {
	return it.Browse(ctx, browseID, params, continuation)
}

// communityTabSlugs are the names the posts tab has gone by. Both are read off
// the response; neither is a params blob.
var communityTabSlugs = []string{"posts", "community"}

// DiscoverCommunityTabParams finds the posts tab's params for a channel.
//
// The params come off the response every time. This used to compare against two
// blobs written down here, and one of them had gone stale: @RickAstleyYT's posts
// tab answers to EgVwb3N0c_IGBAoCSgA%3D and the list held EgVwb3N0c_IGBAoCEgA, so
// the match failed and the tool reported no posts tab on a channel that has one.
//
// The tab is found by its slug, which is field 2 of the params protobuf and is the
// same in every language. A title match never worked: fetched from an address
// YouTube reads as Vietnamese the strip comes back "Trang chu / Video / Shorts /
// Phat truc tiep / Danh sach phat", and even in English the tab titled Home is
// featured and the one titled Live is streams.
func (it *InnerTubeClient) DiscoverCommunityTabParams(ctx context.Context, browseID string) (string, error) {
	data, err := it.Browse(ctx, browseID, "", "")
	if err != nil {
		return "", err
	}
	for _, slug := range communityTabSlugs {
		if tab, ok := FindTab(data, slug); ok {
			return tab.Params, nil
		}
	}
	return "", AssertTab(data, communityTabSlugs[0])
}

// ChannelTab browses one tab of a channel by name, with the params read off the
// channel's own tab strip, and checks afterwards that the response is the tab that
// was asked for.
//
// The check is not paranoia. YouTube does not error on a tab a channel does not
// have, it serves the home tab, and the videos on that page are real videos, so
// nothing downstream can tell that the wrong question was answered.
func (it *InnerTubeClient) ChannelTab(ctx context.Context, browseID, slug string) (map[string]any, error) {
	strip, err := it.Browse(ctx, browseID, "", "")
	if err != nil {
		return nil, err
	}
	tab, ok := FindTab(strip, slug)
	if !ok {
		return nil, AssertTab(strip, slug)
	}
	resp, err := it.Browse(ctx, browseID, tab.Params, "")
	if err != nil {
		return nil, err
	}
	if err := AssertTab(resp, slug); err != nil {
		return nil, err
	}
	return resp, nil
}

// ResolveURL resolves any youtube.com URL to the endpoint that serves it.
//
// This is how a handle becomes a channel id. The response is 1170 bytes against
// 1.9 MB for the channel page, so a read that only needs the id should come
// through here.
func (it *InnerTubeClient) ResolveURL(ctx context.Context, pageURL string) (map[string]any, error) {
	return it.c.Call(ctx, ClientWEB(), "navigation/resolve_url", map[string]any{"url": pageURL}, "resolve "+pageURL)
}

// ResolveHashtag resolves a hashtag to its browseId and params.
func (it *InnerTubeClient) ResolveHashtag(ctx context.Context, hashtag string) (string, string, error) {
	tag := strings.TrimPrefix(hashtag, "#")
	data, err := it.ResolveURL(ctx, "https://www.youtube.com/hashtag/"+strings.ToLower(tag))
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

// MusicSearch calls music.youtube.com /search as WEB_REMIX.
func (it *InnerTubeClient) MusicSearch(ctx context.Context, query, params, continuation string) (map[string]any, error) {
	body := map[string]any{"query": query}
	if params != "" {
		body["params"] = params
	}
	if continuation != "" {
		body["continuation"] = continuation
	}
	return it.c.Call(ctx, ClientWEBREMIX(), "search", body, "music search "+query)
}

// MusicBrowse calls music.youtube.com /browse as WEB_REMIX.
func (it *InnerTubeClient) MusicBrowse(ctx context.Context, browseID, params, continuation string) (map[string]any, error) {
	body := map[string]any{}
	if browseID != "" {
		body["browseId"] = browseID
	}
	if params != "" {
		body["params"] = params
	}
	if continuation != "" {
		body["continuation"] = continuation
	}
	return it.c.Call(ctx, ClientWEBREMIX(), "browse", body, "music browse "+browseID)
}

// MusicPlayer calls music.youtube.com /player for a track's details.
func (it *InnerTubeClient) MusicPlayer(ctx context.Context, videoID string) (map[string]any, error) {
	return it.c.Call(ctx, ClientWEBREMIX(), "player", map[string]any{"videoId": videoID}, "music track "+videoID)
}

// MusicNext calls music.youtube.com /next, for a watch queue or a lyrics tab.
func (it *InnerTubeClient) MusicNext(ctx context.Context, body map[string]any, subject string) (map[string]any, error) {
	return it.c.Call(ctx, ClientWEBREMIX(), "next", body, subject)
}

// Suggest fetches autocomplete suggestions from the public suggestqueries
// endpoint. This is JSONP, not InnerTube: no key, no POST, no context.
func (it *InnerTubeClient) Suggest(ctx context.Context, input string) ([]string, error) {
	url := "https://suggestqueries-clients6.youtube.com/complete/search?client=youtube&ds=yt&hl=" +
		it.hl + "&gl=" + it.gl + "&q=" + strings.ReplaceAll(input, " ", "+")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", uaDesktopChrome)
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
