package ytb

import (
	"sort"
	"strings"
)

// continuation.go finds a paging token by searching for the key it lives under,
// never by walking a fixed path.
//
// There are four shapes in the wild and they are all live:
//
//	continuationItemRenderer.continuationEndpoint.continuationCommand.token
//	continuationItemRenderer.button.buttonRenderer.command.continuationCommand.token
//	continuationItemViewModel.continuationCommand.innertubeCommand.continuationCommand.token
//	continuations[].nextContinuationData.continuation
//
// A /next response for dQw4w9WgXcQ carries the first two at once, on the same
// renderer, holding the same token. A parser written against one path would page
// some lists and silently stop after page one on the others, which reads as a
// short playlist rather than as a bug.
//
// Searching for the key is only half the job. A response carries tokens that are
// not "more of this list": a search response for "lofi hip hop" carries seven
// tokens, one for the results and six for the filter chips in the header, and a
// channel page carries the grid's token plus the about panel's. Following a chip
// token as if it were the next page changes the query mid-read. So a candidate
// has to be reached through a marker that means continue, and must not be reached
// through one that means start something else.
//
// Nothing here builds a token. Tokens are opaque server state, and a constructed
// one returned "This video isn't available anymore" for a healthy video. The
// policy test proves the package holds no token builder.

// ContinuationToken is one token found in a response, with the keys walked to
// reach it. The path is what makes the choice between several tokens explainable
// rather than a coin toss.
type ContinuationToken struct {
	Token string
	Path  string
}

// listMarkers are the keys that mean "the token below me continues the list you
// are already reading".
var listMarkers = []string{
	"continuationItemRenderer",
	"continuationItemViewModel",
	"nextContinuationData",
	"reloadContinuationData",
}

// foreignMarkers are the keys that mean "the token below me starts a different
// list". A chip is a filter and an engagement panel is its own feed, so both
// carry real tokens that are the wrong answer to "what is the next page".
var foreignMarkers = []string{
	"chipCloudChipRenderer",
	"chipViewModel",
	"chipBarViewModel",
	"feedFilterChipBarRenderer",
	"searchHeaderRenderer",
	"showEngagementPanelEndpoint",
	// A reply thread is its own list, and it sits inside the comment it belongs to.
	// Its token pages that thread, not the comment section, so following it as the
	// next page of comments would reread one conversation forever.
	"commentRepliesRenderer",
	"commentRepliesViewModel",
}

// FindContinuationToken returns the token that pages the list in root, or "" when
// the list ends here.
func FindContinuationToken(root any) string {
	best := ""
	bestPath := ""
	for _, c := range FindContinuationTokens(root) {
		if !hasMarker(c.Path, listMarkers) || hasMarker(c.Path, foreignMarkers) {
			continue
		}
		// After filtering there is normally exactly one candidate, and where there
		// are two they have been the same token on the same renderer. The tie is
		// broken on the path rather than on arrival order because a Go map has no
		// order, and a token picked by map iteration would page a different list on
		// every run.
		if bestPath == "" || c.Path < bestPath {
			best, bestPath = c.Token, c.Path
		}
	}
	return best
}

// FindContinuationTokenUnder returns the first token whose path walked through
// every one of markers, ignoring the list and foreign rules. This is how the about
// panel is read: its token sits under showEngagementPanelEndpoint, which
// FindContinuationToken is right to skip and this is right to ask for.
//
// More than one marker is how two panels of the same kind are told apart. One
// marker is not always enough: @Computerphile's featured tab carries a shelf of
// Brady Haran's other channels, and that shelf is an engagement panel too, so
// asking for showEngagementPanelEndpoint alone found the shelf and the about read
// came back with a list of channels.
func FindContinuationTokenUnder(root any, markers ...string) string {
	best := ""
	bestPath := ""
	for _, c := range FindContinuationTokens(root) {
		missing := false
		for _, m := range markers {
			if !hasMarker(c.Path, []string{m}) {
				missing = true
				break
			}
		}
		if missing {
			continue
		}
		if bestPath == "" || c.Path < bestPath {
			best, bestPath = c.Token, c.Path
		}
	}
	return best
}

// FindContinuationTokens returns every token in root, sorted by path. Both
// callers above filter this list; a test reads it whole to prove all four shapes
// are found.
func FindContinuationTokens(root any) []ContinuationToken {
	var out []ContinuationToken
	walkPaths(root, "", func(path string, key string, m map[string]any) {
		switch key {
		case "continuationCommand":
			if t := stringValue(m["token"]); t != "" {
				out = append(out, ContinuationToken{Token: t, Path: path})
			}
		case "nextContinuationData", "reloadContinuationData":
			if t := stringValue(m["continuation"]); t != "" {
				out = append(out, ContinuationToken{Token: t, Path: path})
			}
		}
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// hasMarker reports whether path walked through any of keys.
func hasMarker(path string, keys []string) bool {
	for _, k := range keys {
		if strings.Contains(path, "/"+k+"/") || strings.HasSuffix(path, "/"+k) {
			return true
		}
	}
	return false
}

// walkPaths walks a decoded JSON tree and calls visit for every object, with the
// slash-joined key path that reached it and the key it was stored under. Map keys
// are visited in sorted order so two runs over the same response walk it the same
// way. Array indices are not in the path: what matters is which renderers were
// passed through, not which slot of a list a renderer sat in.
func walkPaths(node any, path string, visit func(path, key string, m map[string]any)) {
	switch v := node.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			child := path + "/" + k
			if m, ok := v[k].(map[string]any); ok {
				visit(child, k, m)
			}
			walkPaths(v[k], child, visit)
		}
	case []any:
		for _, item := range v {
			walkPaths(item, path, visit)
		}
	}
}
