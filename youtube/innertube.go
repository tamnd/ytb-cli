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
	innertubeURL      = "https://www.youtube.com/youtubei/v1"
	musicInnertubeURL = "https://music.youtube.com/youtubei/v1"
	defaultClientVer  = "2.20250401.00.00"
	musicClientVer    = "1.20250401.01.00"
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
	return it.c.postJSON(ctx, innertubeURL+"/player", map[string]any{
		"context": it.context(), "videoId": videoID,
	})
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
