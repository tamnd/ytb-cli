package youtube

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The four shapes, written out as the responses carry them. Fixture files would
// be more faithful but less readable, and what is being tested here is only which
// path the token sits on.
const (
	shapeEndpoint = `{"contents":[{"continuationItemRenderer":{
		"continuationEndpoint":{"continuationCommand":{"token":"TOK_ENDPOINT"}}}}]}`

	shapeButton = `{"contents":[{"continuationItemRenderer":{
		"button":{"buttonRenderer":{"command":{"continuationCommand":{"token":"TOK_BUTTON"}}}}}}]}`

	shapeViewModel = `{"contents":[{"continuationItemViewModel":{
		"continuationCommand":{"innertubeCommand":{"continuationCommand":{"token":"TOK_VIEWMODEL"}}}}}]}`

	shapeLegacy = `{"contents":[{"itemSectionRenderer":{
		"continuations":[{"nextContinuationData":{"continuation":"TOK_LEGACY"}}]}}]}`
)

func TestFindContinuationTokenAllFourShapes(t *testing.T) {
	cases := []struct {
		name, raw, want string
	}{
		{"continuationEndpoint", shapeEndpoint, "TOK_ENDPOINT"},
		{"button command", shapeButton, "TOK_BUTTON"},
		{"nested view model", shapeViewModel, "TOK_VIEWMODEL"},
		{"legacy nextContinuationData", shapeLegacy, "TOK_LEGACY"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FindContinuationToken(decode(t, tc.raw)); got != tc.want {
				t.Fatalf("token = %q, want %q", got, tc.want)
			}
		})
	}
}

// A filter chip carries a real token that starts a different query. Following it
// as the next page silently changes what is being read, which is worse than
// stopping.
func TestFindContinuationTokenSkipsFilterChips(t *testing.T) {
	raw := `{
		"header":{"searchHeaderRenderer":{"chipBar":{"chipCloudRenderer":{"chips":[
			{"chipCloudChipRenderer":{"navigationEndpoint":{"continuationCommand":{"token":"TOK_CHIP"}}}}]}}}},
		"contents":{"twoColumnSearchResultsRenderer":{"primaryContents":{"sectionListRenderer":{"contents":[
			{"continuationItemRenderer":{"continuationEndpoint":{"continuationCommand":{"token":"TOK_RESULTS"}}}}]}}}}}`
	if got := FindContinuationToken(decode(t, raw)); got != "TOK_RESULTS" {
		t.Fatalf("token = %q, want TOK_RESULTS", got)
	}
}

// A channel page carries the grid's token and the about panel's at once. Picking
// between them by map iteration order, which is what a walkJSON search does, pages
// the about panel on some runs and the videos on others.
func TestFindContinuationTokenSkipsEngagementPanel(t *testing.T) {
	root := map[string]any{}
	for k, v := range aboutPanelTree("TOK_ABOUT") {
		root[k] = v
	}
	for k, v := range channelGridTree("TOK_GRID") {
		root[k] = v
	}
	// Run it more than once: the bug this guards against only showed up on some map
	// iteration orders, so one pass proves nothing.
	for i := 0; i < 20; i++ {
		if got := FindContinuationToken(root); got != "TOK_GRID" {
			t.Fatalf("pass %d: token = %q, want TOK_GRID", i, got)
		}
	}
	if got := FindContinuationTokenUnder(root, "showEngagementPanelEndpoint"); got != "TOK_ABOUT" {
		t.Fatalf("about token = %q, want TOK_ABOUT", got)
	}
}

func TestFindContinuationTokenEmptyWhenListEnds(t *testing.T) {
	raw := `{"contents":[{"videoRenderer":{"videoId":"dQw4w9WgXcQ"}}]}`
	if got := FindContinuationToken(decode(t, raw)); got != "" {
		t.Fatalf("token = %q, want empty", got)
	}
}

// The channel page's two token sites, built by nesting rather than by typing
// fifteen levels of JSON braces. The paths are the real ones, read off
// @RickAstleyYT's page.
func aboutPanelTree(token string) map[string]any {
	return nest([]string{
		"header", "pageHeaderRenderer", "content", "pageHeaderViewModel", "description",
		"descriptionPreviewViewModel", "rendererContext", "commandContext", "onTap",
		"innertubeCommand", "showEngagementPanelEndpoint", "engagementPanel",
		"engagementPanelSectionListRenderer", "content", "sectionListRenderer", "contents",
	}, []any{
		map[string]any{"itemSectionRenderer": map[string]any{
			"contents": []any{continuationItem(token)},
		}},
	})
}

// shelfPanelTree is the other engagement panel on a channel page: a shelf whose
// title opens a panel of its own. The path is the one measured on
// @Computerphile's featured tab.
func shelfPanelTree(token string) map[string]any {
	return nest([]string{"contents", "twoColumnBrowseResultsRenderer", "tabs"}, []any{
		map[string]any{"tabRenderer": nest([]string{
			"content", "sectionListRenderer", "contents",
		}, []any{
			map[string]any{"itemSectionRenderer": map[string]any{"contents": []any{
				map[string]any{"shelfRenderer": nest([]string{
					"endpoint", "showEngagementPanelEndpoint", "engagementPanel",
					"engagementPanelSectionListRenderer", "content", "sectionListRenderer", "contents",
				}, []any{
					map[string]any{"itemSectionRenderer": map[string]any{
						"contents": []any{continuationItem(token)},
					}},
				})},
			}}},
		})},
	})
}

func channelGridTree(token string) map[string]any {
	return nest([]string{"contents", "twoColumnBrowseResultsRenderer", "tabs"}, []any{
		map[string]any{"tabRenderer": nest([]string{"content", "richGridRenderer", "contents"},
			[]any{continuationItem(token)})},
	})
}

func continuationItem(token string) map[string]any {
	return map[string]any{"continuationItemRenderer": map[string]any{
		"continuationEndpoint": map[string]any{
			"continuationCommand": map[string]any{"token": token},
		},
	}}
}

// nest wraps leaf in the given keys, outermost first.
func nest(keys []string, leaf any) map[string]any {
	for i := len(keys) - 1; i > 0; i-- {
		leaf = map[string]any{keys[i]: leaf}
	}
	return map[string]any{keys[0]: leaf}
}

func decode(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("bad fixture: %v", err)
	}
	return m
}

func loadFixture(t *testing.T, name string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("parse fixture %s: %v", name, err)
	}
	return m
}
