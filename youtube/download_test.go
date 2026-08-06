package youtube

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// download_test.go guards the one rule of the media plane: every request to a
// stream URL carries a Range header. Doc 01 section 8 measured what happens
// without it, on itag 140 of dQw4w9WgXcQ, same URL, seconds apart:
//
//	ranged, 1 MiB chunks : 3449447 b in  0.80 s = 4232 KiB/s (complete)
//	un-ranged, one GET   :  327680 b in 10.02 s =   32 KiB/s (never finished)
//
// So the tests below do not check that ranges are used on the happy path. They
// check that there is no path at all, including retries, refreshes, resumes and
// unknown-length streams, that produces a request without the header.

// mediaServer is a stand-in for googlevideo that records every request it is
// given and can be told to misbehave in the specific ways the real one does.
type mediaServer struct {
	body []byte

	mu       sync.Mutex
	requests []*http.Request
	// fail returns a status to answer with instead of serving the range, given
	// the request count so far. Zero means serve normally.
	fail func(n int) int
	// short truncates the nth response to this many bytes, which is what a CDN
	// closing the body early looks like.
	short func(n int) int
}

func (m *mediaServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	m.requests = append(m.requests, r.Clone(context.Background()))
	n := len(m.requests)
	m.mu.Unlock()

	if m.fail != nil {
		if code := m.fail(n); code != 0 {
			w.WriteHeader(code)
			return
		}
	}

	from, to, ok := parseRangeHeader(r.Header.Get("Range"))
	if !ok {
		// The un-ranged path. A real googlevideo answers this slowly rather than
		// refusing, but nothing here should ever reach it, so it is loud.
		http.Error(w, "no range header", http.StatusBadRequest)
		return
	}
	if from >= int64(len(m.body)) {
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return
	}
	if to >= int64(len(m.body)) {
		to = int64(len(m.body)) - 1
	}
	data := m.body[from : to+1]
	if m.short != nil {
		if cut := m.short(n); cut > 0 && cut < len(data) {
			data = data[:cut]
		}
	}
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, from+int64(len(data))-1, len(m.body)))
	w.WriteHeader(http.StatusPartialContent)
	_, _ = w.Write(data)
}

func (m *mediaServer) ranges() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.requests))
	for _, r := range m.requests {
		out = append(out, r.Header.Get("Range"))
	}
	return out
}

func parseRangeHeader(h string) (from, to int64, ok bool) {
	if !strings.HasPrefix(h, "bytes=") {
		return 0, 0, false
	}
	lo, hi, found := strings.Cut(strings.TrimPrefix(h, "bytes="), "-")
	if !found {
		return 0, 0, false
	}
	from, err := strconv.ParseInt(lo, 10, 64)
	if err != nil {
		return 0, 0, false
	}
	to, err = strconv.ParseInt(hi, 10, 64)
	if err != nil {
		return 0, 0, false
	}
	return from, to, true
}

func testBody(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		// A byte that depends on its own offset, so a file assembled out of order
		// or with a gap in it does not compare equal by accident.
		b[i] = byte(i*7 + i/251)
	}
	return b
}

func newTestClient(t *testing.T) *Client {
	t.Helper()
	return NewClient(Config{Retries: 2})
}

// TestEveryMediaRequestIsRanged is the rule. It walks the paths that could
// plausibly reach the network another way and asserts that none of them did.
func TestEveryMediaRequestIsRanged(t *testing.T) {
	cases := []struct {
		label string
		total int64 // what the caller was told the length is, 0 for unknown
		srv   func(m *mediaServer)
	}{
		{label: "known length", total: 4000},
		// The interesting one. contentLength missing used to mean a plain GET,
		// which is the 32 KiB/s path, so the unknown case walks open chunks instead.
		{label: "unknown length", total: 0},
		{label: "retry after a 500", total: 4000, srv: func(m *mediaServer) {
			m.fail = func(n int) int {
				if n == 2 {
					return http.StatusInternalServerError
				}
				return 0
			}
		}},
		{label: "resume after a 403", total: 4000, srv: func(m *mediaServer) {
			m.fail = func(n int) int {
				if n == 2 {
					return http.StatusForbidden
				}
				return 0
			}
		}},
		{label: "body cut short", total: 4000, srv: func(m *mediaServer) {
			m.short = func(n int) int {
				if n%2 == 0 {
					return 300
				}
				return 0
			}
		}},
	}

	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			m := &mediaServer{body: testBody(4000)}
			if c.srv != nil {
				c.srv(m)
			}
			srv := httptest.NewServer(m)
			defer srv.Close()

			dst := filepath.Join(t.TempDir(), "out.bin")
			refreshed := 0
			err := newTestClient(t).DownloadToFile(context.Background(), srv.URL, dst, DownloadOptions{
				Total:     c.total,
				ChunkSize: 1000,
				Workers:   2,
				Refresh: func(context.Context) (string, error) {
					refreshed++
					return srv.URL, nil
				},
			})
			if err != nil {
				t.Fatalf("download: %v", err)
			}

			got, err := os.ReadFile(dst)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(m.body) {
				t.Errorf("file is %d bytes and does not match the source (want %d)", len(got), len(m.body))
			}
			ranges := m.ranges()
			if len(ranges) == 0 {
				t.Fatal("no requests reached the server")
			}
			for i, r := range ranges {
				if r == "" {
					t.Errorf("request %d of %d went out with no Range header", i+1, len(ranges))
				}
			}
		})
	}
}

// TestDownloadRefreshesAnExpiredURL covers the expire in the URL. A long
// download outlives it, and that is a re-read of the player and not a failure.
func TestDownloadRefreshesAnExpiredURL(t *testing.T) {
	m := &mediaServer{body: testBody(4000)}
	srv := httptest.NewServer(m)
	defer srv.Close()

	// Already expired when the download starts, so the first chunk refreshes
	// before it asks for anything.
	stale := srv.URL + "/videoplayback?expire=" + strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10)
	fresh := srv.URL + "/videoplayback?expire=" + strconv.FormatInt(time.Now().Add(6*time.Hour).Unix(), 10)

	var mu sync.Mutex
	refreshed := 0
	dst := filepath.Join(t.TempDir(), "out.bin")
	err := newTestClient(t).DownloadToFile(context.Background(), stale, dst, DownloadOptions{
		Total:     4000,
		ChunkSize: 1000,
		Workers:   2,
		Refresh: func(context.Context) (string, error) {
			mu.Lock()
			defer mu.Unlock()
			refreshed++
			return fresh, nil
		},
	})
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if refreshed == 0 {
		t.Error("the URL was already expired and nothing re-read the format")
	}
	// Four chunks, and the refresh is shared: whoever gets there first re-reads
	// and everyone else takes that answer rather than hitting the player again.
	if refreshed > 2 {
		t.Errorf("re-read the format %d times for one expiry", refreshed)
	}
	for _, r := range m.requests {
		if !strings.Contains(r.URL.RawQuery, "expire=") {
			t.Errorf("request went out to %s, which is not the refreshed URL", r.URL)
		}
	}
	got, _ := os.ReadFile(dst)
	if string(got) != string(m.body) {
		t.Error("the file does not match the source after a refresh")
	}
}

// TestDownloadWithoutRefreshSaysSo is the other half. An expired URL and no way
// to re-read it is a real failure and it says which of the two it is.
func TestDownloadWithoutRefreshSaysSo(t *testing.T) {
	m := &mediaServer{body: testBody(2000)}
	srv := httptest.NewServer(m)
	defer srv.Close()
	stale := srv.URL + "?expire=" + strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10)

	// Nil Refresh, so the expiry is just information. Requests still go out
	// ranged, because a URL that might work is worth trying and a plain GET is
	// not worth trying under any circumstances.
	err := newTestClient(t).DownloadToFile(context.Background(), stale, filepath.Join(t.TempDir(), "out.bin"), DownloadOptions{
		Total: 2000, ChunkSize: 1000,
	})
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	for i, r := range m.ranges() {
		if r == "" {
			t.Errorf("request %d went out with no Range header", i+1)
		}
	}
}

// TestResumeStartsAtTheFileSize is why the writes are in order. The part file's
// size is the resume offset, and that is only true if the file has no holes.
func TestResumeStartsAtTheFileSize(t *testing.T) {
	body := testBody(4000)
	m := &mediaServer{body: body}
	srv := httptest.NewServer(m)
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "out.bin")
	if err := os.WriteFile(dst, body[:1500], 0o644); err != nil {
		t.Fatal(err)
	}

	c := newTestClient(t)
	if err := c.DownloadToFile(context.Background(), srv.URL, dst, DownloadOptions{
		Total: 4000, ChunkSize: 1000, Workers: 2, Resume: true,
	}); err != nil {
		t.Fatalf("resume: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(body) {
		t.Fatalf("resumed file is %d bytes and does not match the source", len(got))
	}
	// Nothing before byte 1500 was asked for again. Re-fetching what is already
	// on disk is the bug resume exists to avoid.
	for _, r := range m.ranges() {
		from, _, ok := parseRangeHeader(r)
		if !ok {
			t.Fatalf("resume sent %q, which is not a range", r)
		}
		if from < 1500 {
			t.Errorf("resume asked for %s, but the first 1500 bytes were already on disk", r)
		}
	}
}

// TestResumeOfAFinishedFileFetchesNothing covers the part file that is already
// the whole file, which is what a rerun after a completed download looks like.
func TestResumeOfAFinishedFileFetchesNothing(t *testing.T) {
	body := testBody(2000)
	m := &mediaServer{body: body}
	srv := httptest.NewServer(m)
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "out.bin")
	if err := os.WriteFile(dst, body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := newTestClient(t).DownloadToFile(context.Background(), srv.URL, dst, DownloadOptions{
		Total: 2000, ChunkSize: 1000, Resume: true,
	}); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if n := len(m.ranges()); n != 0 {
		t.Errorf("sent %d requests for a file that was already complete", n)
	}
}

// TestProgressCountsUpToTheStatedTotal covers the promise contentLength makes:
// the total is known before the first byte, so the percentage is real rather
// than an estimate that jumps around.
func TestProgressCountsUpToTheStatedTotal(t *testing.T) {
	m := &mediaServer{body: testBody(4000)}
	srv := httptest.NewServer(m)
	defer srv.Close()

	var mu sync.Mutex
	var seen []DownloadProgress
	err := newTestClient(t).DownloadToFile(context.Background(), srv.URL, filepath.Join(t.TempDir(), "out.bin"), DownloadOptions{
		Total: 4000, ChunkSize: 1000, Workers: 2,
		OnProgress: func(p DownloadProgress) {
			mu.Lock()
			defer mu.Unlock()
			seen = append(seen, p)
		},
	})
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if len(seen) < 2 {
		t.Fatalf("got %d progress reports for a four chunk download", len(seen))
	}
	last := DownloadProgress{}
	for i, p := range seen {
		if p.Total != 4000 {
			t.Errorf("report %d states total %d, want the contentLength 4000", i, p.Total)
		}
		if p.Downloaded < last.Downloaded {
			t.Errorf("report %d went backwards: %d after %d", i, p.Downloaded, last.Downloaded)
		}
		last = p
	}
	if last.Downloaded != 4000 {
		t.Errorf("final report is %d of 4000", last.Downloaded)
	}
}

// TestURLExpiry reads the deadline the CDN enforces. Both spellings are real:
// the query parameter is what a player response hands back, and the path
// segment is what some hosts use for the same number.
func TestURLExpiry(t *testing.T) {
	cases := []struct {
		raw  string
		want int64
	}{
		{"https://rr3---sn-x.googlevideo.com/videoplayback?expire=1786013780&ei=x", 1786013780},
		{"https://rr3---sn-x.googlevideo.com/videoplayback/expire/1786013780/ei/x", 1786013780},
		{"https://rr3---sn-x.googlevideo.com/videoplayback?ei=x", 0},
		{"https://rr3---sn-x.googlevideo.com/videoplayback?expire=nope", 0},
		{"", 0},
	}
	for _, c := range cases {
		got := urlExpiry(c.raw)
		if c.want == 0 {
			if !got.IsZero() {
				t.Errorf("urlExpiry(%q) = %v, want no deadline", c.raw, got)
			}
			continue
		}
		if got.Unix() != c.want {
			t.Errorf("urlExpiry(%q) = %d, want %d", c.raw, got.Unix(), c.want)
		}
	}
}
