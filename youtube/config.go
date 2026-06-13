package youtube

import (
	"time"
)

const (
	// BaseURL is the YouTube web origin.
	BaseURL = "https://www.youtube.com"
	// MusicBaseURL is the YouTube Music web origin.
	MusicBaseURL = "https://music.youtube.com"

	// DefaultDelay is the polite minimum delay between requests.
	DefaultDelay = 1500 * time.Millisecond
	// DefaultWorkers is the default concurrency for detail fetches and crawl workers.
	DefaultWorkers = 4
	// DefaultTimeout is the default per-request timeout.
	DefaultTimeout = 30 * time.Second
	// DefaultRetries is the default retry count on transient failures.
	DefaultRetries = 3
	// DefaultMaxResults caps a list command's rows when the user gives no -n.
	DefaultMaxResults = 0 // 0 == unlimited (page-capped instead)

	// Entity kinds used by the crawl queue.
	EntityVideo     = "video"
	EntityChannel   = "channel"
	EntityPlaylist  = "playlist"
	EntitySearch    = "search"
	EntityHashtag   = "hashtag"
	EntityComments  = "comments"
	EntityCommunity = "community"
)

// userAgents is a small pool of realistic desktop UA strings rotated per request
// so the traffic looks like an ordinary browser.
var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36",
}

// Config controls the HTTP client and InnerTube behaviour.
type Config struct {
	Workers int
	Delay   time.Duration
	Timeout time.Duration
	Retries int
	HL      string // interface language, e.g. "en"
	GL      string // content country, e.g. "US"
}

// DefaultConfig returns the zero-setup defaults.
func DefaultConfig() Config {
	return Config{
		Workers: DefaultWorkers,
		Delay:   DefaultDelay,
		Timeout: DefaultTimeout,
		Retries: DefaultRetries,
		HL:      "en",
		GL:      "US",
	}
}
