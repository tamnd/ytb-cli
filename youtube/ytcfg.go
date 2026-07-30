package youtube

import (
	"context"
	"fmt"
	"sync"
)

// ytcfg.go harvests the InnerTube API key rather than shipping it.
//
// Every YouTube HTML page calls ytcfg.set({...}) inline, and that object carries
// INNERTUBE_API_KEY, INNERTUBE_CLIENT_VERSION and VISITOR_DATA. The key is not a
// credential: it identifies the web app, it is the same for every visitor, and
// it is served to anyone who loads youtube.com. But it does rotate, and a
// hardcoded one is a tool that breaks on a Tuesday for no reason the user can
// see.
//
// So we read it off the first page of a run and keep it for the process. A
// policy test fails the build on any string literal matching the key shape, so
// this stays the only way a key enters the program.
//
// music.youtube.com serves a different key from a different ytcfg, so it gets
// its own harvest under its own host.
type ytcfg struct {
	APIKey        string
	ClientVersion string
	VisitorData   string
}

// ytcfgCache holds one harvest per host for the life of the process.
type ytcfgCache struct {
	mu   sync.Mutex
	byID map[string]*ytcfg
}

func newYTCfgCache() *ytcfgCache {
	return &ytcfgCache{byID: map[string]*ytcfg{}}
}

// harvestPages names the cheapest page per host that still carries a full
// ytcfg. The www embed page is 40 KB against 1.9 MB for a watch page and
// carries the same key.
var harvestPages = map[string]string{
	wwwHost:   "https://www.youtube.com/embed/dQw4w9WgXcQ",
	musicHost: "https://music.youtube.com/",
}

// ytcfgFor returns the harvested config for a host, fetching it once.
func (c *Client) ytcfgFor(ctx context.Context, host string) (*ytcfg, error) {
	c.cfgCache.mu.Lock()
	if cfg, ok := c.cfgCache.byID[host]; ok {
		c.cfgCache.mu.Unlock()
		return cfg, nil
	}
	c.cfgCache.mu.Unlock()

	page, ok := harvestPages[host]
	if !ok {
		return nil, fmt.Errorf("no ytcfg harvest page known for host %q", host)
	}
	body, code, err := c.Fetch(ctx, page)
	if err != nil {
		return nil, fmt.Errorf("harvest ytcfg from %s: %w", page, err)
	}
	if code != 200 {
		return nil, fmt.Errorf("harvest ytcfg from %s: HTTP %d", page, code)
	}
	cfg := parseYTCfg(string(body))
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("harvest ytcfg from %s: no INNERTUBE_API_KEY on the page", page)
	}

	c.cfgCache.mu.Lock()
	defer c.cfgCache.mu.Unlock()
	// Another goroutine may have harvested while this one was fetching. Keep
	// whichever landed first so every caller in a run sees one visitor id.
	if existing, ok := c.cfgCache.byID[host]; ok {
		return existing, nil
	}
	c.cfgCache.byID[host] = cfg
	return cfg, nil
}

// SeedYTCfg records a harvest taken from a page the caller already had, so a
// read that fetched a watch page does not then fetch an embed page for the key
// that page was already carrying.
func (c *Client) SeedYTCfg(host string, data *PageData) {
	if data == nil || data.APIKey == "" {
		return
	}
	c.cfgCache.mu.Lock()
	defer c.cfgCache.mu.Unlock()
	if _, ok := c.cfgCache.byID[host]; ok {
		return
	}
	c.cfgCache.byID[host] = &ytcfg{
		APIKey:        data.APIKey,
		ClientVersion: data.ClientVersion,
		VisitorData:   data.VisitorData,
	}
}

// parseYTCfg pulls the three fields we need out of a page's inline config. It
// reads the ytcfg.set() object when it can and falls back to the bare quoted
// assignments, because not every surface spells it the same way: the embed page
// and the music page differ here.
func parseYTCfg(html string) *ytcfg {
	cfg := &ytcfg{
		APIKey:        extractQuotedConfig(html, "INNERTUBE_API_KEY"),
		ClientVersion: extractQuotedConfig(html, "INNERTUBE_CLIENT_VERSION"),
		VisitorData:   extractQuotedConfig(html, "VISITOR_DATA"),
	}
	if cfg.APIKey != "" && cfg.ClientVersion != "" && cfg.VisitorData != "" {
		return cfg
	}
	set := parseJSONObject(extractJSONCall(html, "ytcfg.set("))
	if set == nil {
		return cfg
	}
	if cfg.APIKey == "" {
		cfg.APIKey = stringValue(set["INNERTUBE_API_KEY"])
	}
	if cfg.ClientVersion == "" {
		cfg.ClientVersion = stringValue(set["INNERTUBE_CLIENT_VERSION"])
	}
	if cfg.VisitorData == "" {
		cfg.VisitorData = stringValue(set["VISITOR_DATA"])
	}
	return cfg
}
