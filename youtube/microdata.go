package youtube

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// microdata.go reads the schema.org markup a YouTube page publishes about itself.
//
// Under the three JavaScript blobs on a watch page there is a fourth thing nobody
// looks at: a complete schema.org/VideoObject in plain HTML, itemid set to the
// watch URL, twenty itemprop values, the author as a Person, a one-element
// BreadcrumbList naming the channel, the thumbnail as an ImageObject with its
// dimensions, and both counts exact. Doc 01 section 1.2.
//
// It is worth reading for three separate reasons.
//
// It is a free second opinion. microformat.lengthSeconds says 214 and
// videoDetails.lengthSeconds says 213 for dQw4w9WgXcQ, two objects in one JSON
// document disagreeing by a second on the most fetched video on the site, and the
// microdata's PT3M34S breaks the tie. regionsAllowed is microformat's
// availableCountries exactly, 249 codes in the same order, which makes it a free
// check on the country parser.
//
// It is the vocabulary. Doc 04 maps this tool's records to RDF, and this is
// YouTube naming the schema.org classes of its own things: VideoObject, Person,
// ProfilePage, InteractionCounter with LikeAction and WatchAction, BreadcrumbList.
// None of that mapping needs inventing.
//
// And it must not be trusted over the payload. The markup is generated for
// crawlers and it rounds where the payload does not, so exact counts stay with
// videoDetails and microformat and this is kept beside them for comparison. That
// is what --microdata prints.
//
// Two details cost an afternoon if they are not written down. The author url is
// plain http on a page served over TLS, so the scheme is normalised. And booleans
// are spelled two ways in the same block, isFamilyFriendly="true" next to
// requiresSubscription="False", so they are parsed case insensitively.

// Microdata is the VideoObject a watch page publishes about itself.
type Microdata struct {
	ItemID   string `json:"item_id,omitempty"`
	ItemType string `json:"item_type,omitempty"`

	Identifier  string `json:"identifier,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url,omitempty"`
	EmbedURL    string `json:"embed_url,omitempty"`
	// Duration is the ISO 8601 string verbatim, e.g. PT3M34S, and DurationSeconds
	// is it parsed. Both are kept because the string is the evidence.
	Duration        string   `json:"duration,omitempty"`
	DurationSeconds int      `json:"duration_seconds,omitempty"`
	Genre           string   `json:"genre,omitempty"`
	Keywords        []string `json:"keywords,omitempty"`

	DatePublished time.Time `json:"date_published,omitzero"`
	UploadDate    time.Time `json:"upload_date,omitzero"`

	// Nil means the page did not say, which is not the same as false.
	IsFamilyFriendly     *bool    `json:"is_family_friendly,omitempty"`
	RequiresSubscription *bool    `json:"requires_subscription,omitempty"`
	RegionsAllowed       []string `json:"regions_allowed,omitempty"`

	// PlayerType, Width and Height describe the embedded player, not the video.
	PlayerType string `json:"player_type,omitempty"`
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`

	ThumbnailURL string     `json:"thumbnail_url,omitempty"`
	Thumbnail    *Thumbnail `json:"thumbnail,omitempty"`

	AuthorName string `json:"author_name,omitempty"`
	// AuthorURL is normalised to https. The page states it as http.
	AuthorURL string `json:"author_url,omitempty"`
	// Breadcrumb is the BreadcrumbList inside the author, one entry naming the
	// channel. It is kept because it is the page's own statement of what contains
	// what, which is an edge in doc 04's graph.
	Breadcrumb []string `json:"breadcrumb,omitempty"`

	// LikeCount and ViewCount are the two InteractionCounter blocks, keyed by their
	// interactionType. Both are exact here, and both are compared against the
	// payload rather than replacing it.
	LikeCount int64 `json:"like_count,omitempty"`
	ViewCount int64 `json:"view_count,omitempty"`
}

// microdataVideoFields is every itemprop the watch page's VideoObject carries,
// with what happens to it. A test walks the real page against this map, so a
// twenty-first itemprop shows up as a failing test rather than as data silently
// dropped.
var microdataVideoFields = map[string]string{
	"identifier":            "Identifier",
	"name":                  "Name",
	"description":           "Description",
	"url":                   "URL",
	"embedUrl":              "EmbedURL",
	"duration":              "Duration, DurationSeconds",
	"genre":                 "Genre",
	"keywords":              "Keywords",
	"datePublished":         "DatePublished",
	"uploadDate":            "UploadDate",
	"isFamilyFriendly":      "IsFamilyFriendly",
	"requiresSubscription":  "RequiresSubscription",
	"regionsAllowed":        "RegionsAllowed",
	"playerType":            "PlayerType",
	"width":                 "Width",
	"height":                "Height",
	"thumbnailUrl":          "ThumbnailURL",
	"thumbnail":             "Thumbnail, an ImageObject",
	"author":                "AuthorName, AuthorURL, Breadcrumb, a Person",
	"interactionStatistic":  "LikeCount or ViewCount by interactionType",
	"interactionType":       "read inside interactionStatistic",
	"userInteractionCount":  "read inside interactionStatistic",
	"itemListElement":       "read inside the author's BreadcrumbList",
	"item":                  "read inside the author's BreadcrumbList",
	"position":              "ignored: the list has one entry, so its position says nothing",
	"unlisted":              "ignored: microformat.isUnlisted is the payload's own answer",
	"paid":                  "ignored: hasYpcMetadata is the payload's own answer",
	"channelId":             "ignored: videoDetails.channelId is the payload's own answer",
	"videoId":               "ignored: videoDetails.videoId is the payload's own answer",
	"isFamilyFriendlyVideo": "ignored: a duplicate spelling of isFamilyFriendly",
	"interactionCount":      "ignored: the older spelling, superseded by userInteractionCount",
}

// ParseVideoMicrodata reads the VideoObject out of a watch page's HTML. It
// returns nil when the page carries no such block, which is what an embed page and
// a consent interstitial do.
func ParseVideoMicrodata(html string) *Microdata {
	if html == "" {
		return nil
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil
	}
	root := doc.Find(`[itemscope][itemtype$="schema.org/VideoObject"]`).First()
	if root.Length() == 0 {
		return nil
	}
	return microdataFromItem(scrapeMicroItem(root))
}

// microdataFromItem maps a scraped item onto the typed record.
func microdataFromItem(item *microItem) *Microdata {
	if item == nil {
		return nil
	}
	md := &Microdata{
		ItemID:               item.itemID,
		ItemType:             item.itemType,
		Identifier:           item.text("identifier"),
		Name:                 item.text("name"),
		Description:          item.text("description"),
		URL:                  item.text("url"),
		EmbedURL:             item.text("embedUrl"),
		Duration:             item.text("duration"),
		Genre:                item.text("genre"),
		Keywords:             splitList(item.text("keywords")),
		DatePublished:        parseDate(item.text("datePublished")),
		UploadDate:           parseDate(item.text("uploadDate")),
		IsFamilyFriendly:     parseLooseBool(item.text("isFamilyFriendly")),
		RequiresSubscription: parseLooseBool(item.text("requiresSubscription")),
		RegionsAllowed:       splitList(item.text("regionsAllowed")),
		PlayerType:           item.text("playerType"),
		Width:                atoi(item.text("width")),
		Height:               atoi(item.text("height")),
		ThumbnailURL:         item.text("thumbnailUrl"),
	}
	md.DurationSeconds = ParseISODuration(md.Duration)
	if thumb := item.item("thumbnail"); thumb != nil {
		md.Thumbnail = &Thumbnail{
			URL:    thumb.text("url"),
			Width:  atoi(thumb.text("width")),
			Height: atoi(thumb.text("height")),
			Source: ThumbnailFromMicrodata,
		}
	}
	if author := item.item("author"); author != nil {
		md.AuthorName = author.text("name")
		md.AuthorURL = httpsScheme(author.text("url"))
		md.Breadcrumb = breadcrumbNames(author)
	}
	for _, counter := range item.items("interactionStatistic") {
		count := int64Value(counter.text("userInteractionCount"))
		switch {
		case strings.HasSuffix(counter.text("interactionType"), "LikeAction"):
			md.LikeCount = count
		case strings.HasSuffix(counter.text("interactionType"), "WatchAction"):
			md.ViewCount = count
		}
	}
	return md
}

// breadcrumbNames reads the names out of a BreadcrumbList nested in an item. The
// list hangs off the author as an itemscope with no itemprop of its own, so it
// belongs to nobody by name and is found by its type.
func breadcrumbNames(item *microItem) []string {
	var out []string
	for _, anon := range item.anon {
		if !strings.HasSuffix(anon.itemType, "BreadcrumbList") {
			continue
		}
		for _, entry := range anon.items("itemListElement") {
			if thing := entry.item("item"); thing != nil {
				if name := thing.text("name"); name != "" {
					out = append(out, name)
				}
			}
		}
	}
	return out
}

// --- the generic scraper ---

// microItem is one itemscope: its type, its id, the properties on it, and the
// itemscopes nested under it that carry no itemprop of their own.
type microItem struct {
	itemType string
	itemID   string
	// props keeps every value for a property, in document order, because
	// interactionStatistic appears twice and the second is not a correction of the
	// first.
	props map[string][]microValue
	// order is the property names in the order they appeared, so the census test
	// reports them the way the page does.
	order []string
	anon  []*microItem
}

// microValue is either a string or a nested item, never both.
type microValue struct {
	text string
	item *microItem
}

func (m *microItem) add(prop string, v microValue) {
	if m.props == nil {
		m.props = map[string][]microValue{}
	}
	if _, seen := m.props[prop]; !seen {
		m.order = append(m.order, prop)
	}
	m.props[prop] = append(m.props[prop], v)
}

// text returns the first string value of a property.
func (m *microItem) text(prop string) string {
	if m == nil {
		return ""
	}
	for _, v := range m.props[prop] {
		if v.item == nil {
			return v.text
		}
	}
	return ""
}

// item returns the first nested item under a property.
func (m *microItem) item(prop string) *microItem {
	items := m.items(prop)
	if len(items) == 0 {
		return nil
	}
	return items[0]
}

// items returns every nested item under a property.
func (m *microItem) items(prop string) []*microItem {
	if m == nil {
		return nil
	}
	var out []*microItem
	for _, v := range m.props[prop] {
		if v.item != nil {
			out = append(out, v.item)
		}
	}
	return out
}

// scrapeMicroItem reads one itemscope and everything that belongs to it.
//
// Scope is the whole difficulty. url and name appear three times on a watch page,
// once on the VideoObject and once each inside the author Person and the thumbnail
// ImageObject, so a flat sweep for itemprop="url" picks whichever comes first and
// gets the thumbnail's url as the video's. A property belongs to the nearest
// enclosing itemscope, so the walk stops descending when it meets one.
func scrapeMicroItem(sel *goquery.Selection) *microItem {
	item := &microItem{
		itemType: attrOf(sel, "itemtype"),
		itemID:   attrOf(sel, "itemid"),
	}
	sel.Children().Each(func(_ int, child *goquery.Selection) {
		collectMicroProps(child, item)
	})
	return item
}

// collectMicroProps walks one node, assigning what it finds to into.
func collectMicroProps(node *goquery.Selection, into *microItem) {
	prop := attrOf(node, "itemprop")
	_, isScope := node.Attr("itemscope")
	switch {
	case prop != "" && isScope:
		// A named nested item. It owns its own properties, so the walk stops here.
		into.add(prop, microValue{item: scrapeMicroItem(node)})
		return
	case prop == "" && isScope:
		// An itemscope with no name. The BreadcrumbList is one: it hangs inside the
		// author with no itemprop, so it is kept as an anonymous child rather than
		// having its list items stolen by the author.
		into.anon = append(into.anon, scrapeMicroItem(node))
		return
	case prop != "":
		into.add(prop, microValue{text: microPropText(node)})
	}
	node.Children().Each(func(_ int, child *goquery.Selection) {
		collectMicroProps(child, into)
	})
}

// microPropText reads a property's value from wherever its element keeps it.
func microPropText(node *goquery.Selection) string {
	if v, ok := node.Attr("content"); ok {
		return v
	}
	if v, ok := node.Attr("href"); ok {
		return v
	}
	if v, ok := node.Attr("src"); ok {
		return v
	}
	if v, ok := node.Attr("datetime"); ok {
		return v
	}
	return strings.TrimSpace(node.Text())
}

func attrOf(sel *goquery.Selection, name string) string {
	v, _ := sel.Attr(name)
	return v
}

// --- value parsing ---

// reISODuration matches the PT#H#M#S subset schema.org durations use here. Days
// and above never appear on a video, and a pattern that accepted them would also
// accept P3Y and return a number nobody can act on.
var reISODuration = regexp.MustCompile(`^PT(?:(\d+)H)?(?:(\d+)M)?(?:(\d+(?:\.\d+)?)S)?$`)

// ParseISODuration turns PT3M34S into 214. It returns 0 for anything it does not
// recognise, so a caller compares against the payload rather than against a guess.
func ParseISODuration(s string) int {
	m := reISODuration.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0
	}
	total := atoi(m[1])*3600 + atoi(m[2])*60
	if m[3] != "" {
		secs, err := strconv.ParseFloat(m[3], 64)
		if err == nil {
			total += int(secs)
		}
	}
	return total
}

// parseLooseBool reads a boolean the page spelled either way. It returns nil when
// there was nothing to read, because absent and false are different answers to
// "is this family friendly".
func parseLooseBool(s string) *bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes":
		v := true
		return &v
	case "false", "0", "no":
		v := false
		return &v
	}
	return nil
}

// splitList reads a comma separated attribute, trimming each entry. regionsAllowed
// is 249 entries and keywords is 27 on one video, so an empty entry from a trailing
// comma is dropped rather than stored as "".
func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// httpsScheme upgrades the http url the page states for the author. The page is
// served over TLS and the link works over TLS; the http in the markup is a leftover
// and storing it would store a redirect.
func httpsScheme(raw string) string {
	if strings.HasPrefix(raw, "http://") {
		return "https://" + strings.TrimPrefix(raw, "http://")
	}
	return raw
}

func atoi(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}
