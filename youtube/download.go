package youtube

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

// download.go is the media plane, s8. Doc 01 section 8.
//
// There is one rule here and it is worth 132 times the throughput. Measured on
// itag 140 of dQw4w9WgXcQ, contentLength 3449447, same URL, same session,
// seconds apart:
//
//	ranged, 1 MiB chunks : 3449447 b in  0.80 s = 4232 KiB/s (complete)
//	un-ranged, one GET   :  327680 b in 10.02 s =   32 KiB/s
//	un-ranged, again     :  327680 b in 10.02 s =   32 KiB/s
//
// 32 KiB/s is the throttle floor and not a slow moment: it reproduced exactly,
// and the un-ranged GET never finished at all. So every request to googlevideo
// carries a Range header and there is no code path in this file that omits one,
// including the unknown-length case, which walks open chunks rather than falling
// back to a plain GET. TestEveryMediaRequestIsRanged holds the line.
//
// Two consequences shape the rest.
//
// contentLength is known before the first byte, so the total is real rather than
// estimated, and a resume needs nothing from the server: the part file's size is
// the next chunk's start offset. That only stays true if the file is written in
// order, which is why the workers fetch ahead into memory and one writer appends
// in sequence. A preallocated file written at offsets would report the size of
// the highest chunk that happened to land, and a resume off that number would
// hand back a file with holes in it.
//
// The URL carries an expire, and a long download outlives it. That is a re-read
// of s3 and not a failure, so Refresh is asked for a fresh URL before the chunk
// that would cross it, and again if the CDN answers 403 anyway.

// DefaultChunkSize is the byte span requested per ranged GET. Doc 01 section 8
// measured this size at line speed; larger chunks do not go faster and cost more
// memory per worker, since a chunk is held whole to keep the writes in order.
const DefaultChunkSize = 1 << 20

// expireSkew is how long before a URL's stated expiry the downloader stops
// trusting it. A chunk in flight when the clock passes expire comes back 403,
// and the round trip to re-read the player is cheaper than that failure.
const expireSkew = 60 * time.Second

// DownloadProgress reports cumulative bytes written and the total when known.
type DownloadProgress struct {
	Downloaded int64
	Total      int64
}

// DownloadOptions shapes one file download.
type DownloadOptions struct {
	// Total is contentLength off the format. Zero means unknown, which costs the
	// progress total and nothing else.
	Total int64
	// ChunkSize is the range span. Zero takes DefaultChunkSize.
	ChunkSize int64
	// Workers is how many chunks are in flight. Zero or one is sequential.
	Workers int
	// UserAgent is the client that produced the URL. A stream URL minted for
	// ANDROID_VR and fetched with a browser agent is a different request.
	UserAgent string
	// OnProgress is called as bytes land, from one goroutine.
	OnProgress func(DownloadProgress)
	// Refresh re-reads the format and returns a fresh URL. It is called when the
	// current URL is about to expire and when the CDN refuses one that should
	// still be good. Nil means a download that outlives its URL fails instead.
	Refresh func(context.Context) (string, error)
	// Resume continues into an existing part file rather than starting over.
	Resume bool
}

// DownloadToFile fetches rawURL into dst in ranged chunks.
func (c *Client) DownloadToFile(ctx context.Context, rawURL, dst string, opt DownloadOptions) error {
	if opt.ChunkSize <= 0 {
		opt.ChunkSize = DefaultChunkSize
	}
	if opt.Workers < 1 {
		opt.Workers = 1
	}

	flags := os.O_CREATE | os.O_WRONLY
	if !opt.Resume {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(dst, flags, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	var start int64
	if opt.Resume {
		info, err := f.Stat()
		if err != nil {
			return err
		}
		start = info.Size()
		// A part file at or past the stated length is either finished or is not the
		// file we think it is. Starting over is the only answer that cannot ship a
		// wrong file.
		if opt.Total > 0 && start >= opt.Total {
			if start == opt.Total {
				report(opt.OnProgress, start, opt.Total)
				return nil
			}
			if err := f.Truncate(0); err != nil {
				return err
			}
			start = 0
		}
		if _, err := f.Seek(start, io.SeekStart); err != nil {
			return err
		}
	}

	d := &download{
		client: c,
		url:    rawURL,
		opt:    opt,
		file:   f,
		done:   start,
	}
	d.expiry = urlExpiry(rawURL)
	return d.run(ctx, start)
}

// download is one file's state: the URL in force, how far the file is written,
// and the expiry that decides when to ask for a new URL.
type download struct {
	client *Client
	opt    DownloadOptions
	file   io.Writer

	mu     sync.Mutex
	url    string
	expiry time.Time

	done int64
}

// run walks the stream from start in chunks, fetching up to Workers of them at
// once and writing them in order.
func (d *download) run(ctx context.Context, start int64) error {
	report(d.opt.OnProgress, d.done, d.opt.Total)

	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		index int
		data  []byte
		err   error
	}

	var (
		wg      sync.WaitGroup
		results = make(chan result)
		next    = start
		index   = 0
		issued  = 0
	)

	// The writer holds out-of-order chunks until the one it is waiting for
	// arrives, so the file only ever grows contiguously and its size stays the
	// resume offset.
	pending := map[int][]byte{}
	writeIndex := 0
	var firstErr error

	inFlight := 0
	send := func() bool {
		if d.opt.Total > 0 && next >= d.opt.Total {
			return false
		}
		end := next + d.opt.ChunkSize - 1
		if d.opt.Total > 0 && end >= d.opt.Total {
			end = d.opt.Total - 1
		}
		wg.Add(1)
		go func(i int, from, to int64) {
			defer wg.Done()
			data, err := d.chunk(cctx, from, to)
			select {
			case results <- result{index: i, data: data, err: err}:
			case <-cctx.Done():
			}
		}(index, next, end)
		next = end + 1
		index++
		issued++
		inFlight++
		return true
	}

	for range d.opt.Workers {
		if !send() {
			break
		}
	}

	last := false
	for inFlight > 0 {
		r := <-results
		inFlight--
		if r.err != nil {
			if firstErr == nil {
				firstErr = r.err
			}
			cancel()
			continue
		}
		pending[r.index] = r.data
		for {
			data, ok := pending[writeIndex]
			if !ok {
				break
			}
			delete(pending, writeIndex)
			writeIndex++
			if len(data) == 0 {
				// An empty chunk past the end is how an unknown-length stream says it
				// is finished.
				last = true
				continue
			}
			if _, err := d.file.Write(data); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				cancel()
				break
			}
			d.done += int64(len(data))
			report(d.opt.OnProgress, d.done, d.opt.Total)
			// Short of what was asked for with no error is the end of an
			// unknown-length stream.
			if d.opt.Total <= 0 && int64(len(data)) < d.opt.ChunkSize {
				last = true
			}
		}
		if firstErr == nil && !last {
			send()
		}
	}
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}
	if d.opt.Total > 0 && d.done < d.opt.Total {
		return fmt.Errorf("short download: %d of %d bytes", d.done, d.opt.Total)
	}
	return nil
}

// chunk returns bytes [from,to] of the stream.
//
// A 206 that stops early is the common failure here and it is not an error: the
// CDN closes the body when it feels like it. The next request picks up at the
// offset the last one reached rather than at from, so a chunk that arrives in
// three pieces costs three requests and not three copies of the same bytes.
// Attempts are counted over the whole chunk, but progress resets the count,
// because a body that keeps delivering is a working connection however many
// times it has been reopened.
func (d *download) chunk(ctx context.Context, from, to int64) ([]byte, error) {
	attempts := d.client.retries + 1
	if attempts < 1 {
		attempts = 1
	}
	// maxPasses bounds a server that answers every request with one byte. It is
	// generous because a partial body is normal here and each pass that delivers
	// bytes is real progress, and it is finite because a loop that only ever ends
	// on success is not a loop, it is a hang.
	maxPasses := attempts * 32
	want := to - from + 1
	buf := make([]byte, 0, want)
	failures := 0
	var lastErr error
	for pass := 0; pass < maxPasses; pass++ {
		if failures > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(failures) * 500 * time.Millisecond):
			}
		}
		rawURL, err := d.currentURL(ctx)
		if err != nil {
			return nil, err
		}
		at := from + int64(len(buf))
		n, err := d.fetchRange(ctx, rawURL, at, to, &buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = err
			failures++
			if failures >= attempts {
				return nil, lastErr
			}
			// A refusal on a URL that has not visibly expired still means the URL and
			// not the network. One re-read, then the retry loop decides.
			if errors.Is(err, errStreamURLStale) {
				if _, rerr := d.refresh(ctx); rerr != nil {
					return nil, fmt.Errorf("%w, and re-reading the format failed: %w", err, rerr)
				}
			}
			continue
		}
		if int64(len(buf)) >= want || n == 0 {
			// n == 0 with no error is the end of the stream, which only happens on an
			// unknown-length download asking past the end.
			return buf, nil
		}
		// Bytes landed and the body ended anyway. That is not a failed attempt, so
		// the failure count goes back to zero and the next pass asks from here.
		failures = 0
		lastErr = fmt.Errorf("range GET %d-%d stopped after %d of %d bytes", at, to, len(buf), want)
	}
	return nil, lastErr
}

// errStreamURLStale is the CDN saying no to a URL rather than to the request.
var errStreamURLStale = errors.New("the CDN refused this stream URL")

// fetchRange performs one ranged GET, appends what came back to buf and reports
// how many bytes that was. It appends rather than returning a slice so a chunk
// that arrives in pieces is assembled without copying it again per piece.
func (d *download) fetchRange(ctx context.Context, rawURL string, from, to int64, buf *[]byte) (int, error) {
	d.client.noteRequest(http.MethodGet, rawURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", streamUserAgent(d.opt.UserAgent))
	// The one rule. Every request out of this file carries this header, including
	// the retries and the ones after a refresh.
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", from, to))
	resp, err := d.client.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusPartialContent:
	case http.StatusOK:
		// A 200 to a ranged request is a server that ignored the header. At offset
		// zero the body is the file and the limit below keeps the right part of it.
		// Past offset zero it is bytes from the wrong place, and writing them would
		// corrupt the file quietly.
		if from > 0 {
			return 0, fmt.Errorf("range GET %d-%d: the server answered 200 and ignored the range", from, to)
		}
	case http.StatusRequestedRangeNotSatisfiable:
		// Asked past the end of a stream whose length was not known up front, which
		// is how that download learns it is finished.
		return 0, nil
	case http.StatusForbidden, http.StatusUnauthorized:
		return 0, fmt.Errorf("%w: HTTP %d", errStreamURLStale, resp.StatusCode)
	default:
		return 0, fmt.Errorf("range GET %d-%d: HTTP %d", from, to, resp.StatusCode)
	}

	before := len(*buf)
	body := io.LimitReader(resp.Body, to-from+1)
	for {
		if cap(*buf) == len(*buf) {
			*buf = append(*buf, 0)[:len(*buf)]
		}
		n, err := body.Read((*buf)[len(*buf):cap(*buf)])
		*buf = (*buf)[:len(*buf)+n]
		if err == io.EOF {
			break
		}
		if err != nil {
			// Bytes already in buf are good bytes. The caller counts them, resumes
			// from where they end and does not treat a cut body as a lost chunk.
			return len(*buf) - before, err
		}
	}
	return len(*buf) - before, nil
}

// currentURL returns the URL to use now, re-reading the format first when the
// one in hand is about to expire.
func (d *download) currentURL(ctx context.Context) (string, error) {
	d.mu.Lock()
	rawURL, expiry := d.url, d.expiry
	d.mu.Unlock()
	if expiry.IsZero() || time.Until(expiry) > expireSkew || d.opt.Refresh == nil {
		return rawURL, nil
	}
	return d.refresh(ctx)
}

// refresh asks for a fresh URL. Callers race here, and the second one through
// takes the first one's answer rather than reading the player again.
func (d *download) refresh(ctx context.Context) (string, error) {
	if d.opt.Refresh == nil {
		return "", errors.New("this stream URL has expired and there is no way to re-read it")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.expiry.IsZero() && time.Until(d.expiry) > expireSkew {
		return d.url, nil
	}
	fresh, err := d.opt.Refresh(ctx)
	if err != nil {
		return "", err
	}
	if fresh == "" {
		return "", errors.New("re-reading the format produced no URL")
	}
	d.url = fresh
	d.expiry = urlExpiry(fresh)
	return fresh, nil
}

func report(fn func(DownloadProgress), done, total int64) {
	if fn != nil {
		fn(DownloadProgress{Downloaded: done, Total: total})
	}
}

func streamUserAgent(userAgent string) string {
	if userAgent != "" {
		return userAgent
	}
	return androidVRUA
}
