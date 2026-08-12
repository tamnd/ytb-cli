package ytb

import (
	"context"
	"testing"

	"github.com/tamnd/any-cli/kit/errs"
)

// video, search and suggest take a variadic argument, so kit does not insist on
// one and the handler is what has to. Before this, `ytb video` with nothing
// after it emitted no records and exited 3, and `ytb search` with no terms went
// as far as asking YouTube for the empty string. Every sibling command exits 2.
func TestNothingToWorkOnIsAUsageError(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		call func() error
	}{
		{"video", func() error {
			return getVideo(ctx, videoRef{}, func(*Video) error { return nil })
		}},
		{"search", func() error {
			return search(ctx, searchRef{}, func(any) error { return nil })
		}},
		{"suggest", func() error {
			return suggest(ctx, suggestRef{}, func(Suggestion) error { return nil })
		}},
		// Whitespace counts as nothing, since a shell that expanded an empty
		// variable is the way this happens in a script.
		{"search with blank terms", func() error {
			return search(ctx, searchRef{Query: []string{"", "  "}}, func(any) error { return nil })
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if err == nil {
				t.Fatal("no error at all")
			}
			if got := errs.ExitCode(err); got != 2 {
				t.Errorf("exit %d for %q, want 2", got, err)
			}
		})
	}
}

// notFoundReason prefers YouTube's sentence, because it is the one that says
// which of the several ways an id is gone.
func TestNotFoundReason(t *testing.T) {
	cases := []struct {
		in   Playability
		want string
	}{
		{Playability{Status: "ERROR", Reason: "Video unavailable"}, "video unavailable"},
		{Playability{Status: "ERROR", Reason: "This video is no longer available."}, "this video is no longer available"},
		{Playability{Status: "ERROR"}, "no video at that address"},
	}
	for _, tc := range cases {
		if got := notFoundReason(&tc.in); got != tc.want {
			t.Errorf("notFoundReason(%q) = %q, want %q", tc.in.Reason, got, tc.want)
		}
	}
}
