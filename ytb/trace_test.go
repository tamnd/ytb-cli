package ytb

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestTraceNeverPrintsCredentials is the test that matters here. The trace
// exists to be pasted into a bug report, and a session cookie is the user's
// account, so a header list that grew a Cookie by accident would hand it over.
func TestTraceNeverPrintsCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Set-Cookie", "SID=server-secret; Path=/")
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	var buf bytes.Buffer
	c := NewClient(DefaultConfig())
	c.SetTrace(&buf, 2)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/watch?v=abc", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Cookie", "SID=user-secret")
	req.Header.Set("Authorization", "Bearer user-token")
	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	for _, secret := range []string{"user-secret", "user-token", "server-secret", "Cookie", "Authorization"} {
		if strings.Contains(buf.String(), secret) {
			t.Errorf("trace leaked %q:\n%s", secret, buf.String())
		}
	}
}

// TestTraceLevels holds the split between the two verbosities: -v is meant to be
// read down a screen, -vv is meant to be replayable with curl. A -v that printed
// the query would be a thousand characters of googlevideo signature per line.
func TestTraceLevels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	get := func(level int) string {
		var buf bytes.Buffer
		c := NewClient(DefaultConfig())
		c.SetTrace(&buf, level)
		req, err := http.NewRequest(http.MethodGet, srv.URL+"/watch?v=abc&sig=LONGSIGNATURE", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Range", "bytes=0-1048575")
		resp, err := c.http.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return buf.String()
	}

	one := get(1)
	switch {
	case strings.Contains(one, "LONGSIGNATURE"):
		t.Errorf("-v kept the query:\n%s", one)
	case !strings.Contains(one, "v=abc"):
		t.Errorf("-v dropped the video id, so the line says nothing about which watch:\n%s", one)
	case strings.Contains(one, "Range="):
		t.Errorf("-v printed headers, which is what -vv is for:\n%s", one)
	case strings.Count(one, "\n") != 1:
		t.Errorf("want one line per request, got:\n%s", one)
	}

	two := get(2)
	switch {
	case !strings.Contains(two, "LONGSIGNATURE"):
		t.Errorf("-vv dropped the query, so the request cannot be replayed:\n%s", two)
	case !strings.Contains(two, "Range=bytes=0-1048575"):
		t.Errorf("-vv did not show the Range, which is the whole point on a download:\n%s", two)
	}
}

// TestTraceOffByDefault: a client nobody asked to trace writes nothing, and
// SetTrace with level 0 is the same as not calling it.
func TestTraceOffByDefault(t *testing.T) {
	var buf bytes.Buffer
	c := NewClient(DefaultConfig())
	c.SetTrace(&buf, 0)
	if _, ok := c.http.Transport.(*traceTransport); ok {
		t.Fatal("level 0 installed a trace transport")
	}
	if buf.Len() != 0 {
		t.Fatalf("wrote %q with tracing off", buf.String())
	}
}
