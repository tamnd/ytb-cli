// Package ytid classifies and derives YouTube ids with no network at all.
//
// YouTube's ids are the most regular of any site in this series, and four of the
// shapes decode locally. A channel id is five playlist ids. A UU-family playlist
// id is a channel id. A reply id carries its parent in front of a dot. A VL
// prefix is routing rather than identity. Everything in here is one of those
// facts, so the package is importable on its own and never reaches the network.
//
// What it deliberately does not do is decode a comment or post id. Both are
// base64 protobuf and both can be taken apart, and what is inside is a
// client-side path rather than an entity id, so a tool that reported a decoded
// field YouTube may change next month would be worse than one that reports the id
// it was given. The dot in a reply id is the exception, because a dot is a
// delimiter rather than an encoding.
package ytid

import (
	"net/url"
	"regexp"
	"strings"
)

// Kind is what an id names. The zero value means the input matched no shape.
type Kind string

const (
	Unknown Kind = ""
	// Video covers a video, a short, a stream and a music track, because they
	// share one id space and one watch page.
	Video   Kind = "video"
	Channel Kind = "channel"
	// Handle, LegacyUser and LegacyCustom are aliases rather than identities. A
	// channel can change its handle, so each of these needs a request before it
	// names anything.
	Handle       Kind = "handle"
	LegacyUser   Kind = "legacy_user"
	LegacyCustom Kind = "legacy_custom"
	Playlist     Kind = "playlist"
	// Uploads and the four kind playlists are all derived from a channel id.
	Uploads Kind = "uploads_playlist"
	Videos  Kind = "videos_playlist"
	Shorts  Kind = "shorts_playlist"
	Streams Kind = "streams_playlist"
	Popular Kind = "popular_playlist"
	Album   Kind = "album_playlist"
	Mix     Kind = "mix"
	// MusicAlbum and MusicArtist are music.youtube.com's own browse ids.
	MusicAlbum  Kind = "music_album"
	MusicArtist Kind = "music_artist"
	// Feed is a browse destination rather than an entity.
	Feed    Kind = "feed"
	Comment Kind = "comment"
	Reply   Kind = "reply"
	// Itag names a format and means nothing without a video.
	Itag Kind = "itag"
	// Hashtag is a feed keyed on a word.
	Hashtag Kind = "hashtag"
)

// Info is everything an id says about itself before any request goes out.
type Info struct {
	// Input is the string as it was given, so a caller can quote it back.
	Input string
	Kind  Kind
	// ID is the canonical form: the VL prefix removed, a URL reduced to the id it
	// carries, a handle kept with its @ because that is how YouTube writes it.
	ID string
	// ChannelID is filled when the id names a channel or derives one, which is
	// how a UU-family playlist in a payload names a channel nobody fetched.
	ChannelID string
	// ParentID is a reply's comment.
	ParentID string
	// Playlists are the five ids a channel id derives. Nil for everything else.
	Playlists *ChannelPlaylists
	// NeedsRequest is set on the three alias shapes, which name a channel only
	// after navigation/resolve_url has answered.
	NeedsRequest bool
	// Unviewable is why there is nothing to read, empty when there is. A mix is
	// the case that matters: RD<video id> browses to a 200 carrying "This playlist
	// type is unviewable." because a mix is generated per viewer, so reporting it
	// as unviewable beats handing back a URL that refuses.
	Unviewable string
	// Note is a fact about the id that a caller would otherwise get wrong.
	Note string
}

// ChannelPlaylists are the playlist ids a channel id derives, with no request.
// Doc 01 section 2.5 measured all five against one channel, and the three
// kind-specific ones partition the uploads playlist exactly: 139 videos plus 294
// shorts plus 2 streams is the 435 the uploads playlist reports.
type ChannelPlaylists struct {
	Uploads string
	Videos  string
	Shorts  string
	Streams string
	Popular string
	// Browse is the uploads playlist with the VL prefix on it. It goes on the wire
	// and never into a URI or the store, which is why it is named for what it is
	// used for rather than sitting next to the ids as if it were one.
	Browse string
	// Feed is the channel's RSS feed, the one surface here that is a URL. It is
	// derived from the channel id the same way and is the cheapest way to see a
	// channel's 15 most recent uploads.
	Feed string
}

// The suffix every channel-derived id shares: a channel id minus its UC.
const suffixLen = 22

var (
	videoRe    = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
	channelRe  = regexp.MustCompile(`^UC[A-Za-z0-9_-]{22}$`)
	handleRe   = regexp.MustCompile(`^@[A-Za-z0-9._-]{3,30}$`)
	playlistRe = regexp.MustCompile(`^PL[A-Za-z0-9_-]{16,32}$`)
	albumRe    = regexp.MustCompile(`^OLAK5uy_[A-Za-z0-9_-]{33}$`)
	musicAlbRe = regexp.MustCompile(`^MPREb_[A-Za-z0-9]{11}$`)
	feedRe     = regexp.MustCompile(`^FE[a-z][a-z0-9_]*$`)
	commentRe  = regexp.MustCompile(`^Ug[A-Za-z0-9_-]{20,}$`)
	itagRe     = regexp.MustCompile(`^[0-9]{1,3}$`)
	suffixRe   = regexp.MustCompile(`^[A-Za-z0-9_-]{22}$`)
)

// knownFeeds are the browse destinations the site ships with. The general shape
// is FE plus lowercase words, and FEmusic_home is exactly eleven characters,
// which is also a video id's length, so the list is checked first and the general
// pattern then refuses length eleven rather than reading a video as a feed.
var knownFeeds = map[string]bool{
	"FEwhat_to_watch":         true,
	"FEtrending":              true,
	"FEsubscriptions":         true,
	"FEhistory":               true,
	"FElibrary":               true,
	"FEmy_videos":             true,
	"FEmusic_home":            true,
	"FEmusic_explore":         true,
	"FEmusic_charts":          true,
	"FEmusic_library_landing": true,
	"FEmusic_liked_playlists": true,
}

// kindPrefixes maps a channel-derived playlist prefix to what it lists. Longest
// first, because UULF starts with UU and a shorter match would swallow it.
var kindPrefixes = []struct {
	prefix string
	kind   Kind
}{
	{"UULF", Videos},
	{"UUSH", Shorts},
	{"UULV", Streams},
	{"UULP", Popular},
	{"UU", Uploads},
}

// Classify reads an id, a reference or a URL and says what it names. It never
// makes a request, and it never guesses: an input that matches no shape comes
// back as Unknown rather than as a video id, because taking anything unrecognised
// for a video id is how a typo turns into a 404 from the far end.
func Classify(input string) Info {
	s := strings.TrimSpace(input)
	info := Info{Input: input, ID: s}
	if s == "" {
		return Info{Input: input}
	}
	if strings.Contains(s, "://") || strings.HasPrefix(s, "www.youtube.com/") || strings.HasPrefix(s, "youtube.com/") || strings.HasPrefix(s, "youtu.be/") {
		return fromURL(input, s)
	}
	if strings.HasPrefix(s, "/") {
		return fromPath(input, s)
	}
	if strings.HasPrefix(s, "@") {
		if handleRe.MatchString(s) {
			info.Kind = Handle
			info.NeedsRequest = true
			info.Note = "a handle is an alias a channel can change, so it is a property of a channel and never the key"
			return info
		}
		return Info{Input: input}
	}
	// VL is a routing prefix rather than an id. The kind is whatever the playlist
	// under it is, and the id reported is the one without the prefix.
	if rest := strings.TrimPrefix(s, "VL"); rest != s && rest != "" {
		inner := Classify(rest)
		if inner.Kind == Unknown {
			return Info{Input: input}
		}
		inner.Input = input
		inner.Note = joinNotes("VL is how a playlist is browsed and is not part of its id, so it goes on the wire and never into a URI or the store", inner.Note)
		return inner
	}
	switch {
	case channelRe.MatchString(s):
		info.Kind = Channel
		info.ChannelID = s
		pl := playlistsFor(s)
		info.Playlists = &pl
		info.Note = "the four kind playlists are derived, not fetched"
		return info
	case musicAlbRe.MatchString(s):
		info.Kind = MusicAlbum
		return info
	case strings.HasPrefix(s, "MPLA"):
		if id := strings.TrimPrefix(s, "MPLA"); channelRe.MatchString(id) {
			info.Kind = MusicArtist
			info.ChannelID = id
			info.Note = "an artist page is the channel's own id behind MPLA, so it names a channel without a request"
			return info
		}
		return Info{Input: input}
	case albumRe.MatchString(s):
		info.Kind = Album
		return info
	// Every mix form is longer than a video id: RD plus a video id is thirteen
	// characters, and RDCMUC, RDMM, RDEM and RDAMVM are longer again. The length
	// check is there because a video id starting with RD is an ordinary video.
	case strings.HasPrefix(s, "RD") && len(s) > 11:
		info.Kind = Mix
		info.Unviewable = "a mix is generated per viewer, and browsing one returns 200 with \"This playlist type is unviewable.\""
		return info
	case knownFeeds[s], feedRe.MatchString(s) && len(s) != 11:
		info.Kind = Feed
		info.Note = "a feed is a browse destination rather than an entity"
		return info
	case itagRe.MatchString(s):
		info.Kind = Itag
		info.Note = "an itag names a format and means nothing without a video"
		return info
	}
	for _, kp := range kindPrefixes {
		suffix := strings.TrimPrefix(s, kp.prefix)
		if suffix == s || !suffixRe.MatchString(suffix) {
			continue
		}
		info.Kind = kp.kind
		info.ChannelID = "UC" + suffix
		return info
	}
	if playlistRe.MatchString(s) {
		info.Kind = Playlist
		return info
	}
	// The three personal lists. LL and WL are those two letters and nothing else,
	// and FL carries the channel suffix, so none of them can swallow a video id.
	switch {
	case s == "LL":
		info.Kind = Playlist
		info.Note = "the signed-in account's liked videos, so it needs cookies"
		return info
	case s == "WL":
		info.Kind = Playlist
		info.Note = "the signed-in account's watch later, so it needs cookies"
		return info
	case strings.HasPrefix(s, "FL") && suffixRe.MatchString(strings.TrimPrefix(s, "FL")):
		info.Kind = Playlist
		info.ChannelID = "UC" + strings.TrimPrefix(s, "FL")
		info.Note = "a legacy favourites list, keyed on the channel that owns it"
		return info
	}
	if commentRe.MatchString(s) {
		info.Kind = Comment
		info.Note = "a community post id has the same shape as a comment id, so only the surface it arrived on tells them apart"
		return info
	}
	// A reply id is the parent's id, a dot, and a suffix.
	if parent, _, ok := strings.Cut(s, "."); ok && commentRe.MatchString(parent) {
		info.Kind = Reply
		info.ParentID = parent
		info.Note = "the dot is a delimiter, so a reply names its parent comment without a request"
		return info
	}
	if videoRe.MatchString(s) {
		info.Kind = Video
		info.Note = "a video id is eleven characters of base64url and decodes to nothing else"
		return info
	}
	return Info{Input: input}
}

// PlaylistsFor derives a channel's five playlist ids. The bool is false when the
// input is not a channel id, rather than returning ids built from a string that
// was never a channel.
func PlaylistsFor(channelID string) (ChannelPlaylists, bool) {
	if !channelRe.MatchString(strings.TrimSpace(channelID)) {
		return ChannelPlaylists{}, false
	}
	return playlistsFor(strings.TrimSpace(channelID)), true
}

func playlistsFor(channelID string) ChannelPlaylists {
	suffix := channelID[2:]
	return ChannelPlaylists{
		Uploads: "UU" + suffix,
		Videos:  "UULF" + suffix,
		Shorts:  "UUSH" + suffix,
		Streams: "UULV" + suffix,
		Popular: "UULP" + suffix,
		Browse:  "VLUU" + suffix,
		Feed:    "https://www.youtube.com/feeds/videos.xml?channel_id=" + channelID,
	}
}

// ChannelFor returns the channel a UU-family playlist id belongs to. All five
// strip back to the same channel, so a UU id found in a payload names a channel
// nobody fetched.
func ChannelFor(playlistID string) (string, bool) {
	s := strings.TrimPrefix(strings.TrimSpace(playlistID), "VL")
	for _, kp := range kindPrefixes {
		suffix := strings.TrimPrefix(s, kp.prefix)
		if suffix != s && suffixRe.MatchString(suffix) {
			return "UC" + suffix, true
		}
	}
	return "", false
}

// ParentOf returns the comment a reply belongs to.
func ParentOf(replyID string) (string, bool) {
	parent, _, ok := strings.Cut(strings.TrimSpace(replyID), ".")
	if !ok || !commentRe.MatchString(parent) {
		return "", false
	}
	return parent, true
}

// WireID is the browseId form of a playlist id: VL in front. A bare playlist id
// sent as a browseId is a 400 with a 285-byte body that names no field, so this
// exists to be called at exactly one place, the request builder.
func WireID(playlistID string) string {
	s := strings.TrimSpace(playlistID)
	if s == "" || strings.HasPrefix(s, "VL") {
		return s
	}
	return "VL" + s
}

// StripWire removes a browse prefix. Everything that stores or prints an id runs
// it through here, because a prefix is routing and not identity.
//
// VL is what www browses a playlist with. MPSP is the same idea on music, where
// a podcast show browses as MPSP in front of the PL id its episodes live under,
// and the show and the playlist are the same thing under two addresses.
func StripWire(id string) string {
	s := strings.TrimSpace(id)
	for _, prefix := range []string{"VL", "MPSP"} {
		if rest := strings.TrimPrefix(s, prefix); rest != s && rest != "" {
			return rest
		}
	}
	return s
}

// IsChannel, IsVideo and IsPlaylist are the three shape questions asked often
// enough to deserve names.
func IsChannel(s string) bool { return channelRe.MatchString(strings.TrimSpace(s)) }
func IsVideo(s string) bool   { return videoRe.MatchString(strings.TrimSpace(s)) }

// IsPlaylist covers every playlist shape, derived or made by a person, which is
// what a caller deciding whether to browse with VL needs to know.
func IsPlaylist(s string) bool {
	switch Classify(s).Kind {
	case Playlist, Uploads, Videos, Shorts, Streams, Popular, Album, Mix:
		return true
	}
	return false
}

// fromPath reads the reference forms that are paths rather than ids.
func fromPath(input, s string) Info {
	trimmed := strings.Trim(s, "/")
	head, rest, _ := strings.Cut(trimmed, "/")
	info := Info{Input: input, ID: trimmed}
	switch head {
	case "user":
		if rest == "" {
			return Info{Input: input}
		}
		info.Kind = LegacyUser
		info.ID = "/user/" + rest
		info.NeedsRequest = true
		info.Note = "a pre-2013 user name, resolved through navigation/resolve_url like a handle"
		return info
	case "c":
		if rest == "" {
			return Info{Input: input}
		}
		info.Kind = LegacyCustom
		info.ID = "/c/" + rest
		info.NeedsRequest = true
		info.Note = "a pre-handles vanity name, resolved through navigation/resolve_url like a handle"
		return info
	case "channel", "playlist", "shorts", "live", "embed", "watch", "hashtag":
		return fromURL(input, "https://www.youtube.com/"+trimmed)
	}
	if strings.HasPrefix(head, "@") {
		return Classify(head)
	}
	return Info{Input: input}
}

// fromURL reduces a URL to the id it carries and classifies that. A watch link
// carries a video and often a playlist, and the video wins, because that is what
// a person means by the link.
func fromURL(input, s string) Info {
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return Info{Input: input}
	}
	if v := u.Query().Get("v"); v != "" {
		return withInput(input, Classify(v))
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if strings.Contains(u.Host, "youtu.be") && parts[0] != "" {
		return withInput(input, Classify(parts[0]))
	}
	if len(parts) >= 2 {
		switch parts[0] {
		case "shorts", "live", "embed", "v":
			return withInput(input, Classify(parts[1]))
		case "channel":
			return withInput(input, Classify(parts[1]))
		// music.youtube.com writes an album, an artist and a feed the same way,
		// /browse/<browse id>, so whatever is under it is classified as an id.
		case "browse":
			return withInput(input, Classify(parts[1]))
		case "user":
			return withInput(input, fromPath(input, "/user/"+parts[1]))
		case "c":
			return withInput(input, fromPath(input, "/c/"+parts[1]))
		case "hashtag":
			tag, err := url.PathUnescape(parts[1])
			if err != nil {
				tag = parts[1]
			}
			return Info{Input: input, Kind: Hashtag, ID: strings.ToLower(strings.TrimPrefix(tag, "#"))}
		}
	}
	if list := u.Query().Get("list"); list != "" {
		return withInput(input, Classify(list))
	}
	if parts[0] != "" && strings.HasPrefix(parts[0], "@") {
		return withInput(input, Classify(parts[0]))
	}
	return Info{Input: input}
}

func withInput(input string, info Info) Info {
	info.Input = input
	return info
}

func joinNotes(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	default:
		return a + "; " + b
	}
}
