package ytb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/tamnd/any-cli/kit/errs"
)

// A run that YouTube rate limited into the ground and a run with no network used
// to exit 1 alike, which is nothing for a script to branch on. These are the two
// exit codes the troubleshooting page promises.
func TestTheFailureThatSurvivesTheRetriesIsClassified(t *testing.T) {
	tooMany := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer tooMany.Close()

	broken := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	brokenURL := broken.URL
	broken.Close() // nothing is listening there now

	unavailable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer unavailable.Close()

	cases := []struct {
		name string
		url  string
		want int
	}{
		{"429 to the last attempt", tooMany.URL, 5},
		// A 5xx is left generic on purpose: it is not the transport failing and it
		// is not a rate limit, and calling it either would be a tidier table that
		// says something untrue.
		{"503 to the last attempt", unavailable.URL, 1},
		{"nothing listening", brokenURL, 8},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Delay = 0
			cfg.Retries = 0
			_, _, err := NewClient(cfg).Fetch(context.Background(), tc.url)
			if err == nil {
				t.Fatal("no error at all")
			}
			if got := errs.ExitCode(err); got != tc.want {
				t.Errorf("exit %d for %q, want %d", got, err, tc.want)
			}
		})
	}
}

// The client builds its own http.Transport for the connection limits, and a
// hand-built one starts with no Proxy at all where http.DefaultTransport reads
// the environment. Dropping it means HTTP_PROXY, HTTPS_PROXY and NO_PROXY are
// ignored, so on a machine that reaches the internet only through a proxy every
// request fails to connect and nothing in the error says the proxy was never
// tried.
func TestTheTransportReadsTheProxyEnvironment(t *testing.T) {
	tr, ok := NewClient(DefaultConfig()).HTTP().Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport is %T, want *http.Transport", NewClient(DefaultConfig()).HTTP().Transport)
	}
	if tr.Proxy == nil {
		t.Fatal("the transport has no Proxy, so HTTPS_PROXY is ignored")
	}
	// It has to be the standard library's reader and not some other function.
	// Setting HTTPS_PROXY here and calling tr.Proxy would be the better test and
	// cannot work: net/http reads the proxy environment once per process behind a
	// sync.Once, so by the time this runs in the full suite the variables have
	// already been read and a t.Setenv after that changes nothing.
	if got, want := reflect.ValueOf(tr.Proxy).Pointer(), reflect.ValueOf(http.ProxyFromEnvironment).Pointer(); got != want {
		t.Error("the transport's Proxy is not http.ProxyFromEnvironment, so the proxy environment is not what decides")
	}
}
