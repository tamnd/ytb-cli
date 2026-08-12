package ytb

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// cache.go is a disk cache keyed by the URL and the client that claimed it.
//
// The client is in the key, and that is the whole point of this file rather than
// a detail of it. The same URL answers differently depending on who asks:
// /youtubei/v1/player for dQw4w9WgXcQ returns 29 formats with stream URLs and
// caption tracks that yield bytes when ANDROID asks, and zero formats with
// caption tracks that return 200 and nothing when WEB asks. One cache entry for
// both would serve a WEB player response to `ytb transcript` and the transcript
// would come back empty with no error anywhere to explain it.
//
// So the key is a hash of method, URL, client name, client version and the
// request body. Two reads collide only when they genuinely asked the same
// question as the same client.
//
// `ytb archive` does not use this. An archive is a permanent record written
// outside the cache, and it never expires.

// DefaultCacheTTL is how long a cached response is served before it is refetched.
// Fifteen minutes is long enough that a crawl of one channel does not fetch the
// same page twice, and short enough that a view count is not stale in a way that
// matters.
const DefaultCacheTTL = 15 * time.Minute

// Cache stores responses on disk under a directory, one file per key.
type Cache struct {
	dir string
	ttl time.Duration

	mu sync.Mutex
	// hits and misses are counted so `ytb crawl` can report a request budget it
	// actually spent rather than one it estimated.
	hits, misses int
	// bypass makes every read a miss while still storing what comes back. It is
	// what ytb archive turns on: the point of a capture is what YouTube is serving
	// now, and the point of still writing is that the parse step then sees the
	// same bytes the capture kept.
	bypass bool
}

// cacheEntry is what gets written. StoredAt is in the file rather than taken
// from the file's mtime, because a copied or restored cache directory keeps its
// meaning that way.
type cacheEntry struct {
	StoredAt time.Time `json:"stored_at"`
	URL      string    `json:"url"`
	Client   string    `json:"client"`
	Status   int       `json:"status"`
	Body     []byte    `json:"body"`
}

// NewCache opens a cache under dir. A ttl of zero means DefaultCacheTTL; a
// negative ttl disables the cache, which is what --no-cache passes.
func NewCache(dir string, ttl time.Duration) *Cache {
	if ttl == 0 {
		ttl = DefaultCacheTTL
	}
	return &Cache{dir: dir, ttl: ttl}
}

// Enabled reports whether this cache stores anything. A nil Cache and one built
// with a negative ttl are both disabled, so every call site can treat the cache
// as optional without a nil check of its own.
func (c *Cache) Enabled() bool {
	return c != nil && c.dir != "" && c.ttl > 0
}

// SetBypass makes every Get a miss until it is turned off again, without
// stopping Put. Only ytb archive uses it, and it puts it back afterwards.
func (c *Cache) SetBypass(on bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bypass = on
}

func (c *Cache) bypassing() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.bypass
}

// Stats returns the hit and miss counts for this run.
func (c *Cache) Stats() (hits, misses int) {
	if c == nil {
		return 0, 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits, c.misses
}

// CacheKey is everything that distinguishes one read from another.
type CacheKey struct {
	Method string
	URL    string
	// Client is the claimed client name and version, e.g. "ANDROID/20.10.38".
	// A version bump invalidates the entries taken under the old one, which is
	// correct: the answer may have changed with it.
	Client string
	// Body is the request payload for a POST, nil for a GET.
	Body []byte
}

// String is the on-disk filename for a key.
func (k CacheKey) String() string {
	h := sha256.New()
	for _, part := range []string{k.Method, k.URL, k.Client} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	h.Write(k.Body)
	return hex.EncodeToString(h.Sum(nil))
}

// Get returns a cached response, or ok false when there is none or it has
// expired.
func (c *Cache) Get(k CacheKey) (status int, body []byte, ok bool) {
	if !c.Enabled() || c.bypassing() {
		return 0, nil, false
	}
	raw, err := os.ReadFile(c.path(k))
	if err != nil {
		c.count(false)
		return 0, nil, false
	}
	var e cacheEntry
	if err := json.Unmarshal(raw, &e); err != nil {
		c.count(false)
		return 0, nil, false
	}
	if time.Since(e.StoredAt) > c.ttl {
		c.count(false)
		return 0, nil, false
	}
	c.count(true)
	return e.Status, e.Body, true
}

// Put stores a response. A refusal is cached like any other answer, because a
// refusal is an answer and refetching it changes nothing. A transient failure
// never reaches here: only a response the caller accepted is stored.
func (c *Cache) Put(k CacheKey, status int, body []byte) {
	if !c.Enabled() {
		return
	}
	path := c.path(k)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	raw, err := json.Marshal(cacheEntry{
		StoredAt: time.Now(),
		URL:      k.URL,
		Client:   k.Client,
		Status:   status,
		Body:     body,
	})
	if err != nil {
		return
	}
	// Write to a temp file and rename, so a cancelled run never leaves a half
	// written entry that parses as valid JSON.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
	}
}

// Dir is where entries are written, or "" when the cache is off.
func (c *Cache) Dir() string {
	if c == nil {
		return ""
	}
	return c.dir
}

// TTL is how long an entry is served before it is refetched.
func (c *Cache) TTL() time.Duration {
	if c == nil {
		return 0
	}
	return c.ttl
}

// CacheInfo is what is on disk right now, as `ytb cache info` reports it.
type CacheInfo struct {
	Dir     string        `json:"dir"`
	TTL     time.Duration `json:"ttl"`
	Entries int           `json:"entries"`
	Fresh   int           `json:"fresh"`
	Stale   int           `json:"stale"`
	Bytes   int64         `json:"bytes"`
	Oldest  time.Time     `json:"oldest,omitzero"`
	Newest  time.Time     `json:"newest,omitzero"`
}

// Info walks the cache and reports what is in it.
//
// Fresh and stale are counted against the current TTL rather than stored, since
// the TTL is a flag and the same file is fresh at --cache-ttl 1h and stale at
// the default. A stale entry is not deleted when it expires, it is just not
// served, so a cache can be almost entirely stale and still take up the space.
func (c *Cache) Info() (CacheInfo, error) {
	info := CacheInfo{Dir: c.Dir(), TTL: c.TTL()}
	if info.Dir == "" {
		return info, nil
	}
	now := time.Now()
	err := c.walk(func(path string, size int64) {
		info.Entries++
		info.Bytes += size
		stored := storedAt(path)
		if stored.IsZero() {
			return
		}
		if now.Sub(stored) < c.ttl {
			info.Fresh++
		} else {
			info.Stale++
		}
		if info.Oldest.IsZero() || stored.Before(info.Oldest) {
			info.Oldest = stored
		}
		if stored.After(info.Newest) {
			info.Newest = stored
		}
	})
	return info, err
}

// storedAt reads just the timestamp out of an entry. A file that will not parse
// counts toward the total and toward neither fresh nor stale, because it is
// taking up space and will never be served either way.
func storedAt(path string) time.Time {
	b, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}
	}
	var e struct {
		StoredAt time.Time `json:"stored_at"`
	}
	if json.Unmarshal(b, &e) != nil {
		return time.Time{}
	}
	return e.StoredAt
}

// Purge removes every entry, expired or not, and reports how many went.
func (c *Cache) Purge() (int, error) {
	n := 0
	err := c.walk(func(path string, _ int64) {
		if os.Remove(path) == nil {
			n++
		}
	})
	return n, err
}

// walk visits every entry file. It recurses, because path shards on the first
// two hex characters and a walk of the top level alone finds nothing but the
// 256 shard directories. Purge used to do exactly that and reported deleting
// zero files from a full cache, which is the kind of bug a command has to exist
// before anybody notices.
func (c *Cache) walk(fn func(path string, size int64)) error {
	if c == nil || c.dir == "" {
		return nil
	}
	shards, err := os.ReadDir(c.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, shard := range shards {
		p := filepath.Join(c.dir, shard.Name())
		if !shard.IsDir() {
			if fi, err := shard.Info(); err == nil {
				fn(p, fi.Size())
			}
			continue
		}
		files, err := os.ReadDir(p)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			if fi, err := f.Info(); err == nil {
				fn(filepath.Join(p, f.Name()), fi.Size())
			}
		}
	}
	return nil
}

// path shards on the first two hex characters, so a long crawl does not put a
// hundred thousand files in one directory.
func (c *Cache) path(k CacheKey) string {
	name := k.String()
	return filepath.Join(c.dir, name[:2], name[2:]+".json")
}

func (c *Cache) count(hit bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if hit {
		c.hits++
		return
	}
	c.misses++
}
