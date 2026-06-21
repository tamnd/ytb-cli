package youtube

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// downloadChunkSize is the byte span requested per Range GET. googlevideo
// throttles or caps very large single requests, so the downloader walks the
// stream in chunks; this also keeps memory flat and enables resume.
const downloadChunkSize = 9 << 20 // 9 MiB

// DownloadProgress reports cumulative bytes written and the total when known.
type DownloadProgress struct {
	Downloaded int64
	Total      int64
}

// DownloadToFile fetches rawURL into dst. When total is known and workers > 1
// it downloads ranges concurrently, writing each at its offset; otherwise it
// streams sequentially. onProgress, if non-nil, is called as bytes land.
func (c *Client) DownloadToFile(ctx context.Context, rawURL, dst string, total int64, workers int, onProgress func(DownloadProgress)) error {
	return c.DownloadToFileWithUserAgent(ctx, rawURL, dst, total, workers, "", onProgress)
}

// DownloadToFileWithUserAgent fetches rawURL into dst using the stream's
// matching client User-Agent when one is known.
func (c *Client) DownloadToFileWithUserAgent(ctx context.Context, rawURL, dst string, total int64, workers int, userAgent string, onProgress func(DownloadProgress)) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	if total <= 0 {
		// Unknown length: a single streaming GET is the only option.
		n, err := c.streamTo(ctx, rawURL, f, 0, userAgent, onProgress)
		if err != nil {
			return err
		}
		if onProgress != nil {
			onProgress(DownloadProgress{Downloaded: n, Total: n})
		}
		return nil
	}

	if err := f.Truncate(total); err != nil {
		return err
	}

	if workers < 1 {
		workers = 1
	}

	type chunk struct{ start, end int64 }
	var chunks []chunk
	for start := int64(0); start < total; start += downloadChunkSize {
		end := start + downloadChunkSize - 1
		if end >= total {
			end = total - 1
		}
		chunks = append(chunks, chunk{start, end})
	}

	var done int64
	report := func(n int64) {
		if onProgress == nil {
			return
		}
		onProgress(DownloadProgress{Downloaded: atomic.AddInt64(&done, n), Total: total})
	}

	if workers == 1 {
		for _, ch := range chunks {
			if err := c.fetchRangeTo(ctx, rawURL, f, ch.start, ch.end, userAgent, report); err != nil {
				return err
			}
		}
		return nil
	}

	jobs := make(chan chunk)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ch := range jobs {
				if cctx.Err() != nil {
					return
				}
				if err := c.fetchRangeTo(cctx, rawURL, f, ch.start, ch.end, userAgent, report); err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					cancel()
					return
				}
			}
		}()
	}
	for _, ch := range chunks {
		select {
		case <-cctx.Done():
			goto wait
		case jobs <- ch:
		}
	}
wait:
	close(jobs)
	wg.Wait()
	return firstErr
}

// fetchRangeTo downloads bytes [start,end] of rawURL and writes them at start
// using WriteAt, retrying transient failures.
func (c *Client) fetchRangeTo(ctx context.Context, rawURL string, w io.WriterAt, start, end int64, userAgent string, report func(int64)) error {
	attempts := c.retries + 1
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * 500 * time.Millisecond):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", streamUserAgent(userAgent))
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("range GET: HTTP %d", resp.StatusCode)
			continue
		}
		offset := start
		buf := make([]byte, 64<<10)
		for {
			n, rerr := resp.Body.Read(buf)
			if n > 0 {
				if _, werr := w.WriteAt(buf[:n], offset); werr != nil {
					_ = resp.Body.Close()
					return werr
				}
				offset += int64(n)
				if report != nil {
					report(int64(n))
				}
			}
			if rerr == io.EOF {
				break
			}
			if rerr != nil {
				lastErr = rerr
				break
			}
		}
		_ = resp.Body.Close()
		if lastErr == nil {
			return nil
		}
		// Partial read failure: rewind progress is not tracked per-attempt, so
		// resume from where we stopped on the next attempt.
		start = offset
	}
	return lastErr
}

// streamTo performs a single GET, copying the body to w starting at offset 0.
func (c *Client) streamTo(ctx context.Context, rawURL string, w io.Writer, _ int64, userAgent string, onProgress func(DownloadProgress)) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", streamUserAgent(userAgent))
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("GET stream: HTTP %d", resp.StatusCode)
	}
	var written int64
	buf := make([]byte, 64<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return written, werr
			}
			written += int64(n)
			if onProgress != nil {
				onProgress(DownloadProgress{Downloaded: written, Total: 0})
			}
		}
		if rerr == io.EOF {
			return written, nil
		}
		if rerr != nil {
			return written, rerr
		}
	}
}

func streamUserAgent(userAgent string) string {
	if userAgent != "" {
		return userAgent
	}
	return androidVRUA
}
