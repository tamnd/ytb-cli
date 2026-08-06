package youtube

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strings"

	"github.com/tamnd/ytb-cli/pkg/ytid"
)

// music.go is surface s10, music.youtube.com's InnerTube as WEB_REMIX. Doc 01
// section 10, doc 03 section 12.
//
// The rule that shapes this file: never decide what a thing is from a rendered
// word. music.youtube.com types every endpoint it serves. A browse endpoint
// carries a pageType, so an album row says MUSIC_PAGE_TYPE_ALBUM whatever the
// shelf above it is called, and a watch endpoint carries a musicVideoType, so an
// art track says ATV where the official video of the same song says OMV. A
// parser that reads "Albums" out of a shelf heading returns nothing the day
// somebody passes --hl ja, and returns the wrong thing the day YouTube renames a
// shelf.
//
// The rendered words are still kept, because they are what the page said. They
// are just never load-bearing.

const musicBaseURL = "https://music.youtube.com"

// The pageType values a browse endpoint carries. These are the discriminators.
const (
	musicPageArtist      = "MUSIC_PAGE_TYPE_ARTIST"
	musicPageAlbum       = "MUSIC_PAGE_TYPE_ALBUM"
	musicPagePlaylist    = "MUSIC_PAGE_TYPE_PLAYLIST"
	musicPageUserChannel = "MUSIC_PAGE_TYPE_USER_CHANNEL"
	musicPageLyrics      = "MUSIC_PAGE_TYPE_TRACK_LYRICS"
	// A podcast show is a playlist with a show page in front of it: the browse id
	// is MPSP and then the PL id its episodes are in. An episode is a video id and
	// arrives as a watch endpoint like any other, so it needs no page type here.
	musicPagePodcast = "MUSIC_PAGE_TYPE_PODCAST_SHOW_DETAIL_PAGE"
)

// The MusicItem.Kind values, which are the record kinds a lockup can name.
const (
	musicKindTrack    = "track"
	musicKindAlbum    = "album"
	musicKindArtist   = "artist"
	musicKindPlaylist = "playlist"
)

// musicSearchParams returns the WEB_REMIX filter param for a type string. An
// unknown or empty type returns "", which searches everything.
func musicSearchParams(typ string) string {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "song":
		return "EgWKAQIIAWoMEA4QChADEAQQCRAF"
	case "video":
		return "EgWKAQIQAWoMEA4QChADEAQQCRAF"
	case "album":
		return "EgWKAQIYAWoMEA4QChADEAQQCRAF"
	case "artist":
		return "EgWKAQIgAWoMEA4QChADEAQQCRAF"
	case "playlist":
		return "EgWKAQIoAWoMEA4QChADEAQQCRAF"
	case "podcast":
		return "EgWKAQJQAWoMEA4QChADEAQQCRAF"
	case "episode":
		return "EgWKAQJIAWoMEA4QChADEAQQCRAF"
	default:
		return ""
	}
}

// musicBrowseID normalises a music.youtube.com URL or a bare id to the id to
// browse. An id it does not recognise is passed through, because a new prefix is
// more likely than a typo and the server gives a better error than this can.
func musicBrowseID(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	if !strings.Contains(input, "/") {
		return input
	}
	u, err := url.Parse(input)
	if err != nil {
		return input
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	switch {
	case len(parts) >= 2 && (parts[0] == "browse" || parts[0] == "channel"):
		return parts[1]
	case len(parts) >= 1 && parts[0] == "playlist":
		if id := u.Query().Get("list"); id != "" {
			return id
		}
	case len(parts) >= 1 && parts[0] == "watch":
		if v := u.Query().Get("v"); v != "" {
			return v
		}
	}
	return input
}

// --- the typed layer ---

// musicTarget is what an endpoint points at, with the payload's own type on it.
// An empty target means the endpoint said nothing, which is different from a
// target this file does not have a case for.
type musicTarget struct {
	BrowseID       string
	PageType       string
	VideoID        string
	PlaylistID     string
	MusicVideoType string
}

func (t musicTarget) empty() bool { return t.BrowseID == "" && t.VideoID == "" && t.PlaylistID == "" }

// kind maps a target to the record kind it names, by type and never by prefix
// where a type is available. The id prefixes are the fallback for the handful of
// endpoints that carry no config, and VL is routing rather than identity so it
// comes off here.
func (t musicTarget) kind() string {
	switch t.PageType {
	case musicPageAlbum:
		return musicKindAlbum
	case musicPageArtist:
		return musicKindArtist
	case musicPagePlaylist, musicPagePodcast:
		return musicKindPlaylist
	case musicPageUserChannel:
		return musicKindArtist
	}
	if t.VideoID != "" {
		return musicKindTrack
	}
	switch {
	case strings.HasPrefix(t.BrowseID, "MPRE"):
		return musicKindAlbum
	case strings.HasPrefix(t.BrowseID, "VL"), t.BrowseID == "" && t.PlaylistID != "":
		return musicKindPlaylist
	case strings.HasPrefix(t.BrowseID, "UC"), strings.HasPrefix(t.BrowseID, "MPLA"):
		return musicKindArtist
	}
	return ""
}

// id is the target's own id, with the browse prefix stripped off a playlist.
func (t musicTarget) id() string {
	switch t.kind() {
	case musicKindTrack:
		return t.VideoID
	case musicKindPlaylist:
		if t.BrowseID != "" {
			return ytid.StripWire(t.BrowseID)
		}
		return t.PlaylistID
	default:
		return t.BrowseID
	}
}

// musicEndpointTarget reads a navigationEndpoint, an onTap or a menu entry.
func musicEndpointTarget(v any) musicTarget {
	m := mapValue(v, "")
	if m == nil {
		return musicTarget{}
	}
	var t musicTarget
	if be := mapValue(m, "browseEndpoint"); be != nil {
		t.BrowseID = stringValue(be["browseId"])
		if cfg := mapValue(be, "browseEndpointContextSupportedConfigs"); cfg != nil {
			if mc := mapValue(cfg, "browseEndpointContextMusicConfig"); mc != nil {
				t.PageType = stringValue(mc["pageType"])
			}
		}
	}
	if we := mapValue(m, "watchEndpoint"); we != nil {
		t.VideoID = stringValue(we["videoId"])
		t.PlaylistID = stringValue(we["playlistId"])
		if cfg := mapValue(we, "watchEndpointMusicSupportedConfigs"); cfg != nil {
			if mc := mapValue(cfg, "watchEndpointMusicConfig"); mc != nil {
				t.MusicVideoType = musicShortVideoType(stringValue(mc["musicVideoType"]))
			}
		}
	}
	if we := mapValue(m, "watchPlaylistEndpoint"); we != nil && t.PlaylistID == "" {
		t.PlaylistID = stringValue(we["playlistId"])
	}
	return t
}

// musicShortVideoType drops the MUSIC_VIDEO_TYPE_ prefix, which is on every
// value and says nothing.
func musicShortVideoType(s string) string {
	return strings.TrimPrefix(s, "MUSIC_VIDEO_TYPE_")
}

// musicFirstWatchTarget finds the first watch endpoint under v. It is how a row
// with no navigationEndpoint of its own, which is every row on an album page,
// still says which video it is: the play button in its overlay knows.
func musicFirstWatchTarget(v any) musicTarget {
	var found musicTarget
	walkJSON(v, func(m map[string]any) {
		if found.VideoID != "" {
			return
		}
		if we, ok := m["watchEndpoint"].(map[string]any); ok {
			if id := stringValue(we["videoId"]); id != "" {
				found = musicEndpointTarget(m)
			}
		}
	})
	return found
}

// musicAlbumPlaylistID finds the OLAK5uy_ playlist that plays an album.
//
// The menu of an album lockup holds two playlist ids: the album itself and the
// radio built from it, which is the album id with RDAMPL in front. Radio is a
// per-viewer stream and not the album, so an id that starts with RD is skipped
// rather than ranked, because the two are not two spellings of one thing.
func musicAlbumPlaylistID(v any) string {
	var found string
	walkJSON(v, func(m map[string]any) {
		if found != "" {
			return
		}
		for _, key := range []string{"watchPlaylistEndpoint", "watchEndpoint"} {
			if ep, ok := m[key].(map[string]any); ok {
				id := stringValue(ep["playlistId"])
				if id != "" && !strings.HasPrefix(id, "RD") {
					found = id
					return
				}
			}
		}
	})
	return found
}

// --- runs and bylines ---

// musicRun is one run of a rendered line with whatever endpoint it carried. A
// byline is not a string with bullets in it, it is a list of these, and half of
// them name a node.
type musicRun struct {
	Text   string
	Target musicTarget
}

// musicRuns pulls the runs out of a runs-style field.
func musicRuns(v any) []musicRun {
	m := mapValue(v, "")
	if m == nil {
		return nil
	}
	var out []musicRun
	for _, item := range arrayValue(m["runs"]) {
		rm, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, musicRun{
			Text:   stringValue(rm["text"]),
			Target: musicEndpointTarget(rm["navigationEndpoint"]),
		})
	}
	return out
}

// musicByline is a rendered line taken apart.
//
// The runs that carry an endpoint are read by type. The rest are read by shape:
// a four-digit number is a year, digits with colons are a duration, and anything
// else with a digit in it is a count, whatever the unit word next to it says.
// One untyped run with no digits at all is the type word, "Album" or "Single",
// which is the only field here that is a rendered word and it is stored as one.
type musicByline struct {
	Text         string
	ArtistNames  []string
	ArtistIDs    []string
	ChannelID    string
	ChannelName  string
	AlbumID      string
	AlbumTitle   string
	PlaylistID   string
	Year         string
	DurationText string
	CountText    string
	Handle       string
	TypeWord     string
	Plain        []string
}

var (
	musicClockRe = regexp.MustCompile(`^\d{1,3}(:[0-5]\d){1,2}$`)
	musicDigitRe = regexp.MustCompile(`\d`)
	musicCountRe = regexp.MustCompile(`^[0-9]`)
	musicYearRe  = regexp.MustCompile(`^(19|20)\d{2}$`)
)

// parseMusicByline reads one rendered line.
func parseMusicByline(v any) musicByline {
	b := musicByline{Text: extractText(v)}
	for _, run := range musicRuns(v) {
		text := strings.TrimSpace(run.Text)
		switch run.Target.kind() {
		case musicKindArtist:
			if run.Target.PageType == musicPageUserChannel {
				if b.ChannelID == "" {
					b.ChannelID, b.ChannelName = run.Target.id(), text
				}
				continue
			}
			b.ArtistNames = append(b.ArtistNames, text)
			b.ArtistIDs = append(b.ArtistIDs, run.Target.id())
			continue
		case musicKindAlbum:
			if b.AlbumID == "" {
				b.AlbumID, b.AlbumTitle = run.Target.id(), text
			}
			continue
		case musicKindPlaylist:
			if b.PlaylistID == "" {
				b.PlaylistID = run.Target.id()
			}
			continue
		}
		if musicIsSeparator(text) {
			continue
		}
		switch {
		case musicYearRe.MatchString(text) && b.Year == "":
			b.Year = text
		case musicClockRe.MatchString(text) && b.DurationText == "":
			b.DurationText = text
		// A handle is a shape and not a word, so it is safe to read as one. It has
		// to come before the count, because @rickhardt2237 has digits in it and the
		// count test is only "there is a digit in here".
		case strings.HasPrefix(text, "@") && b.Handle == "":
			b.Handle = text
		// A count leads with its number: "2B plays", "116 songs", "121 views". A
		// podcast episode renders "Oct 5, 2025" in the same slot, which has digits
		// in it and is not a count of anything, so the digit has to be the first
		// thing in the string rather than anywhere in it.
		case musicCountRe.MatchString(text) && b.CountText == "":
			b.CountText = text
		case !musicDigitRe.MatchString(text) && b.TypeWord == "" && len(b.Plain) == 0:
			b.TypeWord = text
		default:
			b.Plain = append(b.Plain, text)
		}
	}
	return b
}

// merge fills this byline's empty fields from another. A row spreads one line
// across two or three flex columns and each of them is parsed on its own.
func (b *musicByline) merge(other musicByline) {
	if len(b.ArtistNames) == 0 {
		b.ArtistNames, b.ArtistIDs = other.ArtistNames, other.ArtistIDs
	}
	for _, pair := range []struct {
		dst *string
		src string
	}{
		{&b.ChannelID, other.ChannelID}, {&b.ChannelName, other.ChannelName},
		{&b.AlbumID, other.AlbumID}, {&b.AlbumTitle, other.AlbumTitle},
		{&b.PlaylistID, other.PlaylistID}, {&b.Year, other.Year},
		{&b.DurationText, other.DurationText}, {&b.CountText, other.CountText},
		{&b.Handle, other.Handle}, {&b.TypeWord, other.TypeWord},
	} {
		if *pair.dst == "" {
			*pair.dst = pair.src
		}
	}
	b.Plain = append(b.Plain, other.Plain...)
}

// musicIsSeparator reports whether a run is punctuation between two facts.
func musicIsSeparator(s string) bool {
	s = strings.TrimSpace(s)
	return s == "" || s == "•" || s == "·" || s == "-" || s == "&" || s == ","
}

// --- rows ---

// musicFlexColumns returns the flex column text fields of a list row, unparsed,
// so the caller can read column 0 as a title and the rest as a byline.
func musicFlexColumns(r map[string]any) []any {
	var out []any
	for _, item := range arrayValue(r["flexColumns"]) {
		if m := mapValue(item, "musicResponsiveListItemFlexColumnRenderer"); m != nil {
			out = append(out, m["text"])
		}
	}
	return out
}

// musicFixedColumns returns the fixed column text fields, which is where a row
// with a fixed layout, an album page, puts the duration.
func musicFixedColumns(r map[string]any) []any {
	var out []any
	for _, item := range arrayValue(r["fixedColumns"]) {
		if m := mapValue(item, "musicResponsiveListItemFixedColumnRenderer"); m != nil {
			out = append(out, m["text"])
		}
	}
	return out
}

// musicRowByline merges every column after the title into one byline.
func musicRowByline(r map[string]any) musicByline {
	cols := musicFlexColumns(r)
	var b musicByline
	for i, col := range cols {
		if i == 0 {
			continue
		}
		b.merge(parseMusicByline(col))
	}
	for _, col := range musicFixedColumns(r) {
		b.merge(parseMusicByline(col))
	}
	b.fillFromMenu(r["menu"])
	return b
}

// fillFromMenu takes the artist and the album off a row's own menu.
//
// A search row under the top result card prints "Song • 3:34" and nothing else,
// because the card above it already says whose song it is, so the byline has no
// artist to give. The menu still does: "go to artist" and "go to album" are
// typed browse endpoints on every track row. The ids arrive without names, and
// that is honest, an id is the identity and the label was never on the page.
func (b *musicByline) fillFromMenu(v any) {
	if len(b.ArtistIDs) > 0 && b.AlbumID != "" {
		return
	}
	walkJSON(v, func(m map[string]any) {
		t := musicEndpointTarget(m)
		switch {
		case t.PageType == musicPageArtist && len(b.ArtistIDs) == 0:
			b.ArtistIDs = []string{t.BrowseID}
		case t.PageType == musicPageAlbum && b.AlbumID == "":
			b.AlbumID = t.BrowseID
		}
	})
}

// musicRowTitle is the first flex column, which is the row's own name.
func musicRowTitle(r map[string]any) string {
	if cols := musicFlexColumns(r); len(cols) > 0 {
		return extractText(cols[0])
	}
	return ""
}

// musicRowTarget picks the endpoint that says what a row is.
//
// The row's own navigationEndpoint is the answer when there is one. A track row
// usually has none: it is played rather than navigated to, so the play button in
// the overlay is the endpoint, and it is also the one carrying the
// musicVideoType.
func musicRowTarget(r map[string]any) musicTarget {
	if t := musicEndpointTarget(r["navigationEndpoint"]); !t.empty() {
		return t
	}
	if t := musicFirstWatchTarget(r["overlay"]); !t.empty() {
		return t
	}
	if cols := musicFlexColumns(r); len(cols) > 0 {
		for _, run := range musicRuns(cols[0]) {
			if !run.Target.empty() {
				return run.Target
			}
		}
	}
	if pid := mapValue(r, "playlistItemData"); pid != nil {
		if id := stringValue(pid["videoId"]); id != "" {
			return musicTarget{VideoID: id}
		}
	}
	return musicTarget{}
}

// musicRowThumbnails reads the thumbnail off a row or a lockup.
func musicRowThumbnails(r map[string]any) []Thumbnail {
	for _, key := range []string{"thumbnail", "thumbnailRenderer"} {
		outer := mapValue(r, key)
		if outer == nil {
			continue
		}
		inner := mapValue(outer, "musicThumbnailRenderer")
		if inner == nil {
			inner = mapValue(outer, "croppedSquareThumbnailRenderer")
		}
		if inner == nil {
			continue
		}
		if t := mapValue(inner, "thumbnail"); t != nil {
			if thumbs := ParseThumbnails(t["thumbnails"]); len(thumbs) > 0 {
				return thumbs
			}
		}
	}
	return nil
}

// musicIsExplicit reports whether a row carries the explicit badge.
func musicIsExplicit(r map[string]any) bool {
	found := false
	walkJSON(r, func(m map[string]any) {
		if found {
			return
		}
		if badge, ok := m["musicInlineBadgeRenderer"].(map[string]any); ok {
			icon := mapValue(badge, "icon")
			if icon == nil || strings.Contains(stringValue(icon["iconType"]), "EXPLICIT") {
				found = true
			}
		}
	})
	return found
}

// musicIndex reads the track number an album row renders in its index column.
func musicIndex(r map[string]any) int {
	if idx, ok := r["index"]; ok {
		return int(parseCountText(extractText(idx)))
	}
	return 0
}

// musicLine is a row or a card reduced to the parts a record is built from.
//
// A search row, an album track and the top result card are three renderers that
// say the same handful of things in three layouts. They are read into this once
// and built from it once, so the four builders below never learn the difference.
type musicLine struct {
	Target   musicTarget
	Title    string
	Byline   musicByline
	Thumbs   []Thumbnail
	Explicit bool
	Position int
	// Raw is the renderer itself, for the parts that are read out of its shape
	// rather than off its text: an album lockup keeps its playlist id in its menu.
	Raw map[string]any
}

// musicRowLine reads a musicResponsiveListItemRenderer, the list shape.
func musicRowLine(r map[string]any) musicLine {
	return musicLine{
		Target:   musicRowTarget(r),
		Title:    musicRowTitle(r),
		Byline:   musicRowByline(r),
		Thumbs:   musicRowThumbnails(r),
		Explicit: musicIsExplicit(r),
		Position: musicIndex(r),
		Raw:      r,
	}
}

// musicCardLine reads a musicCardShelfRenderer, the top result.
//
// The card is the best answer to the search and it is a record like any other:
// the endpoint on its title run says which kind, so a top result artist is an
// Artist and a top result song is a Track. Its own contents are ordinary rows
// and are emitted separately, which is why the caller carries a seen set.
func musicCardLine(c map[string]any) musicLine {
	t := musicEndpointTarget(c["onTap"])
	if t.empty() {
		for _, run := range musicRuns(c["title"]) {
			if !run.Target.empty() {
				t = run.Target
				break
			}
		}
	}
	b := parseMusicByline(c["subtitle"])
	b.fillFromMenu(c["menu"])
	return musicLine{
		Target: t,
		Title:  extractText(c["title"]),
		Byline: b,
		Thumbs: musicRowThumbnails(c),
		Raw:    c,
	}
}

// musicParseRow turns one musicResponsiveListItemRenderer into the record its
// endpoint says it is: a Track, an Album, an Artist or a Playlist. It returns
// nil for a row that names nothing, which is what a header row or a divider is.
func musicParseRow(r map[string]any, source string) any {
	return musicParseLine(musicRowLine(r), source)
}

// musicParseCard turns the top result card into the record it names.
func musicParseCard(c map[string]any, source string) any {
	return musicParseLine(musicCardLine(c), source)
}

func musicParseLine(line musicLine, source string) any {
	switch line.Target.kind() {
	case musicKindTrack:
		return musicTrackFromLine(line, source)
	case musicKindAlbum:
		return musicAlbumFromLine(line, source)
	case musicKindArtist:
		return musicArtistFromLine(line, source)
	case musicKindPlaylist:
		return musicPlaylistFromLine(line, source)
	}
	return nil
}

// musicTrackFromLine builds a Track.
func musicTrackFromLine(line musicLine, source string) Track {
	b := line.Byline
	track := Track{
		VideoID:        line.Target.VideoID,
		URL:            musicWatchURL(line.Target.VideoID),
		Title:          line.Title,
		ArtistNames:    b.ArtistNames,
		ArtistIDs:      b.ArtistIDs,
		AlbumID:        b.AlbumID,
		AlbumTitle:     b.AlbumTitle,
		DurationText:   b.DurationText,
		PlaysText:      b.CountText,
		IsExplicit:     line.Explicit,
		MusicVideoType: line.Target.MusicVideoType,
		Year:           b.Year,
		Position:       line.Position,
		MetadataParts:  b.Plain,
		Thumbnails:     line.Thumbs,
		Envelope:       musicEnvelope("track", source),
	}
	track.DurationSeconds = parseDurationSeconds(track.DurationText)
	return track
}

// musicAlbumFromLine builds an Album.
func musicAlbumFromLine(line musicLine, source string) Album {
	b := line.Byline
	return Album{
		AlbumID:     line.Target.id(),
		PlaylistID:  musicAlbumPlaylistID(line.Raw),
		URL:         musicBrowseURL(line.Target.id()),
		Title:       line.Title,
		ArtistNames: b.ArtistNames,
		ArtistIDs:   b.ArtistIDs,
		AlbumType:   b.TypeWord,
		Year:        b.Year,
		Thumbnails:  line.Thumbs,
		Envelope:    musicEnvelope("album", source),
	}
}

// musicArtistFromLine builds an Artist.
func musicArtistFromLine(line musicLine, source string) Artist {
	a := Artist{
		ArtistID:   line.Target.id(),
		URL:        musicArtistURL(line.Target.id()),
		Name:       line.Title,
		Thumbnails: line.Thumbs,
		Envelope:   musicEnvelope("artist", source),
	}
	// The count on a lockup is a number with a unit word and nothing typed to say
	// which number it is: the search row renders subscribers and the top result
	// card renders a monthly audience. Both fields on the record mean one of those
	// two, so the rendered line is kept as it came and neither is claimed. The
	// artist page states them properly and that read fills them in.
	if count := line.Byline.CountText; count != "" {
		a.MetadataParts = append(a.MetadataParts, count)
	}
	a.Handle = line.Byline.Handle
	if ytid.IsChannel(a.ArtistID) {
		a.ChannelID = a.ArtistID
	}
	a.miss("a search row is a lockup, so the discography and the top songs are on the artist page")
	return a
}

// musicPlaylistFromLine builds a Playlist. A music playlist is a playlist, so it
// goes in the record the rest of the tool already uses rather than in an album
// with its type set to the word playlist.
func musicPlaylistFromLine(line musicLine, source string) Playlist {
	b := line.Byline
	p := newPlaylist(line.Target.id(), SurfaceMusic)
	p.addClient("WEB_REMIX")
	p.addSource(source)
	p.Title = line.Title
	p.ChannelID, p.ChannelTitle = b.ChannelID, b.ChannelName
	if p.ChannelTitle == "" && len(b.ArtistNames) > 0 {
		p.ChannelID, p.ChannelTitle = firstOrEmpty(b.ArtistIDs), b.ArtistNames[0]
	}
	// The count is unlabelled the same way an artist's is. One of YouTube Music's
	// own playlists renders "116 songs" and a community one renders "121 views",
	// and nothing but the word separates them, so filing either as a video count
	// would put a view count in the item count half the time.
	if b.CountText != "" {
		p.MetadataParts = append(p.MetadataParts, b.CountText)
	}
	p.Thumbnails = line.Thumbs
	return p
}

// musicItemFromTwoRow turns a musicTwoRowItemRenderer, the card shape a carousel
// is made of, into a lockup.
func musicItemFromTwoRow(r map[string]any, shelf string) MusicItem {
	t := musicEndpointTarget(r["navigationEndpoint"])
	b := parseMusicByline(r["subtitle"])
	item := MusicItem{
		Kind:           t.kind(),
		ID:             t.id(),
		Title:          extractText(r["title"]),
		Subtitle:       b.Text,
		Shelf:          shelf,
		AlbumType:      b.TypeWord,
		Year:           b.Year,
		MusicVideoType: t.MusicVideoType,
		CountText:      b.CountText,
		ArtistNames:    b.ArtistNames,
		ArtistIDs:      b.ArtistIDs,
		Thumbnails:     musicRowThumbnails(r),
	}
	switch item.Kind {
	case musicKindTrack:
		item.URL = musicWatchURL(item.ID)
	case musicKindAlbum:
		item.URL = musicBrowseURL(item.ID)
		item.PlaylistID = musicAlbumPlaylistID(r)
	case musicKindArtist:
		item.URL = musicArtistURL(item.ID)
	case musicKindPlaylist:
		item.URL = musicPlaylistURL(item.ID)
	}
	return item
}

// --- search ---

// MusicSearch streams search results for a query and an optional type filter.
// emit receives a Track, an Album, an Artist or a Playlist. Iteration stops on
// ErrStop.
//
// Rows are classified one at a time by their own endpoints, so the unfiltered
// search works: its results arrive under itemSectionRenderer with no shelf
// heading above them to read a type out of, which is why this used to return
// nothing without --type.
func (c *Client) MusicSearch(ctx context.Context, query, typ string, opt PageOptions, emit func(any) error) error {
	it := NewInnerTube(c)
	params := musicSearchParams(typ)
	source := musicBaseURL + "/search?q=" + url.QueryEscape(query)

	seen := map[string]bool{}
	total := 0
	pages := 0
	continuation := ""

	for opt.MaxPages <= 0 || pages < opt.MaxPages {
		data, err := it.MusicSearch(ctx, query, params, continuation)
		if err != nil {
			return err
		}
		pages++

		emitted, err := musicEmitRows(data, source, seen, emit, opt.Max-total)
		total += emitted
		if err != nil {
			if errors.Is(err, ErrStop) {
				return nil
			}
			return err
		}
		if opt.Max > 0 && total >= opt.Max {
			break
		}
		continuation = FindContinuationToken(data)
		if continuation == "" {
			break
		}
	}
	return nil
}

// musicEmitRows walks a response and emits every list row it recognises, in
// payload order, skipping an id it has already emitted.
//
// The top result is a card holding three rows of its own, and those rows are
// also rows of the response, which is why the caller passes a seen set that
// outlives the page.
func musicEmitRows(data any, source string, seen map[string]bool, emit func(any) error, limit int) (int, error) {
	count := 0
	var emitErr error

	walkJSON(data, func(m map[string]any) {
		if emitErr != nil || (limit > 0 && count >= limit) {
			return
		}
		var val any
		switch {
		case m["musicResponsiveListItemRenderer"] != nil:
			r, ok := m["musicResponsiveListItemRenderer"].(map[string]any)
			if !ok {
				return
			}
			val = musicParseRow(r, source)
		case m["musicCardShelfRenderer"] != nil:
			card, ok := m["musicCardShelfRenderer"].(map[string]any)
			if !ok {
				return
			}
			val = musicParseCard(card, source)
		default:
			return
		}
		if val == nil {
			return
		}
		key := musicRecordKey(val)
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		if err := emit(val); err != nil {
			emitErr = err
			return
		}
		count++
	})

	return count, emitErr
}

// musicRecordKey is the kind and id of an emitted record, for the seen set.
func musicRecordKey(v any) string {
	switch r := v.(type) {
	case Track:
		return "track/" + r.VideoID
	case Album:
		return "album/" + r.AlbumID
	case Artist:
		return "artist/" + r.ArtistID
	case Playlist:
		return "playlist/" + r.PlaylistID
	}
	return ""
}

// --- artist ---

// FetchArtist reads an artist page: the header, the top songs shelf and every
// carousel on it.
//
// The shelves are sorted by what their items are rather than by what the shelf
// is called, so "Live performances" lands in Videos with its heading kept as a
// label. Albums and singles are both album lockups and the page does not type
// them apart, but a single states its own type word where an album shelf entry
// states only a year, and that survives a language change where the two shelf
// headings do not.
func (c *Client) FetchArtist(ctx context.Context, idOrURL string) (*Artist, error) {
	browseID := musicBrowseID(idOrURL)
	if browseID == "" {
		return nil, errors.New("FetchArtist: empty browse id")
	}

	it := NewInnerTube(c)
	data, err := it.MusicBrowse(ctx, browseID, "", "")
	if err != nil {
		return nil, err
	}

	source := musicArtistURL(browseID)
	a := &Artist{ArtistID: browseID, Envelope: musicEnvelope("artist", source)}

	walkJSON(data, func(m map[string]any) {
		if h, ok := m["musicImmersiveHeaderRenderer"].(map[string]any); ok {
			musicFillArtistHeader(a, h)
		}
		if h, ok := m["musicVisualHeaderRenderer"].(map[string]any); ok {
			musicFillArtistHeader(a, h)
		}
		if h, ok := m["musicResponsiveHeaderRenderer"].(map[string]any); ok {
			musicFillArtistHeader(a, h)
		}
		if sub, ok := m["subscribeButtonRenderer"].(map[string]any); ok {
			if a.ChannelID == "" {
				a.ChannelID = stringValue(sub["channelId"])
			}
			if a.SubscriberCountText == "" {
				a.SubscriberCountText = extractText(sub["longSubscriberCountText"])
				if a.SubscriberCountText == "" {
					a.SubscriberCountText = extractText(sub["subscriberCountText"])
				}
				a.SubscriberCount = parseCountText(a.SubscriberCountText)
			}
		}
	})

	// The artist a page is about is not always the id it was asked for: browsing a
	// person's user channel lands on their official artist channel, and the
	// discography then links the other id. The shelves are the ones that name the
	// artist the catalogue is filed under, so they win over the request.
	if id := musicArtistIDFromShelves(data); id != "" {
		a.ArtistID = id
	}
	if a.ChannelID == "" && ytid.IsChannel(a.ArtistID) {
		a.ChannelID = a.ArtistID
	}
	a.URL = musicArtistURL(a.ArtistID)

	musicFillArtistShelves(a, data, source)
	if a.MonthlyListenersText == "" {
		a.miss("this page states no monthly listener count")
	}
	a.miss("the discography shelves are one page each: more releases sit behind their own browse id")
	return a, nil
}

// musicFillArtistHeader reads whichever of the three header shapes answered.
func musicFillArtistHeader(a *Artist, h map[string]any) {
	if a.Name == "" {
		a.Name = extractText(h["title"])
	}
	if a.Description == "" {
		a.Description = extractText(h["description"])
	}
	if a.MonthlyListenersText == "" {
		a.MonthlyListenersText = extractText(h["monthlyListenerCount"])
	}
	if len(a.Thumbnails) == 0 {
		a.Thumbnails = musicRowThumbnails(h)
	}
}

// musicArtistIDFromShelves finds the artist id the page's own rows point at.
func musicArtistIDFromShelves(data any) string {
	counts := map[string]int{}
	best, bestN := "", 0
	walkJSON(data, func(m map[string]any) {
		if r, ok := m["musicResponsiveListItemRenderer"].(map[string]any); ok {
			for _, id := range musicRowByline(r).ArtistIDs {
				counts[id]++
				if counts[id] > bestN {
					best, bestN = id, counts[id]
				}
			}
		}
	})
	return best
}

// musicFillArtistShelves sorts every shelf on the page into the artist's lists.
func musicFillArtistShelves(a *Artist, data any, source string) {
	walkJSON(data, func(m map[string]any) {
		if shelf, ok := m["musicCarouselShelfRenderer"].(map[string]any); ok {
			title := musicShelfTitle(shelf)
			for _, item := range arrayValue(shelf["contents"]) {
				im, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if two, ok := im["musicTwoRowItemRenderer"].(map[string]any); ok {
					a.addItem(musicItemFromTwoRow(two, title))
					continue
				}
				if row, ok := im["musicResponsiveListItemRenderer"].(map[string]any); ok {
					if track, ok := musicParseRow(row, source).(Track); ok {
						a.TopTracks = append(a.TopTracks, track)
					}
				}
			}
		}
		if shelf, ok := m["musicShelfRenderer"].(map[string]any); ok {
			source := source
			for _, item := range arrayValue(shelf["contents"]) {
				im, ok := item.(map[string]any)
				if !ok {
					continue
				}
				row, ok := im["musicResponsiveListItemRenderer"].(map[string]any)
				if !ok {
					continue
				}
				if track, ok := musicParseRow(row, source).(Track); ok {
					a.TopTracks = append(a.TopTracks, track)
				}
			}
		}
	})
}

// addItem files one lockup under the list its own type belongs in.
func (a *Artist) addItem(item MusicItem) {
	switch item.Kind {
	case musicKindAlbum:
		if item.AlbumType == "" {
			a.Albums = append(a.Albums, item)
		} else {
			a.Singles = append(a.Singles, item)
		}
	case musicKindTrack:
		a.Videos = append(a.Videos, item)
	case musicKindPlaylist:
		a.Playlists = append(a.Playlists, item)
	case musicKindArtist:
		a.RelatedArtists = append(a.RelatedArtists, item)
	}
}

// musicShelfTitle is the rendered heading of a carousel, kept as a label.
func musicShelfTitle(shelf map[string]any) string {
	header := mapValue(shelf, "header")
	if header == nil {
		return ""
	}
	if basic := mapValue(header, "musicCarouselShelfBasicHeaderRenderer"); basic != nil {
		return extractText(basic["title"])
	}
	return extractText(header["title"])
}

// --- album ---

// FetchAlbum reads an album page: the header and its track list.
//
// The tracks come back as their own records rather than as fields of the album,
// because each one is a video id and a node of its own.
func (c *Client) FetchAlbum(ctx context.Context, idOrURL string) (*Album, []Track, error) {
	browseID := musicBrowseID(idOrURL)
	if browseID == "" {
		return nil, nil, errors.New("FetchAlbum: empty browse id")
	}

	it := NewInnerTube(c)
	data, err := it.MusicBrowse(ctx, browseID, "", "")
	if err != nil {
		return nil, nil, err
	}

	alb, tracks := parseAlbumPage(data, browseID)
	return alb, tracks, nil
}

// parseAlbumPage turns an album browse into the album and its tracks. It is the
// whole of the read and none of the fetch, so a saved page reads the same way a
// live one does.
func parseAlbumPage(data any, browseID string) (*Album, []Track) {
	source := musicBrowseURL(browseID)
	alb := &Album{AlbumID: browseID, URL: source, Envelope: musicEnvelope("album", source)}

	walkJSON(data, func(m map[string]any) {
		h, ok := m["musicResponsiveHeaderRenderer"].(map[string]any)
		if !ok {
			if h, ok = m["musicDetailHeaderRenderer"].(map[string]any); !ok {
				return
			}
		}
		if alb.Title == "" {
			alb.Title = extractText(h["title"])
		}
		sub := parseMusicByline(h["subtitle"])
		if alb.AlbumType == "" {
			alb.AlbumType = sub.TypeWord
		}
		if alb.Year == "" {
			alb.Year = sub.Year
		}
		// The credit sits in its own field on the newer header and is a run of the
		// subtitle on the older one, and only the field carries the artist's id.
		strap := parseMusicByline(h["straplineTextOne"])
		if len(strap.ArtistNames) > 0 {
			alb.ArtistNames, alb.ArtistIDs = strap.ArtistNames, strap.ArtistIDs
		} else if len(sub.ArtistNames) > 0 {
			alb.ArtistNames, alb.ArtistIDs = sub.ArtistNames, sub.ArtistIDs
		} else if len(sub.Plain) > 0 && len(alb.ArtistNames) == 0 {
			alb.ArtistNames = []string{sub.Plain[0]}
		}
		// The second subtitle is a count and a running time, in that order and with
		// no type on either, so they are read by position.
		if counts := musicRunTexts(h["secondSubtitle"]); len(counts) > 0 {
			alb.TrackCountText = counts[0]
			if len(counts) > 1 {
				alb.DurationText = counts[len(counts)-1]
			}
		}
		if alb.Description == "" {
			alb.Description = extractText(mapValue(h, "description"))
		}
		if len(alb.Thumbnails) == 0 {
			alb.Thumbnails = musicRowThumbnails(h)
		}
	})

	if alb.Description == "" {
		walkJSON(data, func(m map[string]any) {
			if d, ok := m["musicDescriptionShelfRenderer"].(map[string]any); ok && alb.Description == "" {
				alb.Description = extractText(d["description"])
			}
		})
	}
	alb.PlaylistID = musicAlbumPlaylistID(data)

	var tracks []Track
	position := 0
	walkJSON(data, func(m map[string]any) {
		row, ok := m["musicResponsiveListItemRenderer"].(map[string]any)
		if !ok {
			return
		}
		track, ok := musicParseRow(row, source).(Track)
		if !ok || track.VideoID == "" {
			return
		}
		position++
		if track.Position == 0 {
			track.Position = position
		}
		if track.AlbumID == "" {
			track.AlbumID, track.AlbumTitle = alb.AlbumID, alb.Title
		}
		if len(track.ArtistNames) == 0 {
			track.ArtistNames, track.ArtistIDs = alb.ArtistNames, alb.ArtistIDs
		}
		if track.Year == "" {
			track.Year = alb.Year
		}
		tracks = append(tracks, track)
	})
	alb.TrackCount = len(tracks)
	if alb.TrackCountText != "" && parseCountText(alb.TrackCountText) != int64(alb.TrackCount) {
		alb.miss("the header says %s and this read saw %d, so a track is not available here", alb.TrackCountText, alb.TrackCount)
	}

	return alb, tracks
}

// --- playlist ---

// FetchMusicPlaylist reads a music playlist and its tracks, following every
// continuation.
func (c *Client) FetchMusicPlaylist(ctx context.Context, idOrURL string) (*Playlist, []Track, error) {
	raw := musicBrowseID(idOrURL)
	if raw == "" {
		return nil, nil, errors.New("FetchMusicPlaylist: empty playlist id")
	}

	// A playlist browses under VL<id>. The prefix belongs to the request and
	// nowhere else, so the record keeps the bare id.
	playlistID := ytid.StripWire(raw)
	source := musicPlaylistURL(playlistID)

	it := NewInnerTube(c)
	data, err := it.MusicBrowse(ctx, ytid.WireID(playlistID), "", "")
	if err != nil {
		return nil, nil, err
	}

	p := newPlaylist(playlistID, SurfaceMusic)
	p.addClient("WEB_REMIX")
	p.addSource(source)
	p.URL = source

	walkJSON(data, func(m map[string]any) {
		h, ok := m["musicResponsiveHeaderRenderer"].(map[string]any)
		if !ok {
			if h, ok = m["musicDetailHeaderRenderer"].(map[string]any); !ok {
				return
			}
		}
		if p.Title == "" {
			p.Title = extractText(h["title"])
		}
		if p.Description == "" {
			p.Description = extractText(mapValue(h, "description"))
		}
		strap := parseMusicByline(h["straplineTextOne"])
		if p.ChannelTitle == "" && len(strap.ArtistNames) > 0 {
			p.ChannelTitle, p.ChannelID = strap.ArtistNames[0], firstOrEmpty(strap.ArtistIDs)
		}
		if p.ChannelTitle == "" && strap.ChannelName != "" {
			p.ChannelTitle, p.ChannelID = strap.ChannelName, strap.ChannelID
		}
		if counts := musicRunTexts(h["secondSubtitle"]); len(counts) > 0 && p.VideoCountText == "" {
			p.VideoCountText = counts[0]
			p.VideoCount = parseCountText(counts[0])
		}
		if len(p.Thumbnails) == 0 {
			p.Thumbnails = musicRowThumbnails(h)
		}
	})

	if p.Title == "" {
		// An album's own playlist browses with rows and no header at all: the left
		// column comes back empty and everything a person would call the header is
		// on the album page instead.
		if strings.HasPrefix(playlistID, "OLAK") {
			p.miss("an album's playlist has no header of its own, so the title and the credits are on the album page")
		} else {
			p.miss("this browse answered with no header, so the playlist arrives as its rows and nothing else")
		}
	}

	var tracks []Track
	collect := func(root any) {
		walkJSON(root, func(m map[string]any) {
			row, ok := m["musicResponsiveListItemRenderer"].(map[string]any)
			if !ok {
				return
			}
			if track, ok := musicParseRow(row, source).(Track); ok && track.VideoID != "" {
				track.Position = len(tracks) + 1
				tracks = append(tracks, track)
			}
		})
	}
	collect(data)

	for token := FindContinuationToken(data); token != ""; {
		next, err := it.MusicBrowse(ctx, "", "", token)
		if err != nil {
			p.miss("a continuation failed, so the track list stops at %d", len(tracks))
			break
		}
		collect(next)
		token = FindContinuationToken(next)
	}

	if !ytid.IsChannel(p.ChannelID) {
		p.ChannelID = ""
	}
	return &p, tracks, nil
}

// --- track ---

// FetchTrack reads one track through the music app, and its lyrics with them.
//
// This goes to /next rather than /player. WEB_REMIX /player is refused for a
// signed out read, on every id tried, and /next answers with everything a track
// record needs: the title, the byline with the artist and the album both typed
// and linked, the running time, and the id the music app files the song under,
// which is not always the id it was asked about.
func (c *Client) FetchTrack(ctx context.Context, videoID string, withLyrics bool) (*Track, error) {
	if videoID == "" {
		return nil, errors.New("FetchTrack: empty video id")
	}

	it := NewInnerTube(c)
	data, err := it.MusicNext(ctx, map[string]any{
		"videoId":                       videoID,
		"isAudioOnly":                   true,
		"enablePersistentPlaylistPanel": true,
	}, "music track "+videoID)
	if err != nil {
		return nil, err
	}

	source := musicWatchURL(videoID)
	track := &Track{VideoID: videoID, URL: source, Envelope: musicEnvelope("track", source)}

	var row map[string]any
	walkJSON(data, func(m map[string]any) {
		if row != nil {
			return
		}
		if r, ok := m["playlistPanelVideoRenderer"].(map[string]any); ok {
			row = r
		}
	})
	if row == nil {
		return nil, errors.New("FetchTrack: the music app returned no track for " + videoID)
	}

	t := musicEndpointTarget(row["navigationEndpoint"])
	if t.VideoID != "" {
		track.VideoID = t.VideoID
		track.URL = musicWatchURL(t.VideoID)
	}
	track.MusicVideoType = t.MusicVideoType
	track.Title = extractText(row["title"])
	b := parseMusicByline(row["longBylineText"])
	track.ArtistNames, track.ArtistIDs = b.ArtistNames, b.ArtistIDs
	track.AlbumID, track.AlbumTitle = b.AlbumID, b.AlbumTitle
	track.Year = b.Year
	track.DurationText = extractText(row["lengthText"])
	track.DurationSeconds = parseDurationSeconds(track.DurationText)
	track.IsExplicit = musicIsExplicit(row)
	track.Thumbnails = musicRowThumbnails(row)
	track.miss("/next states no play count, which only a listing row carries")

	if !withLyrics {
		return track, nil
	}
	lyricsID := musicLyricsBrowseID(data)
	if lyricsID == "" {
		track.miss("this track has no lyrics tab")
		return track, nil
	}
	lyricsData, err := it.MusicBrowse(ctx, lyricsID, "", "")
	if err != nil {
		track.miss("the lyrics tab did not answer: %v", err)
		return track, nil
	}
	track.addSource(musicBrowseURL(lyricsID))
	track.Lyrics, track.LyricsSource = musicParseLyrics(lyricsData)
	if track.Lyrics == "" {
		// The tab exists and the page still says no. Whatever it said is the
		// answer, so it is kept as the answer rather than reported as nothing.
		if said := musicMessage(lyricsData); said != "" {
			track.miss("the lyrics tab says: %s", said)
		} else {
			track.miss("the lyrics tab answered with no lyrics in it")
		}
	}
	return track, nil
}

// musicMessage reads a messageRenderer, which is how music.youtube.com says no.
// The text is the site's own wording and is kept as it came.
func musicMessage(data any) string {
	var said string
	walkJSON(data, func(m map[string]any) {
		if said != "" {
			return
		}
		if msg, ok := m["messageRenderer"].(map[string]any); ok {
			said = extractText(msg["text"])
		}
	})
	return said
}

// musicLyricsBrowseID finds the lyrics tab by its page type. The tab is titled
// "Lyrics" in English and something else in every other language, and the type
// is on the endpoint either way.
func musicLyricsBrowseID(data any) string {
	var found string
	walkJSON(data, func(m map[string]any) {
		if found != "" {
			return
		}
		tab, ok := m["tabRenderer"].(map[string]any)
		if !ok {
			return
		}
		for _, key := range []string{"endpoint", "unselectedEndpoint"} {
			if t := musicEndpointTarget(tab[key]); t.PageType == musicPageLyrics {
				found = t.BrowseID
				return
			}
		}
	})
	return found
}

// musicParseLyrics reads a lyrics page, returning the text and its credit.
func musicParseLyrics(data any) (string, string) {
	var text, source string
	walkJSON(data, func(m map[string]any) {
		if text != "" {
			return
		}
		if shelf, ok := m["musicDescriptionShelfRenderer"].(map[string]any); ok {
			text = extractLines(shelf["description"])
			source = extractText(shelf["footer"])
		}
	})
	if text != "" {
		return text, source
	}
	// The timed shape is the same lyrics one line at a time, with a start offset
	// per line that nothing here needs.
	walkJSON(data, func(m map[string]any) {
		if text != "" {
			return
		}
		model, ok := m["timedLyricsModel"].(map[string]any)
		if !ok {
			return
		}
		lyricsData := mapValue(model, "lyricsData")
		if lyricsData == nil {
			return
		}
		var lines []string
		for _, line := range arrayValue(lyricsData["lines"]) {
			if lm, ok := line.(map[string]any); ok {
				if s := stringValue(lm["lyricLine"]); s != "" {
					lines = append(lines, s)
				}
			}
		}
		text = strings.Join(lines, "\n")
		source = extractText(lyricsData["sourceMessage"])
	})
	return text, source
}

// --- small helpers ---

// musicEnvelope stamps a music record. Every read here is one surface and one
// client, so this is the whole of it.
func musicEnvelope(kind, source string) Envelope {
	e := newEnvelope(kind, SurfaceMusic)
	e.addClient("WEB_REMIX")
	e.addSource(source)
	return e
}

// musicRunTexts returns the non-separator run texts of a rendered line.
func musicRunTexts(v any) []string {
	var out []string
	for _, run := range musicRuns(v) {
		if text := strings.TrimSpace(run.Text); !musicIsSeparator(text) {
			out = append(out, text)
		}
	}
	return out
}

func musicWatchURL(id string) string {
	if id == "" {
		return ""
	}
	return musicBaseURL + "/watch?v=" + id
}

func musicBrowseURL(id string) string {
	if id == "" {
		return ""
	}
	return musicBaseURL + "/browse/" + id
}

func musicPlaylistURL(id string) string {
	if id == "" {
		return ""
	}
	return musicBaseURL + "/playlist?list=" + id
}

// musicArtistURL is /channel for an artist with a channel and /browse for one
// with only a music id, which is the address each of them actually has.
func musicArtistURL(id string) string {
	if id == "" {
		return ""
	}
	if ytid.IsChannel(id) {
		return musicBaseURL + "/channel/" + id
	}
	return musicBrowseURL(id)
}

func firstOrEmpty(list []string) string {
	if len(list) == 0 {
		return ""
	}
	return list[0]
}
