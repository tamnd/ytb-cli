package ytb

import (
	"strings"
	"testing"
)

// TestMessageRefusalOnThePostsTab reads the real posts tab response for
// @Computerphile. The tab is selected, the status was 200, the body was 40 KB and
// there were no alerts anywhere in it, so before this the read came back as an
// empty list and the tool said the channel has no posts. It has 30.
func TestMessageRefusalOnThePostsTab(t *testing.T) {
	resp := loadFixture(t, "posts_refusal.json")

	if len(alertRenderers(resp)) != 0 {
		t.Fatal("the fixture carries alerts, so it no longer stands for the case where the refusal is only in a messageRenderer")
	}
	if r := alertRefusal(resp, "posts for @Computerphile", "browse"); r != nil {
		t.Errorf("alertRefusal found something in a response with no alerts: %v", r)
	}

	r := messageRefusal(resp, "posts for @Computerphile", "browse")
	if r == nil {
		t.Fatal("the posts tab refusal was not found, so an empty list would be reported instead")
	}
	if !IsRefusal(r) {
		t.Error("the result is not recognised as a refusal, so it would be retried and exit 1")
	}
	// YouTube's own sentence, both halves of it.
	if !strings.Contains(r.Message, "Posts aren't currently available on this device") {
		t.Errorf("the refusal does not quote YouTube: %q", r.Message)
	}
	if !strings.Contains(r.Message, "from your desktop or other supported devices") {
		t.Errorf("the subtext was dropped, which is the half that says what to do: %q", r.Message)
	}
	if r.Remedy == "" {
		t.Error("no remedy on a phrase that has one measured")
	}
	if r.Surface != "browse" {
		t.Errorf("surface = %q, want browse", r.Surface)
	}
}

// TestMessageRefusalIgnoresOrdinaryMessages asserts the end of a list stays the
// end of a list. A messageRenderer is also how YouTube says "No more results",
// and treating every one as a refusal would turn a finished read into an error.
func TestMessageRefusalIgnoresOrdinaryMessages(t *testing.T) {
	ordinary := []string{
		"No more results",
		"This channel has no videos.",
		"Sorry, no results found. Try different keywords.",
	}
	for _, text := range ordinary {
		resp := map[string]any{
			"contents": map[string]any{
				"messageRenderer": map[string]any{
					"text": map[string]any{"runs": []any{map[string]any{"text": text}}},
				},
			},
		}
		if r := messageRefusal(resp, "videos for UCx", "browse"); r != nil {
			t.Errorf("%q was read as a refusal: %v", text, r)
		}
	}
}
