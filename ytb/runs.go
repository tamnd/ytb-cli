package ytb

import (
	"net/url"
	"strings"
	"unicode/utf16"
)

// runs.go reads an attributed text node into runs that keep their endpoints.
//
// A video description is two things at once. videoDetails.shortDescription is the
// plain text, authoritative and already unescaped, and it is what `description`
// holds. The linked version lives in ytInitialData under
// videoSecondaryInfoRenderer.attributedDescription, and it is where every link,
// @mention, #hashtag and chapter timestamp in the description actually is. A
// description flattened to a string loses all of them, and a regex over the text
// invents some that are not there: "1:23" in a sentence is not a chapter, and
// "example.com" typed as prose is not a link.
//
// The newer renderers dropped runs[] for a flat content string plus commandRuns,
// where each command is a (startIndex, length) span into that string. So the runs
// are reconstructed by slicing, and the slicing is the part that bites.
//
// The offsets are UTF-16 code units, not bytes and not runes.
//
// This is not documented anywhere and it is invisible on most videos, because
// most descriptions are pure BMP text where all three counts agree. dQw4w9WgXcQ's
// description has one emoji in it, a single U+1F4DA, and every span after that
// emoji comes out one position short: the first link reads "ttps://linktr.ee/..."
// with the h eaten and a trailing newline glued on, and the first hashtag reads
// "RickAstleyNever\n" instead of "#RickAstleyNever". One astral character in the
// text shifts every offset after it by one, and two shift it by two. Sliced as
// UTF-16 the same spans come out exactly "https://linktr.ee/rickastleynever" and
// "#RickAstleyNever".
//
// A parser that gets this wrong does not fail. It stores a URL that 404s and a
// hashtag nobody follows, which is worse.

// TextRun is one run of an attributed text node: the text, and what the endpoint
// under it meant. A plain stretch of prose is a run with no kind.
type TextRun struct {
	Text string `json:"text"`
	// Kind is what the run points at: link, mention, hashtag, timestamp, or empty
	// for plain text.
	Kind string `json:"kind,omitempty"`
	// URL is the destination of a link run, unwrapped from youtube.com/redirect.
	URL string `json:"url,omitempty"`
	// ChannelID is set on a mention run.
	ChannelID string `json:"channel_id,omitempty"`
	// Hashtag is the tag on a hashtag run, without the #.
	Hashtag string `json:"hashtag,omitempty"`
	// StartSeconds is the offset a timestamp run jumps to.
	StartSeconds int `json:"start_seconds,omitempty"`
}

// The four kinds a run can point at.
const (
	RunLink      = "link"
	RunMention   = "mention"
	RunHashtag   = "hashtag"
	RunTimestamp = "timestamp"
)

// Link is one outbound link with the text that stood for it. Both are kept because
// they differ: the visible text is often a truncated "example.com/thi..." while the
// endpoint carries the whole destination.
type Link struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

// ParseAttributedRuns reads an attributedDescription node into runs, in document
// order, with the plain stretches between commands kept as runs of their own so
// the concatenated text is the original text.
func ParseAttributedRuns(node any) []TextRun {
	m, ok := node.(map[string]any)
	if !ok {
		return nil
	}
	content := stringValue(m["content"])
	if content == "" {
		return nil
	}
	// One conversion for the whole node. utf16.Encode is what YouTube's own
	// offsets are counted in; see the file comment for what happens without it.
	units := utf16.Encode([]rune(content))
	cut := func(start, length int) string {
		if start < 0 || length <= 0 || start >= len(units) {
			return ""
		}
		end := start + length
		if end > len(units) {
			end = len(units)
		}
		return string(utf16.Decode(units[start:end]))
	}

	var runs []TextRun
	at := 0
	for _, cr := range attributedCommands(m["commandRuns"]) {
		if cr.start > at {
			runs = append(runs, TextRun{Text: cut(at, cr.start-at)})
		}
		run := runFromCommand(cr.command)
		run.Text = cut(cr.start, cr.length)
		if run.Text != "" {
			runs = append(runs, run)
		}
		if end := cr.start + cr.length; end > at {
			at = end
		}
	}
	if at < len(units) {
		runs = append(runs, TextRun{Text: cut(at, len(units)-at)})
	}
	return runs
}

// attributedCommand is one commandRuns entry, already sorted and bounds checked.
type attributedCommand struct {
	start, length int
	command       map[string]any
}

// attributedCommands reads commandRuns in ascending start order. The response has
// them in order today, and sorting costs nothing next to trusting that.
func attributedCommands(v any) []attributedCommand {
	var out []attributedCommand
	for _, item := range arrayValue(v) {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		length := int(int64Value(m["length"]))
		if length <= 0 {
			continue
		}
		out = append(out, attributedCommand{
			start:   int(int64Value(m["startIndex"])),
			length:  length,
			command: innertubeCommand(m["onTap"]),
		})
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].start < out[j-1].start; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// innertubeCommand unwraps the onTap wrapper around a command.
func innertubeCommand(v any) map[string]any {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	if inner := mapValue(m, "innertubeCommand"); inner != nil {
		return inner
	}
	return m
}

// runFromCommand reads what a command means. The four shapes are all live in one
// description on one video, so none of them is a legacy path.
func runFromCommand(cmd map[string]any) TextRun {
	if cmd == nil {
		return TextRun{}
	}
	if ep := mapValue(cmd, "urlEndpoint"); ep != nil {
		return TextRun{Kind: RunLink, URL: unwrapRedirect(stringValue(ep["url"]))}
	}
	if ep := mapValue(cmd, "watchEndpoint"); ep != nil {
		// A chapter timestamp. startTimeSeconds is 0 on a link to the start of the
		// video, which is a real timestamp, so the kind is set either way.
		return TextRun{Kind: RunTimestamp, StartSeconds: int(int64Value(ep["startTimeSeconds"]))}
	}
	if ep := mapValue(cmd, "browseEndpoint"); ep != nil {
		browseID := stringValue(ep["browseId"])
		webURL := commandWebURL(cmd)
		// A hashtag and a mention are the same endpoint with different browseIds.
		// FEhashtag carries the tag in an opaque params blob, and the tag is also
		// in the web url as /hashtag/<tag>, which needs no protobuf decoded.
		if browseID == "FEhashtag" {
			return TextRun{Kind: RunHashtag, Hashtag: hashtagFromURL(webURL)}
		}
		if strings.HasPrefix(browseID, "UC") {
			return TextRun{Kind: RunMention, ChannelID: browseID, URL: webURL}
		}
	}
	return TextRun{}
}

// commandWebURL reads commandMetadata.webCommandMetadata.url, absolute.
func commandWebURL(cmd map[string]any) string {
	u := stringValue(mapValue(mapValue(cmd, "commandMetadata"), "webCommandMetadata")["url"])
	if u == "" {
		return ""
	}
	if strings.HasPrefix(u, "/") {
		return BaseURL + u
	}
	return u
}

// hashtagFromURL pulls the tag out of a /hashtag/<tag> path.
func hashtagFromURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	_, tag, ok := strings.Cut(strings.Trim(u.Path, "/"), "hashtag/")
	if !ok {
		return ""
	}
	if decoded, err := url.PathUnescape(tag); err == nil {
		return decoded
	}
	return tag
}

// RunLinks returns the outbound links in a set of runs, in order, one per run.
// Repeats are kept: a description that lists the same store twice linked it twice.
func RunLinks(runs []TextRun) []Link {
	var out []Link
	for _, r := range runs {
		if r.Kind == RunLink && r.URL != "" {
			out = append(out, Link{Text: r.Text, URL: r.URL})
		}
	}
	return out
}

// RunMentions returns the channel ids mentioned in a set of runs, deduplicated in
// first-mention order. A mention names a channel, and a channel named twice is one
// edge in the graph.
func RunMentions(runs []TextRun) []string {
	var out []string
	for _, r := range runs {
		if r.Kind == RunMention && r.ChannelID != "" {
			out = appendOnce(out, r.ChannelID)
		}
	}
	return out
}

// RunHashtags returns the tags in a set of runs, deduplicated in first-use order,
// without the #. These come from hashtag endpoints, so a "#" typed in prose is not
// one and a tag YouTube linked is, whatever it looks like.
func RunHashtags(runs []TextRun) []string {
	var out []string
	for _, r := range runs {
		if r.Kind == RunHashtag && r.Hashtag != "" {
			out = appendOnce(out, r.Hashtag)
		}
	}
	return out
}

// RunTimestamps returns the runs that jump to an offset in the video, in document
// order. These are the raw material for description-origin chapters.
func RunTimestamps(runs []TextRun) []TextRun {
	var out []TextRun
	for _, r := range runs {
		if r.Kind == RunTimestamp {
			out = append(out, r)
		}
	}
	return out
}
