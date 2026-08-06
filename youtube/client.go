package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/tamnd/ytb-cli/pkg/ytid"
)

// Client is the rate-limited HTTP front end for YouTube web + InnerTube.
type Client struct {
	http       *http.Client
	userAgents []string
	delay      time.Duration
	retries    int
	hl, gl     string

	mu      sync.Mutex
	lastReq time.Time

	cfgCache  *ytcfgCache
	cache     *Cache
	onRequest func(method, url string)
	onRead    func(Read)
}

// SetOnRequest installs a hook called once for every request that actually goes
// out. Doc 04 section 3.3: a budget is in requests, and it is counted rather
// than estimated.
//
// A cache hit never reaches the hook, so it is not a request and does not count,
// which is why a second walk over the same seeds gets further on the same
// budget. A retry does reach it, because a retry is a request the site had to
// answer.
//
// The hook is called from whatever goroutine made the request, so it must be
// safe to call concurrently, and it should be installed before the first read.
func (c *Client) SetOnRequest(fn func(method, url string)) { c.onRequest = fn }

// noteRequest tells the hook a request is going out. It is called at the point
// the request is built, which is after the cache was consulted and after the
// rate limiter let it through.
func (c *Client) noteRequest(method, url string) {
	if c.onRequest != nil {
		c.onRequest(method, url)
	}
}

// SetCache attaches a disk cache. Every read goes through it keyed by URL plus
// claimed client, so a WEB player response is never served to a caption read.
func (c *Client) SetCache(cache *Cache) { c.cache = cache }

// Cache returns the attached cache, which may be nil.
func (c *Client) Cache() *Cache { return c.cache }

// NewClient builds a Client from cfg.
func NewClient(cfg Config) *Client {
	if cfg.Workers <= 0 {
		cfg.Workers = DefaultWorkers
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.HL == "" {
		cfg.HL = "en"
	}
	if cfg.GL == "" {
		cfg.GL = "US"
	}
	jar, _ := cookiejar.New(nil)
	return &Client{
		http: &http.Client{
			Timeout: cfg.Timeout,
			Jar:     jar,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxConnsPerHost:     cfg.Workers + 2,
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
			},
		},
		userAgents: userAgents,
		delay:      cfg.Delay,
		retries:    cfg.Retries,
		hl:         cfg.HL,
		gl:         cfg.GL,
		cfgCache:   newYTCfgCache(),
	}
}

// HTTP exposes the underlying *http.Client (used by the InnerTube client and yt-dlp probe).
func (c *Client) HTTP() *http.Client { return c.http }

// HL/GL expose the configured language/country for the InnerTube context.
func (c *Client) HL() string { return c.hl }
func (c *Client) GL() string { return c.gl }

// Fetch GETs url with browser-like headers and the polite rate limit, retrying
// transient 429/5xx responses with backoff.
//
// HTML reads go through the cache too, not only the InnerTube POSTs. A watch page
// is 1.9 MB and a channel page the same, so they are the most expensive thing
// this tool fetches and the most worth not fetching twice. They are keyed under
// the WEB client, because that is who asked.
func (c *Client) Fetch(ctx context.Context, url string) ([]byte, int, error) {
	key := CacheKey{
		Method: http.MethodGet,
		URL:    c.localise(url),
		Client: "WEB/html",
	}
	if status, body, ok := c.cache.Get(key); ok && status == 200 {
		return body, status, nil
	}

	var lastErr error
	attempts := c.retries + 1
	if attempts < 1 {
		attempts = 1
	}
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt) * 500 * time.Millisecond
			select {
			case <-ctx.Done():
				return nil, 0, ctx.Err()
			case <-time.After(backoff):
			}
		}
		c.rateLimit()
		c.noteRequest(http.MethodGet, c.localise(url))
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.localise(url), nil)
		if err != nil {
			return nil, 0, err
		}
		req.Header.Set("User-Agent", c.userAgents[rand.Intn(len(c.userAgents))])
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		c.setLanguageHeaders(req)
		read := Read{
			Method:  http.MethodGet,
			URL:     c.localise(url),
			Surface: surfaceForURL(url, ""),
			Headers: sentHeaders(req.Header),
			At:      time.Now(),
		}
		resp, err := c.http.Do(req)
		if err != nil {
			read.Error = err.Error()
			c.noteRead(read)
			lastErr = err
			continue
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		read.Status = resp.StatusCode
		read.Bytes = len(body)
		read.Body = body
		if err != nil {
			read.Error = err.Error()
			c.noteRead(read)
			lastErr = err
			continue
		}
		c.noteRead(read)
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
			continue
		}
		if resp.StatusCode == 200 {
			c.cache.Put(key, resp.StatusCode, body)
		}
		return body, resp.StatusCode, nil
	}
	return nil, 0, lastErr
}

// FetchHTML fetches and parses an HTML document.
func (c *Client) FetchHTML(ctx context.Context, url string) (*goquery.Document, int, error) {
	body, code, err := c.Fetch(ctx, url)
	if err != nil {
		return nil, code, err
	}
	if code == 404 {
		return nil, code, nil
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, code, err
	}
	return doc, code, nil
}

// FetchPageData fetches an HTML page and extracts the embedded JSON bootstrap blobs.
func (c *Client) FetchPageData(ctx context.Context, url string) (*PageData, int, error) {
	body, code, err := c.Fetch(ctx, url)
	if err != nil {
		return nil, code, err
	}
	if code == 404 {
		return nil, code, nil
	}
	html := string(body)
	data := &PageData{
		HTML:          html,
		InitialData:   parseJSONAny(extractJSONVar(html, "var ytInitialData = ")),
		PlayerResp:    parseJSONAny(extractJSONVar(html, "var ytInitialPlayerResponse = ")),
		YTCFG:         parseJSONObject(extractJSONCall(html, "ytcfg.set(")),
		APIKey:        extractQuotedConfig(html, "INNERTUBE_API_KEY"),
		ClientVersion: extractQuotedConfig(html, "INNERTUBE_CLIENT_VERSION"),
		VisitorData:   extractQuotedConfig(html, "VISITOR_DATA"),
	}
	if data.APIKey == "" && data.YTCFG != nil {
		data.APIKey = stringValue(data.YTCFG["INNERTUBE_API_KEY"])
	}
	if data.ClientVersion == "" && data.YTCFG != nil {
		data.ClientVersion = stringValue(data.YTCFG["INNERTUBE_CLIENT_VERSION"])
	}
	if data.VisitorData == "" && data.YTCFG != nil {
		data.VisitorData = stringValue(data.YTCFG["VISITOR_DATA"])
	}
	return data, code, nil
}

// FetchTimedText fetches a caption track's timed-text XML and returns its raw bytes.
func (c *Client) FetchTimedText(ctx context.Context, url string) ([]byte, error) {
	body, code, err := c.Fetch(ctx, url)
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("caption track returned HTTP %d", code)
	}
	return body, nil
}

// ErrChannelNotFound is the answer when nothing lives at an address that looks
// like a channel. Both paths have to come up empty before it is returned, and it
// is a real answer rather than a failure: /c/Vsauce and /c/MrBeast6000 are both
// 404, because neither channel ever took that vanity path, while /c/veritasium is
// UCHnyfMqiRRG1u-2MsSQLbXA. The name in a legacy URL says nothing about whether
// the URL exists.
var ErrChannelNotFound = errors.New("no channel at that address")

// ErrPlaylistNotFound is the answer when a browse of VL<id> comes back with
// neither header shape on it and no alert saying why. A deleted playlist and a
// fabricated id both land here.
var ErrPlaylistNotFound = errors.New("no playlist at that address")

// ResolveChannelID resolves a handle, vanity name, or URL to a UC-style channel ID.
// A UC... input is returned unchanged.
//
// The resolution goes through navigation/resolve_url, which answered in 1577 bytes
// when this was measured against 1.9 MB for the channel page, and the answer is
// cached like every other InnerTube call, so a handle costs one small request on the
// first run and nothing after that. It handles all three alias forms: @handle,
// /user/name and /c/name. The channel page read is still here as the fallback,
// because resolve_url answers about the site's own routes and a URL that is not one
// of those is better read than refused.
func (c *Client) ResolveChannelID(ctx context.Context, input string) (string, error) {
	if id := ytid.Classify(input).ChannelID; id != "" {
		return id, nil
	}
	if id, err := c.resolveChannelIDByURL(ctx, input); err == nil && id != "" {
		return id, nil
	}
	channelURL := NormalizeChannelURL(input)
	data, _, err := c.FetchPageData(ctx, channelURL)
	if err != nil {
		return "", fmt.Errorf("resolve channel %q: %w", input, err)
	}
	// Nothing came back, which is what YouTube's 404 page looks like from here after
	// resolve_url has already said the address routes nowhere. That is not found
	// rather than broken, and the exit code has to say so.
	if data == nil || data.InitialData == nil {
		return "", fmt.Errorf("resolve channel %q: %w", input, ErrChannelNotFound)
	}
	id, _ := data.InitialData.(map[string]any)
	if id == nil {
		return "", fmt.Errorf("resolve channel %q: %w", input, ErrChannelNotFound)
	}
	var channelID string
	walkJSON(id, func(m map[string]any) {
		if channelID != "" {
			return
		}
		if cid := stringValue(m["channelId"]); strings.HasPrefix(cid, "UC") {
			channelID = cid
		}
		if cid := stringValue(m["browseId"]); strings.HasPrefix(cid, "UC") {
			channelID = cid
		}
	})
	if channelID == "" {
		return "", fmt.Errorf("resolve channel %q: %w", input, ErrChannelNotFound)
	}
	return channelID, nil
}

// resolveChannelIDByURL asks navigation/resolve_url what a channel URL routes to.
// The answer is an endpoint, and the browseId on it is the channel id.
func (c *Client) resolveChannelIDByURL(ctx context.Context, input string) (string, error) {
	// NormalizeChannelURL ends every URL with /videos, which is a tab rather than
	// the channel, and resolve_url answers about the address it is given.
	pageURL := strings.TrimSuffix(NormalizeChannelURL(input), "/videos")
	resp, err := NewInnerTube(c).ResolveURL(ctx, pageURL)
	if err != nil {
		return "", err
	}
	var channelID string
	walkJSON(resp, func(m map[string]any) {
		if channelID != "" {
			return
		}
		be, ok := m["browseEndpoint"].(map[string]any)
		if !ok {
			return
		}
		if id := stringValue(be["browseId"]); ytid.IsChannel(id) {
			channelID = id
		}
	})
	if channelID == "" {
		return "", fmt.Errorf("resolve %s: the endpoint carries no channel id", pageURL)
	}
	return channelID, nil
}

func (c *Client) rateLimit() {
	if c.delay <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if since := time.Since(c.lastReq); since < c.delay {
		time.Sleep(c.delay - since)
	}
	c.lastReq = time.Now()
}

func parseJSONAny(raw string) any {
	if raw == "" {
		return nil
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil
	}
	return v
}

func parseJSONObject(raw string) map[string]any {
	if raw == "" {
		return nil
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil
	}
	return v
}

// postJSON is a helper for InnerTube and music POSTs.
func (c *Client) postJSON(ctx context.Context, url string, body map[string]any) (map[string]any, error) {
	return c.postJSONUA(ctx, url, body, "")
}

func (c *Client) postJSONHeaders(ctx context.Context, url string, body map[string]any, headers map[string]string) (map[string]any, error) {
	return c.postJSONWithHeaders(ctx, url, body, headers)
}

// postJSONUA is postJSON with an explicit User-Agent. A non-browser client such
// as ANDROID_VR must send its matching app UA to receive complete, token-free
// streaming data, so the cipher-free download path overrides the default here.
func (c *Client) postJSONUA(ctx context.Context, url string, body map[string]any, ua string) (map[string]any, error) {
	headers := map[string]string{}
	if ua != "" {
		headers["User-Agent"] = ua
	}
	return c.postJSONWithHeaders(ctx, url, body, headers)
}

func (c *Client) postJSONWithHeaders(ctx context.Context, url string, body map[string]any, headers map[string]string) (map[string]any, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	var lastErr error
	attempts := c.retries + 1
	if attempts < 1 {
		attempts = 1
	}
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 500 * time.Millisecond):
			}
		}
		c.rateLimit()
		c.noteRequest(http.MethodPost, url)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		if headers["User-Agent"] == "" {
			req.Header.Set("User-Agent", c.userAgents[0])
		}
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		for k, v := range headers {
			if v != "" {
				req.Header.Set(k, v)
			}
		}
		req.AddCookie(&http.Cookie{Name: "CONSENT", Value: "YES+"})
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		data, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("POST %s: HTTP %d", url, resp.StatusCode)
			continue
		}
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("POST %s: HTTP %d", url, resp.StatusCode)
		}
		var result map[string]any
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, fmt.Errorf("POST %s: invalid JSON: %w", url, err)
		}
		return result, nil
	}
	return nil, lastErr
}
