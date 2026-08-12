package ytb

import (
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// channelparse.go builds the channel record out of a channel page. Doc 03 section 3.
//
// Four blocks answer and they do not overlap much, which is why all four are read
// rather than whichever one is easiest.
//
//	metadata.channelMetadataRenderer      the machine record: id, keywords, rss
//	                                      url, the 249 country codes, isFamilySafe
//	microformat.microformatDataRenderer   what the page tells crawlers, and the
//	                                      ProfilePage with every external link
//	header.pageHeaderViewModel            what a person sees: handle, the two
//	                                      rounded counts, the badge, the banner
//	the about panel                       a continuation, and the only source of
//	                                      the join date, lifetime views and country
//
// The two census maps below are the check that this stays true. A test walks a
// live page against them, so a key YouTube adds is a failing test rather than a
// field that quietly never appears in the output.

// channelMetadataFields is every key channelMetadataRenderer carries.
var channelMetadataFields = map[string]string{
	"externalId":            "ChannelID",
	"title":                 "Title",
	"description":           "Description",
	"keywords":              "Keywords",
	"avatar":                "Avatar",
	"channelUrl":            "CanonicalURL",
	"vanityChannelUrl":      "HandleURL",
	"ownerUrls":             "ignored: the same handle url this record already has as handle_url",
	"rssUrl":                "RSSURL",
	"isFamilySafe":          "IsFamilySafe",
	"facebookProfileId":     "FacebookProfileID",
	"availableCountryCodes": "AvailableCountryCodes",
	// Three deep links into the mobile apps, all of them the channel id with a
	// scheme in front. Nothing in them is a fact about the channel.
	"androidAppindexingLink": "ignored: the channel id in an android-app:// url",
	"androidDeepLink":        "ignored: the channel id in an android-app:// url",
	"iosAppindexingLink":     "ignored: the channel id in an ios-app:// url",
	// An ad conversion beacon. It carries a client version and a tracking blob.
	"channelConversionUrl": "ignored: an ad conversion pixel, no channel data",
}

// channelMicroformatFields is every key microformatDataRenderer carries. Most of it is
// the same channel url with a different scheme in front, which is what the page
// hands to Android, iOS, Twitter and Facebook in turn.
var channelMicroformatFields = map[string]string{
	"urlCanonical":                     "CanonicalURL",
	"title":                            "Title (already from channelMetadataRenderer)",
	"description":                      "ignored: the description with its newlines flattened to spaces, which channelMetadataRenderer gives unflattened",
	"thumbnail":                        "Avatar (a 200px rendition; channelMetadataRenderer gives 900px)",
	"tags":                             "Keywords (already split; channelMetadataRenderer gives the same list as one quoted string)",
	"familySafe":                       "IsFamilySafe (already from channelMetadataRenderer)",
	"availableCountries":               "AvailableCountryCodes (already from channelMetadataRenderer)",
	"channelProfileMicroformatDetails": "Links, Handle, SubscriberCount, via the ProfilePage",
	"noindex":                          "ignored: a crawler directive about the page, not the channel",
	"unlisted":                         "ignored: a crawler directive about the page, not the channel",
	"linkAlternates":                   "ignored: the m. and android-app forms of this same page",
	"schemaDotOrgType":                 "ignored: the string http://schema.org/http://schema.org/YoutubeChannelV2, doubled prefix and all",
	"siteName":                         "ignored: the constant YouTube",
	"appName":                          "ignored: the constant YouTube",
	"androidPackage":                   "ignored: the constant com.google.android.youtube",
	"iosAppStoreId":                    "ignored: the constant 544007664",
	"iosAppArguments":                  "ignored: the channel url again",
	"ogType":                           "ignored: the constant yt-fb-app:channel",
	"twitterCardType":                  "ignored: the constant summary",
	"twitterSiteHandle":                "ignored: the constant @YouTube",
	"urlApplinksAndroid":               "ignored: the channel url with a vnd.youtube scheme",
	"urlApplinksIos":                   "ignored: the channel url with a vnd.youtube scheme",
	"urlApplinksWeb":                   "ignored: the channel url with ?feature=applinks",
	"urlTwitterAndroid":                "ignored: the channel url with a vnd.youtube scheme",
	"urlTwitterIos":                    "ignored: the channel url with a vnd.youtube scheme",
}

// ParseChannelRecord builds a channel record from a channel page.
//
// pageURL is the address that was read, which is kept as url because it is the
// only field that says what this record is a read of. It is not the canonical
// url: a read of /@RickAstleyYT and a read of /channel/UCuAXFkgsw1L7xaCfnd5JJOw
// answer with the same channel and the site states the second as canonical.
func ParseChannelRecord(data *PageData, pageURL string) *Channel {
	root, _ := data.InitialData.(map[string]any)
	if root == nil {
		return nil
	}
	ch := &Channel{URL: pageURL, Envelope: newEnvelope("channel", SurfaceBrowseHTML)}
	ch.addSource(pageURL)
	ch.SubscriberCountIsApproximate = true

	cm := mapValue(mapValue(root, "metadata"), "channelMetadataRenderer")
	mf := mapValue(mapValue(root, "microformat"), "microformatDataRenderer")
	header := channelHeader(root)
	profile := channelProfilePage(mf, data.HTML)

	if cm != nil {
		ch.ChannelID = stringValue(cm["externalId"])
		ch.Title = stringValue(cm["title"])
		ch.Description = stringValue(cm["description"])
		ch.HandleURL = httpsScheme(stringValue(cm["vanityChannelUrl"]))
		ch.CanonicalURL = stringValue(cm["channelUrl"])
		ch.RSSURL = stringValue(cm["rssUrl"])
		ch.Keywords = splitKeywords(stringValue(cm["keywords"]))
		ch.Avatar = ParseThumbnails(mapValue(cm, "avatar")["thumbnails"])
		if v, ok := cm["isFamilySafe"].(bool); ok {
			ch.IsFamilySafe = boolPtr(v)
		}
		// YouTube hands the 249 codes back in a different order every read, which is
		// a set written out as a list. Sorting makes two reads of the same channel
		// compare equal, and there is no order here to lose.
		ch.AvailableCountryCodes = stringSlice(cm["availableCountryCodes"])
		sort.Strings(ch.AvailableCountryCodes)
		ch.FacebookProfileID = stringValue(cm["facebookProfileId"])
	}
	if mf != nil {
		// The microformat's own canonical url is the one the page publishes to
		// crawlers, and it agrees with channelUrl. It is read second so that a page
		// with no channelMetadataRenderer still gets one.
		ch.CanonicalURL = firstNonEmpty(ch.CanonicalURL, stringValue(mf["urlCanonical"]))
	}

	// The handle is stated three times and they agree, so the cheapest reliable one
	// wins: the ProfilePage says @RickAstleyYT outright, the header prints it as a
	// metadata row, and vanityChannelUrl has it as a path segment.
	ch.Handle = firstNonEmpty(
		stringValue(mapValue(profile, "mainEntity")["alternateName"]),
		handleFromURL(ch.HandleURL),
	)

	if profile != nil {
		ch.setVia("links", "s4 ld+json mainEntity.sameAs")
		me := mapValue(profile, "mainEntity")
		for _, u := range stringSlice(me["sameAs"]) {
			ch.Links = append(ch.Links, ChannelLink{URL: u})
		}
		if n := followerCount(me["interactionStatistic"]); n > 0 {
			ch.SubscriberCount = n
			ch.setVia("subscriber_count", "s4 ld+json FollowAction")
		}
	}

	if header != nil {
		ch.Banner = ParseThumbnails(mapValue(mapValue(mapValue(header, "banner"), "imageBannerViewModel"), "image")["sources"])
		// The header line is the handle, then the subscribers, then the videos, and
		// the two counts are told apart by that order rather than by the words in
		// them. Under --hl vi the same line reads "@BBCNews", "20 Tr người đăng ký",
		// "32 N video", so a match on "subscriber" finds nothing and the record comes
		// back with no counts at all. Doc 01 section 1.3.
		//
		// The English words are still checked first, because a channel with no videos
		// prints two parts rather than three and position alone would read its
		// subscriber count as a video count.
		counts := 0
		for _, part := range pageHeaderMetadataParts(header) {
			switch {
			case strings.HasPrefix(part, "@"):
				ch.Handle = firstNonEmpty(ch.Handle, part)
			case strings.Contains(part, "subscriber") || (counts == 0 && !strings.Contains(part, "video")):
				counts++
				ch.SubscriberCountText = part
				if ch.SubscriberCount == 0 {
					ch.SubscriberCount = parseCountText(part)
					ch.setVia("subscriber_count", "s4 header, rounded from "+part)
				}
			case strings.Contains(part, "video") || counts == 1:
				counts++
				ch.VideoCountText = part
				ch.VideoCount = parseCountText(part)
				ch.setVia("video_count", "s4 header, rounded from "+part)
			}
		}
		ch.IsVerified, ch.IsArtist = channelBadges(header)
	}

	ch.Tabs = ChannelTabs(root)
	if id := ch.ChannelID; strings.HasPrefix(id, "UC") {
		ch.UploadsPlaylistID = "UU" + id[2:]
	}
	if ch.ChannelID == "" && ch.Title == "" {
		return nil
	}
	return ch
}

// channelHeader returns the pageHeaderViewModel, which is where everything a
// person actually sees on a channel page lives.
func channelHeader(root map[string]any) map[string]any {
	if vm := mapValue(mapValue(mapValue(mapValue(root, "header"), "pageHeaderRenderer"), "content"), "pageHeaderViewModel"); vm != nil {
		return vm
	}
	// A channel YouTube still serves with the older header. Nothing else about the
	// read changes, so the walk is worth the one extra pass.
	var found map[string]any
	walkJSON(root, func(m map[string]any) {
		if found == nil {
			if vm, ok := m["pageHeaderViewModel"].(map[string]any); ok {
				found = vm
			}
		}
	})
	return found
}

// channelProfilePage returns the schema.org ProfilePage the page publishes about
// itself, which is where the external links come from without a continuation.
//
// It is published twice and the two are the same object with different key
// spellings: once inside ytInitialData under channelProfileMicroformatDetails,
// with plain type and context keys, and once in the HTML as an
// application/ld+json block with @type and @context. The parsed one is preferred
// because it costs no regex over 1 MB of HTML, and the HTML block is the fallback
// for a page whose microformat did not carry it.
func channelProfilePage(mf map[string]any, html string) map[string]any {
	if p := mapValue(mapValue(mf, "channelProfileMicroformatDetails"), "profilePage"); p != nil {
		return p
	}
	for _, m := range ldJSONRe.FindAllStringSubmatch(html, -1) {
		var obj map[string]any
		if err := json.Unmarshal([]byte(m[1]), &obj); err != nil {
			continue
		}
		// The @ prefixed keys are the JSON-LD spelling of the same fields, so they
		// are folded here and the rest of this file reads one shape.
		if t, ok := obj["@type"].(string); ok {
			obj["type"] = t
		}
		if stringValue(obj["type"]) == "ProfilePage" {
			return obj
		}
	}
	return nil
}

var ldJSONRe = regexp.MustCompile(`(?s)<script type="application/ld\+json"[^>]*>(.*?)</script>`)

// followerCount reads the subscriber count out of the ProfilePage's
// interactionStatistic, which is the only place on the page that states it as a
// number rather than as "4.52M subscribers". It is still rounded: the number is
// 4520000 exactly, three significant figures and five zeros.
func followerCount(v any) int64 {
	for _, item := range arrayValue(v) {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		it := mapValue(m, "interactionType")
		kind := firstNonEmpty(stringValue(it["type"]), stringValue(it["@type"]))
		if kind != "FollowAction" {
			continue
		}
		n, _ := strconv.ParseInt(stringValue(m["userInteractionCount"]), 10, 64)
		return n
	}
	return 0
}

// channelBadges reads the badge beside the channel title.
//
// The badge is an image with a name rather than a labelled flag, and the two
// names that matter are CHECK_CIRCLE_FILLED on a verified channel and AUDIO_BADGE
// on an official artist channel. Measured on @BBCNews and @RickAstleyYT, whose
// accessibility labels read "BBC News, Verified" and "Rick Astley, Official Artist
// Channel". The label is translated and the image name is not, so the name is what
// is matched and the label is only a cross check.
func channelBadges(header map[string]any) (verified, artist bool) {
	title := mapValue(mapValue(header, "title"), "dynamicTextViewModel")
	if title == nil {
		return false, false
	}
	for _, run := range arrayValue(mapValue(title, "text")["attachmentRuns"]) {
		img := mapValue(mapValue(mapValue(mapValue(mapValue(run, "")["element"], ""), "type"), "imageType"), "image")
		for _, src := range arrayValue(img["sources"]) {
			name := stringValue(mapValue(mapValue(src, ""), "clientResource")["imageName"])
			switch name {
			case "CHECK_CIRCLE_FILLED", "CHECK_CIRCLE_THICK", "VERIFIED":
				verified = true
			case "AUDIO_BADGE", "MUSIC_FILLED":
				artist = true
			}
		}
	}
	return verified, artist
}

// splitKeywords splits the keywords string into the list it stands for.
//
// The field is one string with quoted phrases in it: Official Rick Astley "rick
// astley" "never gonna give you up". Splitting on spaces would turn one phrase
// into three keywords, so the quotes are honoured.
func splitKeywords(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
			if !inQuote {
				flush()
			}
		case r == ' ' && !inQuote:
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

// The panel writes the join date two ways and which one it picks follows the
// content country, not the interface language. --gl US gives "Joined Feb 1,
// 2015" and --gl GB gives "Joined 1 Feb 2015", for the same channel on the same
// day, so a reader that knows one of them is a reader that works in one country.
//
// Only an English read is parsed at all. The month is a word, the panel is
// translated, and a month name guessed out of a language this does not cover
// would be an invented date. joined_text keeps the sentence whatever the
// language, so nothing is lost by not parsing it.
var (
	joinedDayFirstRe   = regexp.MustCompile(`(\d{1,2})\s+([A-Za-z]{3,})\.?,?\s+(\d{4})`)
	joinedMonthFirstRe = regexp.MustCompile(`([A-Za-z]{3,})\.?\s+(\d{1,2}),?\s+(\d{4})`)
)

// parseJoinedDate parses "Joined 1 Feb 2015" or "Joined Feb 1, 2015" into a date.
//
// It is a date and not a moment: the panel states no time and no zone, so the
// result is midnight UTC and anything more precise would be invented. A panel
// this cannot read returns the zero time, and the caller says so in missed.
func parseJoinedDate(s string) time.Time {
	day, month, year := "", "", ""
	if m := joinedDayFirstRe.FindStringSubmatch(s); m != nil {
		day, month, year = m[1], m[2], m[3]
	} else if m := joinedMonthFirstRe.FindStringSubmatch(s); m != nil {
		month, day, year = m[1], m[2], m[3]
	} else {
		return time.Time{}
	}
	// Sept is four letters and every other month abbreviates to three, so the
	// truncation is what makes en-GB's "Sept" parse at all.
	if len(month) > 3 {
		month = month[:3]
	}
	month = strings.ToUpper(month[:1]) + strings.ToLower(month[1:])
	t, err := time.Parse("2 Jan 2006", day+" "+month+" "+year)
	if err != nil {
		return time.Time{}
	}
	return t
}

// parseChannelRenderer reads a channelRenderer, which is the row a channel gets
// in a search result or a listing and not a read of the channel itself.
//
// It carries the title, a snippet of the description and the rounded subscriber
// count, and that is all it carries. The envelope says so: the surface is the
// search response and missed names what a real read would add, so a consumer that
// stored one of these knows it is holding a row rather than a record.
//
// The two count fields are read by what they say and not by what they are called.
// YouTube shifted them by one: on every channel row a live search returns,
// subscriberCountText holds the handle and videoCountText holds the subscriber
// count. A channel with no handle, such as a Topic channel, has no
// subscriberCountText at all and its videoCountText really is a video count. So
// each string is classified on its own text and the key names are ignored.
func parseChannelRenderer(r map[string]any) Channel {
	c := Channel{
		ChannelID:   stringValue(r["channelId"]),
		Title:       extractText(r["title"]),
		Description: extractText(r["descriptionSnippet"]),
		URL:         joinURL(endpointURL(r["navigationEndpoint"])),
		Avatar:      ParseThumbnails(mapValue(r, "thumbnail")["thumbnails"]),
		Envelope:    newEnvelope("channel", SurfaceInnerTube),
	}
	for _, key := range []string{"subscriberCountText", "videoCountText"} {
		text := extractText(r[key])
		switch {
		case text == "":
		case strings.HasPrefix(text, "@"):
			c.Handle = text
		case strings.Contains(text, "subscriber"):
			c.SubscriberCountText = text
			c.SubscriberCount = parseCountText(text)
			c.SubscriberCountIsApproximate = true
		case strings.Contains(text, "video"):
			c.VideoCountText = text
			c.VideoCount = parseCountText(text)
		}
	}
	if h := stringValue(mapValue(mapValue(mapValue(r, "navigationEndpoint"), "commandMetadata"), "webCommandMetadata")["url"]); strings.HasPrefix(h, "/@") {
		c.Handle = strings.TrimPrefix(h, "/")
	}
	c.miss("a search result row: no keywords, no links, no tab strip, no join date and no lifetime view count")
	return c
}
