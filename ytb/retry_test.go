package ytb

import (
	"context"
	"net/http"
	"net/http/httptest"
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
