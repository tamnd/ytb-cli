package youtube

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// about.go reads a channel's about panel.
//
// The panel is not on the channel page. What the page carries is a
// showEngagementPanelEndpoint holding a continuation token, and the panel arrives
// only when that token is browsed. So the about read is two calls, and the token
// for the second one has to be found in the first response rather than built.
//
// @RickAstleyYT's page holds two such tokens, one behind the description preview
// and one behind the subscriber line, and both were checked live: they differ only
// in an embedded panel id and both answer with the same aboutChannelViewModel. So
// the first one found is as good as the right one.
//
// aboutChannelViewModel carries exactly 18 fields and all 18 are read here. Two of
// them are easy to conflate and must not be: description is what the channel owner
// wrote, and artistBio is a third party biography YouTube attaches to music
// channels. Rick Astley's description is two lines about a single and a tour, and
// his artistBio is four paragraphs of encyclopedia prose. Folding one into the
// other would put words in the owner's mouth.

// ChannelAbout is a channel's about panel, field for field.
type ChannelAbout struct {
	ChannelID string `json:"channel_id" kit:"id" table:"id"`
	// Description is the channel owner's own text.
	Description string `json:"description" kit:"body" table:"-"`
	// ArtistBio is a third party biography, present on music channels only. It is
	// not the description and is never merged into it.
	ArtistBio       string `json:"artist_bio" table:"-"`
	Country         string `json:"country" table:"country"`
	SubscribersText string `json:"subscribers_text" table:"subscribers"`
	ViewsText       string `json:"views_text" table:"views"`
	VideosText      string `json:"videos_text" table:"videos"`
	JoinedDateText  string `json:"joined_date_text" table:"joined"`
	// CanonicalURL is the http form YouTube states; DisplayURL is the form it
	// prints. They differ in scheme, so both are kept rather than reconstructed.
	CanonicalURL string `json:"canonical_url" table:"-"`
	DisplayURL   string `json:"display_url" table:"-"`
	// BusinessEmailText is a prompt, not an address. Signed out it reads "Sign in
	// to see email address", and it is kept as text so a caller can see that the
	// address exists without this tool pretending to have read it.
	BusinessEmailText string        `json:"business_email_text" table:"-"`
	Links             []ChannelLink `json:"links" table:"-"`
	// Labels are the panel's own captions, translated. They are kept because the
	// goal is a 1:1 mapping of the rendered panel, and because they are the only
	// evidence of which sections YouTube chose to show.
	Labels ChannelAboutLabels `json:"labels" table:"-"`
}

// ChannelAboutLabels are the four section captions on the panel.
type ChannelAboutLabels struct {
	Description    string `json:"description"`
	ArtistBio      string `json:"artist_bio"`
	Links          string `json:"links"`
	AdditionalInfo string `json:"additional_info"`
}

// ChannelLink is one entry in the about panel's links section.
type ChannelLink struct {
	Title string `json:"title"`
	// URL is the real destination. YouTube wraps every outbound link in
	// youtube.com/redirect?...&q=<encoded>, so the wrapper is unwrapped here and a
	// caller never has to know it was there.
	URL string `json:"url"`
	// Display is the shortened form shown on the page, e.g.
	// rickastley.lnk.to/Raindrops for a longer https url.
	Display string `json:"display"`
}

// aboutViewModelFields is every key aboutChannelViewModel carries, with what
// happens to it. A test walks the real response against this map, so a field
// YouTube adds shows up as a failing test rather than as data quietly dropped.
var aboutViewModelFields = map[string]string{
	"channelId":                  "ChannelID",
	"description":                "Description",
	"artistBio":                  "ArtistBio",
	"country":                    "Country",
	"subscriberCountText":        "SubscribersText",
	"viewCountText":              "ViewsText",
	"videoCountText":             "VideosText",
	"joinedDateText":             "JoinedDateText",
	"canonicalChannelUrl":        "CanonicalURL",
	"displayCanonicalChannelUrl": "DisplayURL",
	// Named like a url and is not one. Tapping the channel url opens a share sheet,
	// so this field holds a shareEntityEndpoint whose serializedShareEntity decodes
	// to 1a 18 UCuAXFkgsw1L7xaCfnd5JJOw, the channel id and nothing else. Reading it
	// as a url gave an empty string, which is the field being honest.
	"customUrlOnTap":         "ignored: a share sheet command carrying only the channel id, which is already a field",
	"signInForBusinessEmail": "BusinessEmailText",
	"links":                  "Links",
	"descriptionLabel":       "Labels.Description",
	"artistBioLabel":         "Labels.ArtistBio",
	"customLinksLabel":       "Labels.Links",
	"additionalInfoLabel":    "Labels.AdditionalInfo",
	"rendererContext":        "ignored: click tracking and logging directives, no channel data",
}

// FetchChannelAbout reads a channel's about panel. idOrURL is anything
// ResolveChannelID takes.
func (c *Client) FetchChannelAbout(ctx context.Context, idOrURL string) (*ChannelAbout, error) {
	channelID, err := c.ResolveChannelID(ctx, idOrURL)
	if err != nil {
		return nil, err
	}
	it := NewInnerTube(c)
	page, err := it.Browse(ctx, channelID, "", "")
	if err != nil {
		return nil, err
	}
	token := FindAboutToken(page)
	if token == "" {
		return nil, fmt.Errorf("channel %s: no about panel on this channel", channelID)
	}
	resp, err := it.BrowseContinuation(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("channel %s about panel: %w", channelID, err)
	}
	about := ParseChannelAbout(resp)
	if about == nil {
		return nil, fmt.Errorf("channel %s: about panel carried no aboutChannelViewModel", channelID)
	}
	if about.ChannelID == "" {
		about.ChannelID = channelID
	}
	return about, nil
}

// FindAboutToken returns the about panel's continuation token from a channel
// response. It asks for the token under showEngagementPanelEndpoint, which
// FindContinuationToken deliberately skips because for paging purposes an
// engagement panel is a different list.
func FindAboutToken(resp map[string]any) string {
	return FindContinuationTokenUnder(resp, "showEngagementPanelEndpoint")
}

// ParseChannelAbout reads aboutChannelViewModel out of a continuation response.
func ParseChannelAbout(resp map[string]any) *ChannelAbout {
	var vm map[string]any
	walkJSON(resp, func(m map[string]any) {
		if vm != nil {
			return
		}
		if v, ok := m["aboutChannelViewModel"].(map[string]any); ok {
			vm = v
		}
	})
	if vm == nil {
		return nil
	}
	about := &ChannelAbout{
		ChannelID:       stringValue(vm["channelId"]),
		Description:     stringValue(vm["description"]),
		ArtistBio:       viewModelText(vm["artistBio"]),
		Country:         stringValue(vm["country"]),
		SubscribersText: stringValue(vm["subscriberCountText"]),
		ViewsText:       stringValue(vm["viewCountText"]),
		VideosText:      stringValue(vm["videoCountText"]),
		JoinedDateText:  viewModelText(vm["joinedDateText"]),
		CanonicalURL:    stringValue(vm["canonicalChannelUrl"]),
		DisplayURL:      stringValue(vm["displayCanonicalChannelUrl"]),
		// Signed out this is the prompt rather than the address, which is the
		// truthful thing to carry.
		BusinessEmailText: viewModelText(vm["signInForBusinessEmail"]),
		Links:             parseChannelLinks(vm["links"]),
		Labels: ChannelAboutLabels{
			Description:    viewModelText(vm["descriptionLabel"]),
			ArtistBio:      viewModelText(vm["artistBioLabel"]),
			Links:          viewModelText(vm["customLinksLabel"]),
			AdditionalInfo: viewModelText(vm["additionalInfoLabel"]),
		},
	}
	return about
}

// parseChannelLinks reads the links section, unwrapping each redirect.
func parseChannelLinks(v any) []ChannelLink {
	var out []ChannelLink
	for _, item := range arrayValue(v) {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		lm := mapValue(m, "channelExternalLinkViewModel")
		if lm == nil {
			continue
		}
		link := ChannelLink{
			Title:   viewModelText(lm["title"]),
			Display: viewModelText(lm["link"]),
		}
		link.URL = unwrapRedirect(commandRunURL(lm["link"]))
		if link.URL == "" && link.Display != "" {
			// Some links carry no command at all, only the shortened text. A url
			// built from that text would be a guess, so the scheme is added and
			// nothing else.
			link.URL = "https://" + link.Display
		}
		out = append(out, link)
	}
	return out
}

// viewModelText reads the content of a view model text node. The newer renderers
// dropped runs for a flat content string, so this handles both rather than
// assuming which generation a field belongs to.
func viewModelText(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case map[string]any:
		if s := stringValue(t["content"]); s != "" {
			return s
		}
		return extractText(t)
	}
	return ""
}

// commandURL reads the web url out of an innertube command wrapper.
func commandURL(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	cmd := mapValue(m, "innertubeCommand")
	if cmd == nil {
		cmd = m
	}
	if u := stringValue(mapValue(cmd, "urlEndpoint")["url"]); u != "" {
		return u
	}
	if u := stringValue(mapValue(mapValue(cmd, "commandMetadata"), "webCommandMetadata")["url"]); u != "" {
		if strings.HasPrefix(u, "/") {
			return BaseURL + u
		}
		return u
	}
	return ""
}

// commandRunURL reads the url from the first command run on a text node, which is
// where a link's destination hides.
func commandRunURL(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	for _, run := range arrayValue(m["commandRuns"]) {
		rm, ok := run.(map[string]any)
		if !ok {
			continue
		}
		if u := commandURL(rm["onTap"]); u != "" {
			return u
		}
	}
	return ""
}

// unwrapRedirect returns the destination behind a youtube.com/redirect wrapper.
// Every outbound link on the about panel is wrapped, and the wrapper carries a
// redir_token that expires, so storing the wrapper would store a link that stops
// working.
func unwrapRedirect(raw string) string {
	if raw == "" || !strings.Contains(raw, "/redirect?") {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if q := u.Query().Get("q"); q != "" {
		return q
	}
	return raw
}
