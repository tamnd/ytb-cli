package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// Client is the rate-limited HTTP front end for YouTube web + InnerTube.
type Client struct {
	http       *http.Client
	userAgents []string
	delay      time.Duration
	retries    int
	hl, gl     string
	lastReq    time.Time
}

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
	return &Client{
		http: &http.Client{
			Timeout: cfg.Timeout,
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
	}
}

// HTTP exposes the underlying *http.Client (used by the InnerTube client and yt-dlp probe).
func (c *Client) HTTP() *http.Client { return c.http }

// HL/GL expose the configured language/country for the InnerTube context.
func (c *Client) HL() string { return c.hl }
func (c *Client) GL() string { return c.gl }

// Fetch GETs url with browser-like headers and the polite rate limit, retrying
// transient 429/5xx responses with backoff.
func (c *Client) Fetch(ctx context.Context, url string) ([]byte, int, error) {
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
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, 0, err
		}
		req.Header.Set("User-Agent", c.userAgents[rand.Intn(len(c.userAgents))])
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.AddCookie(&http.Cookie{Name: "CONSENT", Value: "YES+"})
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
			continue
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

// ResolveChannelID resolves a handle, vanity name, or URL to a UC-style channel ID.
// A UC... input is returned unchanged.
func (c *Client) ResolveChannelID(ctx context.Context, input string) (string, error) {
	if strings.HasPrefix(input, "UC") && !strings.Contains(input, "/") {
		return input, nil
	}
	channelURL := NormalizeChannelURL(input)
	data, _, err := c.FetchPageData(ctx, channelURL)
	if err != nil {
		return "", fmt.Errorf("resolve channel %q: %w", input, err)
	}
	if data == nil || data.InitialData == nil {
		return "", fmt.Errorf("resolve channel %q: empty page data", input)
	}
	id, _ := data.InitialData.(map[string]any)
	if id == nil {
		return "", fmt.Errorf("resolve channel %q: no initial data", input)
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
		return "", fmt.Errorf("resolve channel %q: no channel ID found", input)
	}
	return channelID, nil
}

func (c *Client) rateLimit() {
	if c.delay <= 0 {
		return
	}
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
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", c.userAgents[0])
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
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
