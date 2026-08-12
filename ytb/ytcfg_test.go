package ytb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Every YouTube HTML page carries the ytcfg, so a run that has already read one
// holds the InnerTube key and has no reason to fetch the embed page for it.
// SeedYTCfg existed for exactly this and nothing called it, so ytb download read
// the watch page and then fetched /embed anyway: one wasted request, and with
// the default 1.5s pacing a wasted 1.5 seconds on top of it.
//
// The assertion is indirect on purpose. ytcfgFor only knows a harvest page for
// www.youtube.com and music.youtube.com, so for a test server's host it either
// answers from the seed or fails outright. There is no third outcome where it
// quietly refetches and the test still passes.
func TestReadingAPageSeedsTheKeySoNothingHarvestsItAgain(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(`<html><script>ytcfg.set({"INNERTUBE_API_KEY":"AIzaTestKeyNotReal",` +
			`"INNERTUBE_CLIENT_VERSION":"2.20990101.00.00","VISITOR_DATA":"CgtURVNUVklTSVRPUg%3D%3D"});</script></html>`))
	}))
	defer srv.Close()

	cfg := DefaultConfig()
	cfg.Delay = 0
	c := NewClient(cfg)

	if _, _, err := c.FetchPageData(context.Background(), srv.URL+"/watch?v=dQw4w9WgXcQ"); err != nil {
		t.Fatalf("fetch the page: %v", err)
	}
	if hits != 1 {
		t.Fatalf("the page was read %d times, want 1", hits)
	}

	got, err := c.ytcfgFor(context.Background(), hostOf(srv.URL))
	if err != nil {
		t.Fatalf("the key was on the page that was just read and ytcfgFor went looking for it anyway: %v", err)
	}
	if got.APIKey != "AIzaTestKeyNotReal" {
		t.Errorf("api key = %q", got.APIKey)
	}
	if got.ClientVersion != "2.20990101.00.00" {
		t.Errorf("client version = %q", got.ClientVersion)
	}
	if got.VisitorData != "CgtURVNUVklTSVRPUg%3D%3D" {
		t.Errorf("visitor data = %q", got.VisitorData)
	}
	if hits != 1 {
		t.Errorf("the server was hit %d times, so the key was fetched again after being read", hits)
	}
}

// A page with no ytcfg on it must not seed, or the first 404 or consent
// interstitial of a run would cache an empty key for the host and every
// InnerTube call after it would go out with nothing.
func TestAPageWithNoKeyOnItSeedsNothing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body>no config here</body></html>`))
	}))
	defer srv.Close()

	cfg := DefaultConfig()
	cfg.Delay = 0
	c := NewClient(cfg)

	if _, _, err := c.FetchPageData(context.Background(), srv.URL+"/watch?v=dQw4w9WgXcQ"); err != nil {
		t.Fatalf("fetch the page: %v", err)
	}
	if _, err := c.ytcfgFor(context.Background(), hostOf(srv.URL)); err == nil {
		t.Error("a page carrying no key seeded the cache, so the host now has an empty key cached for the run")
	}
}
