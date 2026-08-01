package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// call.go is the one path an InnerTube request takes.
//
// Everything a request needs to get right lives here rather than at each of the
// forty call sites: the harvested key, headers and body built from the same
// ClientSpec, hl and gl in the context, the cache keyed by URL plus client, the
// rate limit, a retry that stops at a refusal, and the alerts check that turns a
// 200 into an error when YouTube declined inside one.

// Call posts to an InnerTube verb as a given client and returns the decoded
// response.
//
// subject names what is being read, for the error message. It is not sent.
func (c *Client) Call(ctx context.Context, spec ClientSpec, verb string, body map[string]any, subject string) (map[string]any, error) {
	cfg, err := c.ytcfgFor(ctx, spec.Host)
	if err != nil {
		return nil, err
	}

	// The client version comes from the harvest when the harvest has one and the
	// spec is a browser client, because a stale web version is a slow failure.
	// A mobile client keeps its own: the harvest is the web app's version and
	// claiming it as ANDROID is the mismatch this file exists to prevent.
	effective := spec
	if effective.Name == "WEB" && cfg.ClientVersion != "" {
		effective.Version = cfg.ClientVersion
	}

	full := map[string]any{}
	for k, v := range body {
		full[k] = v
	}
	full["context"] = effective.Context(c.hl, c.gl, cfg.VisitorData)

	raw, err := json.Marshal(full)
	if err != nil {
		return nil, err
	}
	url := effective.Endpoint(verb) + "?key=" + cfg.APIKey + "&prettyPrint=false"
	data, err := c.doInnerTube(ctx, effective, url, raw, cfg.VisitorData)
	if err != nil {
		return nil, err
	}

	var resp map[string]any
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("%s: %s returned invalid JSON: %w", subject, verb, err)
	}
	// A 200 can mean no. An alertRenderer of type ERROR is a refusal whatever the
	// status code said, and it is checked on every response rather than only the
	// ones that looked suspicious.
	if r := alertRefusal(resp, subject, verb); r != nil {
		return nil, r
	}
	// And a refusal does not always arrive as an alert. The posts tab returns 200
	// with no alerts at all and puts its no in a messageRenderer where the posts
	// should be, so that shape is checked too.
	if r := messageRefusal(resp, subject, verb); r != nil {
		return nil, r
	}
	if r := playabilityRefusal(resp, subject, verb); r != nil {
		return nil, r
	}
	return resp, nil
}

// doInnerTube performs the POST, through the cache and the retry loop.
func (c *Client) doInnerTube(ctx context.Context, spec ClientSpec, url string, body []byte, visitorData string) ([]byte, error) {
	key := CacheKey{
		Method: http.MethodPost,
		// The harvested key is stripped from the cache key. It rotates, and an
		// entry keyed on it would miss for the rest of a run after a rotation
		// without the answer having changed.
		URL:    stripQueryParam(url, "key"),
		Client: spec.Name + "/" + spec.Version,
		Body:   body,
	}
	if status, cached, ok := c.cache.Get(key); ok && status == 200 {
		return cached, nil
	}

	attempts := c.retries + 1
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		c.rateLimit()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		for k, v := range spec.Headers(visitorData) {
			req.Header.Set(k, v)
		}
		c.setLanguageHeaders(req)
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
			lastErr = fmt.Errorf("POST %s as %s: HTTP %d", url, spec.Name, resp.StatusCode)
			continue
		}
		if resp.StatusCode != 200 {
			// A 400 here is usually a malformed id rather than a server problem,
			// and retrying it three times is three ways to be wrong. YouTube's
			// 400 body names no field, so the caller's subject is the only clue
			// the user gets and it has to be a good one.
			return nil, &statusError{
				code: resp.StatusCode,
				msg:  fmt.Sprintf("POST %s as %s: HTTP %d: %s", url, spec.Name, resp.StatusCode, firstLine(data, 200)),
			}
		}
		c.cache.Put(key, resp.StatusCode, data)
		return data, nil
	}
	return nil, lastErr
}

// statusError carries the HTTP status alongside the message, for the caller that
// wants to tell one failure from another rather than print both the same way.
//
// A browse of a playlist id that does not exist answers 404 and a browse of an
// id that is not a playlist id at all answers 400. The first is "no playlist
// there" and the second is "that is not an address", and they are worth
// different exit codes.
type statusError struct {
	code int
	msg  string
}

func (e *statusError) Error() string { return e.msg }

// httpStatus reports the status an error carries, or 0 when it carries none.
func httpStatus(err error) int {
	var se *statusError
	if errors.As(err, &se) {
		return se.code
	}
	return 0
}

// playabilityRefusal turns a non-OK playabilityStatus into a refusal. A player
// response with status UNPLAYABLE or LOGIN_REQUIRED carries YouTube's reason, and
// that sentence is worth more than any wording of ours.
func playabilityRefusal(resp map[string]any, subject, surface string) *Refusal {
	ps := mapValue(resp, "playabilityStatus")
	if ps == nil {
		return nil
	}
	switch stringValue(ps["status"]) {
	case "", "OK":
		return nil
	}
	reason := stringValue(ps["reason"])
	if reason == "" {
		if sub := mapValue(ps, "errorScreen"); sub != nil {
			reason = extractText(sub)
		}
	}
	if reason == "" {
		reason = stringValue(ps["status"])
	}
	return newRefusal(subject, surface, reason)
}

// backoff grows with the attempt and carries a fixed offset rather than jitter
// from a random source, so a replayed run is reproducible.
func backoff(attempt int) time.Duration {
	return time.Duration(attempt)*500*time.Millisecond + 100*time.Millisecond
}

// stripQueryParam removes one parameter from a URL, leaving the rest in place.
func stripQueryParam(raw, param string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	q.Del(param)
	u.RawQuery = q.Encode()
	return u.String()
}

// firstLine returns up to n bytes of a response body as a single line, for an
// error message. A YouTube error body is JSON on one line already, but a proxy
// or a captive portal may return HTML.
func firstLine(b []byte, n int) string {
	if len(b) > n {
		b = b[:n]
	}
	out := make([]rune, 0, len(b))
	for _, r := range string(b) {
		if r == '\n' || r == '\r' {
			r = ' '
		}
		out = append(out, r)
	}
	return string(out)
}
