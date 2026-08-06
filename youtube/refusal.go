package youtube

import (
	"errors"
	"fmt"
	"strings"
)

// refusal.go separates the two things a failed read can mean.
//
// A transient failure is worth retrying: a 429, a 503, a dropped connection.
// A refusal is not. When YouTube says "Restricted Mode has hidden comments for
// this video." or "This playlist type is unviewable." it has answered. Asking
// again three times with backoff gets the same sentence three times slower, and
// it makes the tool look broken when it is in fact working correctly and
// reporting a limit.
//
// The distinction matters more here than it usually does, because YouTube
// refuses with HTTP 200. An alertRenderer of type ERROR inside a successful
// browse response is a refusal, and a status code tells you nothing about it.

// Refusal is a read that YouTube answered by declining. It carries YouTube's own
// sentence, because paraphrasing it loses the only information the user can act
// on.
type Refusal struct {
	// What we were reading, e.g. "comments for kJQP7kiw5Fk".
	Subject string
	// YouTube's sentence, verbatim, in whatever language it arrived in.
	Message string
	// Surface names where the refusal was read, e.g. "next" or "browse".
	Surface string
	// Remedy is what would clear it, when there is something. Empty when there
	// is not, rather than a guess.
	Remedy string
}

func (r *Refusal) Error() string {
	var b strings.Builder
	b.WriteString(r.Subject)
	b.WriteString(": ")
	b.WriteString(r.Message)
	if r.Surface != "" {
		fmt.Fprintf(&b, " (%s)", r.Surface)
	}
	if r.Remedy != "" {
		b.WriteString("\n")
		b.WriteString(r.Remedy)
	}
	return b.String()
}

// IsRefusal reports whether err is a refusal, so a caller can exit 4 rather than
// exit 1 and a retry loop can stop.
func IsRefusal(err error) bool {
	var r *Refusal
	return errors.As(err, &r)
}

// AsRefusal returns the Refusal in err, if there is one.
func AsRefusal(err error) (*Refusal, bool) {
	var r *Refusal
	if errors.As(err, &r) {
		return r, true
	}
	return nil, false
}

// refusalPhrases are the sentences measured while writing spec 3005, each with
// what would clear it. The match is on the English wording, and a page fetched
// in another language will not match, which is why the alertRenderer type and
// the playabilityStatus status are checked first and these only refine the
// remedy. See doc 00 sections 0.4 and 0.5.
var refusalPhrases = []struct {
	contains string
	remedy   string
}{
	{
		contains: "Restricted Mode has hidden comments",
		remedy: "Restricted Mode is set on the network or the account, not the video.\n" +
			"Six client-side bypasses were tried while writing the spec and none worked.\n" +
			"A different network, or `ytb auth import --cookies` with an account that has it off, is what clears it.",
	},
	{
		contains: "Posts aren't currently available on this device",
		remedy:   "The community tab refuses this client. No client swap tried so far changes it.",
	},
	{
		contains: "This playlist type is unviewable",
		remedy:   "Mixes and auto-generated radio playlists have no listable contents. There is no id that reads them.",
	},
	{
		contains: "Sign in to confirm your age",
		remedy:   "Age-gated. `ytb auth import --cookies` with a signed-in adult account is the only path.",
	},
	{
		contains: "This video is private",
		remedy:   "",
	},
	{
		contains: "Video unavailable",
		remedy:   "",
	},
}

// newRefusal builds a Refusal, attaching a remedy when the message is one we
// have measured.
func newRefusal(subject, surface, message string) *Refusal {
	r := &Refusal{Subject: subject, Surface: surface, Message: strings.TrimSpace(message)}
	for _, p := range refusalPhrases {
		if strings.Contains(r.Message, p.contains) {
			r.Remedy = p.remedy
			break
		}
	}
	return r
}

// alertRefusal reads the `alerts` array of a browse or next response and returns
// a Refusal when one of them is an ERROR.
//
// This is checked on every browse response, whatever the status code, because
// alerts is where YouTube volunteers what it left out. Not every alert is a
// refusal: "Unavailable videos are hidden" is a WARNING and belongs in the
// record's `missed` list, not in an error.
func alertRefusal(resp map[string]any, subject, surface string) *Refusal {
	for _, a := range alertRenderers(resp) {
		if strings.EqualFold(stringValue(a["type"]), "ERROR") {
			if msg := extractText(a["text"]); msg != "" {
				return newRefusal(subject, surface, msg)
			}
		}
	}
	return nil
}

// alertWarnings returns the non-error alerts as plain sentences, for the
// record's `missed` list. "Unavailable videos are hidden" is the one that turns
// up most, and quoting it is how a count that does not add up explains itself.
func alertWarnings(resp map[string]any) []string {
	var out []string
	for _, a := range alertRenderers(resp) {
		if strings.EqualFold(stringValue(a["type"]), "ERROR") {
			continue
		}
		if msg := extractText(a["text"]); msg != "" {
			out = append(out, msg)
		}
	}
	return out
}

// messageRefusal finds a refusal that arrived as a messageRenderer where the
// content should have been.
//
// A refusal does not always come as an alert. The posts tab is the case that
// showed it: browsing @Computerphile's posts tab with the params off its own strip
// selects the tab, returns 200 and 39 KB, carries no alerts at all, and puts
// "Posts aren't currently available on this device" in a messageRenderer in place
// of the posts. Read as content that is an empty list, and an empty list is a
// claim that the channel has no posts, which is not what YouTube said.
//
// Only the two phrases measured in this renderer count, and not the whole
// refusalPhrases list. A messageRenderer is how YouTube says ordinary things too,
// including "No more results" and the placeholder title of a deleted entry, so
// matching "Video unavailable" here would turn the end of a list or a gap in a
// playlist into a failed read.
//
// The subtext is quoted alongside the text when there is one. On the posts tab it
// reads "You can access Posts tab content from your desktop or other supported
// devices.", which is the half of YouTube's answer that says what to do about it.
var messageRefusalPhrases = []string{
	// Measured on @Computerphile's posts tab, selected with the params off its own
	// strip: 200, no alerts, this in place of the 30 posts the channel has.
	"Posts aren't currently available on this device",
	// Measured in the comment section of a watch page from this address. The full
	// sentence is "Restricted Mode has hidden comments for this video." and
	// comments.go quotes it whole rather than matching on it.
	"Restricted Mode has hidden comments",
}

func messageRefusal(resp map[string]any, subject, surface string) *Refusal {
	var found string
	walkJSON(resp, func(m map[string]any) {
		if found != "" {
			return
		}
		mr, ok := m["messageRenderer"].(map[string]any)
		if !ok {
			return
		}
		text := extractText(mr["text"])
		matched := false
		for _, p := range messageRefusalPhrases {
			if strings.Contains(text, p) {
				matched = true
				break
			}
		}
		if !matched {
			return
		}
		if sub := extractText(mapValue(mapValue(mr, "subtext"), "messageSubtextRenderer")["text"]); sub != "" {
			text += " " + sub
		}
		found = text
	})
	if found == "" {
		return nil
	}
	return newRefusal(subject, surface, found)
}

// alertRenderers pulls every alert renderer out of a response. YouTube spells
// this two ways, a bare `alertRenderer` and one wrapped in
// `alertWithButtonRenderer`, so both are collected.
func alertRenderers(resp map[string]any) []map[string]any {
	var out []map[string]any
	walkJSON(resp, func(m map[string]any) {
		for _, key := range []string{"alertRenderer", "alertWithButtonRenderer"} {
			if a, ok := m[key].(map[string]any); ok {
				out = append(out, a)
			}
		}
	})
	return out
}
