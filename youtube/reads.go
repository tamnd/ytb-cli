package youtube

import (
	"net/http"
	"strings"
	"time"
)

// reads.go is the audit log's other end. Spec 3005 doc 04 section 4.
//
// SetOnRequest says a request is going out and is what a budget counts on.
// SetOnRead says what came back, and it is what the reads table is written from
// and what ytb archive captures. They are two hooks because they answer at two
// different moments: a budget has to stop the walk before the request is made,
// and a log entry cannot be written until the response has been read.

// Read is one request that actually went out, with its outcome.
type Read struct {
	Method string `json:"method"`
	// URL is what was fetched, with the harvested key stripped and, on an
	// InnerTube POST, the operation written as a fragment. Doc 04 section 4:
	// thirty browses of /youtubei/v1/browse in a log that cannot tell them apart
	// is a log nobody can check a record against. A fragment is never sent to a
	// server, so writing one down invents nothing about the request.
	URL     string `json:"url"`
	Surface string `json:"surface"`
	// Client is WEB, ANDROID, WEB_REMIX, or empty on a plain HTML read.
	Client string    `json:"client,omitempty"`
	Status int       `json:"status"`
	Bytes  int       `json:"bytes"`
	At     time.Time `json:"at"`
	Error  string    `json:"error,omitempty"`
	// Headers are the request headers as they were sent, with anything that
	// carries a session replaced. They are filled for every read and nothing but
	// ytb archive looks at them.
	Headers map[string]string `json:"headers,omitempty"`
	// Body is the response as it arrived. It is passed to the hook and never kept
	// by the client, so a caller that does not want a megabyte per read simply
	// does not hold on to it.
	Body []byte `json:"-"`
}

// SetOnRead installs a hook called once for every request that went out and came
// back, whether it came back with an answer or an error.
//
// A cache hit never reaches it, for the same reason it never reaches the request
// hook: nothing was asked of YouTube, so there is nothing to log.
func (c *Client) SetOnRead(fn func(Read)) { c.onRead = fn }

func (c *Client) noteRead(r Read) {
	if c.onRead == nil {
		return
	}
	if r.At.IsZero() {
		r.At = time.Now()
	}
	c.onRead(r)
}

// sentHeaders copies a request's headers into a plain map, replacing the ones
// that carry a session.
//
// The replacement is a sentence rather than a blank, because a capture with an
// empty Cookie header reads as a request that sent no cookie, and that is a
// different request from the one that was made.
func sentHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		if len(v) == 0 {
			continue
		}
		switch http.CanonicalHeaderKey(k) {
		case "Cookie", "Authorization", "X-Goog-Authuser", "X-Origin":
			out[k] = "(removed: this header carried a session)"
		default:
			out[k] = strings.Join(v, ", ")
		}
	}
	return out
}

// logURL is the address as the log keeps it: the harvested key taken out and
// the operation written on as a fragment.
func logURL(raw, subject string) string {
	raw = stripQueryParam(raw, "key")
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return raw
	}
	return raw + "#" + subject
}

// surfaceForURL names the surface a URL belongs to, in doc 01's numbers.
//
// It reads the address rather than being told, because the reads log is written
// by two functions and the forty call sites above them do not know which surface
// they are on. A URL that matches nothing comes back as the browse page surface,
// which is what an unrecognised www.youtube.com GET is.
func surfaceForURL(rawURL, client string) string {
	u := strings.ToLower(rawURL)
	switch {
	case strings.Contains(u, "music.youtube.com"):
		return SurfaceMusic
	case strings.Contains(u, "/youtubei/v1/player"):
		// The mobile player is a different surface from the web one, and that is not
		// bookkeeping: it is the client that answers with caption tracks and stream
		// URLs on a video the web client calls UNPLAYABLE.
		if client == "ANDROID" || client == "IOS" {
			return SurfaceMobilePlayer
		}
		return SurfaceInnerTube
	case strings.Contains(u, "/youtubei/v1/"):
		return SurfaceInnerTube
	case strings.Contains(u, "/feeds/videos.xml"):
		return SurfaceFeed
	case strings.Contains(u, "/oembed"):
		return SurfaceOEmbed
	case strings.Contains(u, "suggestqueries") || strings.Contains(u, "/complete/search"):
		return SurfaceSuggest
	case strings.Contains(u, "ytimg.com"):
		return SurfaceThumbCDN
	case strings.Contains(u, "googlevideo.com"):
		return SurfaceMediaCDN
	case strings.Contains(u, "/watch") || strings.Contains(u, "youtu.be/") || strings.Contains(u, "/shorts/"):
		return SurfaceWatchHTML
	default:
		return SurfaceBrowseHTML
	}
}
