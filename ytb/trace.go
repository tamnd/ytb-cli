package ytb

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// trace.go turns -v into something you can read.
//
// The trace sits in the transport rather than at the call sites. There are
// dozens of places in this package that build a request, and a hook at each of
// them is a hook the next read forgets to call, which is the failure mode where
// a flag looks like it works because the requests it does show are the ones
// somebody remembered. One RoundTripper sees every request there is, including
// redirects and the download engine's byte ranges.

// traceTransport writes one line per request to w and then delegates.
type traceTransport struct {
	inner http.RoundTripper
	w     io.Writer
	level int

	mu sync.Mutex // one line at a time, since ranges are fetched in parallel
}

// SetTrace makes every outgoing request print a line to w. Level 1 is the
// request and what came back; level 2 adds the headers that decide what a
// request means, which is the Range on a media fetch and the InnerTube client
// name on a POST.
//
// Cookie and Authorization are never printed at any level. A session is the
// user's own account, and a trace line is the kind of thing that gets pasted
// into a bug report.
func (c *Client) SetTrace(w io.Writer, level int) {
	if c == nil || w == nil || level <= 0 {
		return
	}
	inner := c.http.Transport
	if inner == nil {
		inner = http.DefaultTransport
	}
	c.http.Transport = &traceTransport{inner: inner, w: w, level: level}
}

func (t *traceTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := t.inner.RoundTrip(req)
	t.log(req, resp, err, time.Since(start))
	return resp, err
}

// log prints the one line. It never fails the request: a trace that cannot be
// written is not a reason for a download to stop.
func (t *traceTransport) log(req *http.Request, resp *http.Response, err error, took time.Duration) {
	var b strings.Builder
	fmt.Fprintf(&b, "%-4s ", req.Method)
	switch {
	case err != nil:
		b.WriteString("---")
	default:
		fmt.Fprintf(&b, "%3d", resp.StatusCode)
	}
	fmt.Fprintf(&b, " %6s %s", took.Round(time.Millisecond), traceURL(req, t.level))

	if t.level > 1 {
		for _, h := range traceHeaders {
			if v := req.Header.Get(h); v != "" {
				fmt.Fprintf(&b, " %s=%s", h, v)
			}
		}
		if resp != nil {
			if ct := resp.Header.Get("Content-Type"); ct != "" {
				fmt.Fprintf(&b, " type=%s", firstField(ct))
			}
			if resp.ContentLength > 0 {
				fmt.Fprintf(&b, " len=%d", resp.ContentLength)
			}
		}
	}
	if err != nil {
		fmt.Fprintf(&b, " error=%v", err)
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	_, _ = fmt.Fprintln(t.w, b.String())
}

// traceURL is the URL as the line should show it. -v drops the query, because a
// signed googlevideo URL is a thousand characters of signature and a screen of
// them is not a trace anybody reads. -vv keeps the whole thing, since the point
// of the second v is being able to replay the request with curl.
//
// The one query parameter -v keeps is v=, the video id, without which a line
// about /watch says nothing about which watch.
func traceURL(req *http.Request, level int) string {
	if level > 1 {
		return req.URL.String()
	}
	u := *req.URL
	id := u.Query().Get("v")
	u.RawQuery = ""
	u.Fragment = ""
	if id != "" {
		return u.String() + "?v=" + id
	}
	return u.String()
}

// traceHeaders are the request headers worth seeing, and the list is closed on
// purpose so that a header added later cannot leak by being new.
var traceHeaders = []string{"Range", "X-Youtube-Client-Name", "X-Youtube-Client-Version", "X-Goog-Visitor-Id"}

// firstField trims "text/html; charset=utf-8" to "text/html".
func firstField(s string) string {
	if i := strings.IndexByte(s, ';'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}
