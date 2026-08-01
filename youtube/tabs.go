package youtube

import (
	"encoding/base64"
	"net/url"
	"sort"
	"strings"

	"github.com/tamnd/any-cli/kit/errs"
)

// tabs.go reads a channel's tab strip off the response and picks a tab by name.
//
// Two things go wrong if a tab is picked any other way.
//
// A hardcoded params blob goes stale. The posts tab on @RickAstleyYT answers to
// EgVwb3N0c_IGBAoCSgA%3D, and this package shipped EgVwb3N0c_IGBAoCEgA, so a
// hardcoded match found nothing and the tool would have reported no posts tab on
// a channel that has one.
//
// A title match never worked. Fetched from an address YouTube reads as
// Vietnamese, the strip comes back "Trang chu / Video / Shorts / Phat truc tiep /
// Danh sach phat". Even in English the titles lie about the slug: the tab titled
// Home is featured and the one titled Live is streams.
//
// What is stable is the tab's own name, and the response states it twice: as the
// last segment of the web url, and as field 2 of the params protobuf. Both are
// read here and a test asserts they agree.

// Tab is one entry in a channel's tab strip.
type Tab struct {
	// Slug is the language independent tab name: featured, videos, shorts,
	// streams, releases, playlists, posts, search.
	Slug string `json:"slug"`
	// Title is what a person sees. It is translated, so it is for display only.
	Title string `json:"title,omitempty"`
	// BrowseID and Params are what a browse call needs.
	BrowseID string `json:"browse_id,omitempty"`
	Params   string `json:"params,omitempty"`
	Selected bool   `json:"selected,omitempty"`
}

// tabRendererKeys are the renderers a tab arrives as. The search tab is an
// expandableTabRenderer and carries the same fields, so a reader that only knows
// tabRenderer loses it.
var tabRendererKeys = []string{"tabRenderer", "expandableTabRenderer"}

// ChannelTabs returns every tab in resp, in the order the strip shows them where
// the response says so, and otherwise sorted by slug for a stable answer.
func ChannelTabs(resp map[string]any) []Tab {
	var tabs []Tab
	seen := map[string]bool{}
	for _, key := range tabRendererKeys {
		walkJSON(resp, func(m map[string]any) {
			tr, ok := m[key].(map[string]any)
			if !ok {
				return
			}
			be := mapValue(mapValue(tr, "endpoint"), "browseEndpoint")
			if be == nil {
				return
			}
			t := Tab{
				Title:    extractText(tr["title"]),
				BrowseID: stringValue(be["browseId"]),
				Params:   stringValue(be["params"]),
				Selected: boolValue(tr["selected"]),
			}
			if t.Title == "" {
				t.Title = stringValue(tr["title"])
			}
			t.Slug = tabSlug(tr)
			if t.Slug == "" || seen[t.Slug] {
				return
			}
			seen[t.Slug] = true
			tabs = append(tabs, t)
		})
	}
	sort.SliceStable(tabs, func(i, j int) bool { return tabs[i].Slug < tabs[j].Slug })
	return tabs
}

// tabSlugFor maps the word a person types to the tab's own name. They mostly
// agree, and where they do not it is because YouTube renamed a tab in the UI and
// not in the data: the tab titled Live is streams, and Home is featured.
func tabSlugFor(word string) string {
	switch strings.ToLower(word) {
	case "live":
		return "streams"
	case "home":
		return "featured"
	case "community":
		return "posts"
	default:
		return strings.ToLower(word)
	}
}

// FindTab returns the tab named slug.
func FindTab(resp map[string]any, slug string) (Tab, bool) {
	for _, t := range ChannelTabs(resp) {
		if t.Slug == slug {
			return t, true
		}
	}
	return Tab{}, false
}

// AssertTab checks that the response actually is the tab that was asked for, and
// otherwise reports the tabs the channel really has.
//
// This runs after every tab read because YouTube does not error when a tab is
// missing, it serves the home tab instead. A channel with no shorts answers a
// shorts read with its featured page, and the videos on it are real videos, so
// nothing downstream can tell that the wrong question was answered. The exit code
// is 3, no results, because that is what happened: the tab asked for is not there.
func AssertTab(resp map[string]any, want string) error {
	tabs := ChannelTabs(resp)
	for _, t := range tabs {
		if t.Slug != want {
			continue
		}
		// A tab strip with nothing marked selected is normal on a continuation
		// response, and the tab being present is the claim being checked.
		if t.Selected || !anySelected(tabs) {
			return nil
		}
		return errs.NoResults("read the %s tab but YouTube selected %s; tabs on this channel: %s",
			want, selectedSlug(tabs), strings.Join(slugs(tabs), ", "))
	}
	if len(tabs) == 0 {
		return errs.NoResults("read the %s tab but the response carries no tab strip", want)
	}
	return errs.NoResults("this channel has no %s tab; it has: %s", want, strings.Join(slugs(tabs), ", "))
}

func anySelected(tabs []Tab) bool {
	for _, t := range tabs {
		if t.Selected {
			return true
		}
	}
	return false
}

func selectedSlug(tabs []Tab) string {
	for _, t := range tabs {
		if t.Selected {
			return t.Slug
		}
	}
	return "nothing"
}

func slugs(tabs []Tab) []string {
	out := make([]string, 0, len(tabs))
	for _, t := range tabs {
		out = append(out, t.Slug)
	}
	return out
}

// tabSlug reads a tab's name out of the response. The params blob is preferred
// over the url because the blob is what the server acts on and the url is a route
// for the browser.
func tabSlug(tr map[string]any) string {
	ep := mapValue(tr, "endpoint")
	if s := paramsSlug(stringValue(mapValue(ep, "browseEndpoint")["params"])); s != "" {
		return s
	}
	return urlSlug(stringValue(mapValue(mapValue(ep, "commandMetadata"), "webCommandMetadata")["url"]))
}

// urlSlug returns the last path segment of a channel tab url, so
// /@RickAstleyYT/videos gives videos. A bare channel url gives "" rather than the
// handle, because a handle is not a tab name.
func urlSlug(raw string) string {
	raw = strings.TrimSuffix(raw, "/")
	i := strings.LastIndex(raw, "/")
	if i < 0 {
		return ""
	}
	seg := raw[i+1:]
	if seg == "" || strings.HasPrefix(seg, "@") || strings.HasPrefix(seg, "UC") {
		return ""
	}
	return seg
}

// paramsSlug reads the tab name out of a browse params blob.
//
// The blob is a protobuf and field 2 is the tab name in plain bytes:
// EgZ2aWRlb3PyBgQKAjoA decodes to 12 06 "videos" f2 06 04 0a 02 3a 00. That name
// is the same in every language, which is the whole reason for decoding it. This
// reads a blob and never writes one.
func paramsSlug(params string) string {
	if params == "" {
		return ""
	}
	// The blob arrives percent encoded inside the JSON, so %3D padding has to come
	// back before base64 will take it.
	if unescaped, err := url.QueryUnescape(params); err == nil {
		params = unescaped
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(params, "="))
	if err != nil {
		return ""
	}
	name := protoStringField(raw, 2)
	for _, r := range name {
		if r < 'a' || r > 'z' {
			return ""
		}
	}
	return name
}

// protoStringField returns the bytes of the given protobuf field number as a
// string, skipping the fields before it. This is the smallest reader that answers
// the question and pulls in no dependency; anything more would be a protobuf
// library for one field of one blob.
func protoStringField(b []byte, field int) string {
	for len(b) > 0 {
		tag, n := protoVarint(b)
		if n == 0 {
			return ""
		}
		b = b[n:]
		num, wire := int(tag>>3), int(tag&7)
		switch wire {
		case 0: // varint
			_, n := protoVarint(b)
			if n == 0 {
				return ""
			}
			b = b[n:]
		case 1: // fixed64
			if len(b) < 8 {
				return ""
			}
			b = b[8:]
		case 2: // length delimited
			size, n := protoVarint(b)
			if n == 0 || uint64(len(b[n:])) < size {
				return ""
			}
			val := b[n : n+int(size)]
			if num == field {
				return string(val)
			}
			b = b[n+int(size):]
		case 5: // fixed32
			if len(b) < 4 {
				return ""
			}
			b = b[4:]
		default:
			return ""
		}
	}
	return ""
}

// protoVarint decodes a varint and returns it with the bytes consumed, or 0
// consumed when the encoding runs off the end.
func protoVarint(b []byte) (uint64, int) {
	var v uint64
	for i := 0; i < len(b) && i < 10; i++ {
		v |= uint64(b[i]&0x7f) << (7 * i)
		if b[i] < 0x80 {
			return v, i + 1
		}
	}
	return 0, 0
}
