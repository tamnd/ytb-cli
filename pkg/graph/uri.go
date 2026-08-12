// Package graph is the identity and claim layer. Spec 3005 doc 04.
//
// Everything ytb reads gets turned into two things: nodes, named by a URI that
// is stable across runs and across machines, and claims, which are edges with
// the observation that produced them attached. That is the whole plane. The
// records in the youtube package are what one read of one object looked like;
// the graph is what all the reads together say about how the objects connect.
//
// The value is in what a single read already knows. One watch page names a
// channel, twenty related videos, a handful of external links and a hashtag,
// and every one of those is a node the tool has heard of and never fetched. A
// store that holds only what was fetched cannot tell you what it has not looked
// at yet, which is the one question a crawler needs answered.
package graph

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"

	"github.com/tamnd/ytb-cli/pkg/ytid"
)

// Kind is what a node is. It is deliberately smaller than ytid.Kind: ytid
// classifies id shapes, and several distinct shapes are one kind of thing here.
// UU, UULF, UUSH, UULV, UULP, PL and OLAK5uy_ are seven shapes and one kind of
// node, because they are all playlists and they all behave like playlists.
type Kind string

const (
	// Video covers videos, shorts and past streams. Doc 04 section 2: the id space
	// is one space and the difference is a flag on the record, so two URI spaces
	// would mean a short that stops being a short moves house.
	Video    Kind = "video"
	Channel  Kind = "channel"
	Playlist Kind = "playlist"
	Comment  Kind = "comment"
	Post     Kind = "post"
	Hashtag  Kind = "hashtag"
	// Album and Artist are YouTube Music's own nodes. An artist with a UC id is
	// the same node as the channel; an artist with only an MPLA id is its own
	// node until something resolves it.
	Album  Kind = "album"
	Artist Kind = "artist"
	// External is any URL off the site. The URI is a hash of the URL, so two
	// videos linking the same tracking-laden address converge on one node without
	// the key being a URL with a query string in it.
	External Kind = "external"
)

// The fragment kinds. These name the parts of a video that have no id of their
// own, so they are always a fragment on a video URI and never a node on their
// own: a format without its video is an itag, which is a number.
const (
	FragFormat  = "format"
	FragCaption = "captions"
	FragChapter = "chapter"
	FragThumb   = "thumb"
)

// Scheme is the URI scheme. It is not http on purpose: a yt:// URI names the
// thing, not a page about the thing, and there are nodes here (a caption track,
// a chapter, a format) that no URL addresses.
const Scheme = "yt"

// URI is a node name. It is a string underneath so it can be a map key and a
// SQL primary key without ceremony.
type URI string

func (u URI) String() string { return string(u) }

// Node builds a URI from a kind and an id, with no validation beyond trimming.
// The typed constructors below are what callers should use; this is here for
// the parser and for kinds that are just an id.
func Node(kind Kind, id string) URI {
	return URI(fmt.Sprintf("%s://%s/%s", Scheme, kind, strings.TrimSpace(id)))
}

func VideoURI(id string) URI    { return Node(Video, id) }
func PlaylistURI(id string) URI { return Node(Playlist, ytid.StripWire(id)) }
func CommentURI(id string) URI  { return Node(Comment, id) }
func PostURI(id string) URI     { return Node(Post, id) }
func AlbumURI(id string) URI    { return Node(Album, id) }

// ChannelURI names a channel. It takes a channel id and nothing else.
//
// A handle is not an identity. It is case-insensitive, a channel can change it,
// and two spellings reach one channel, so yt://channel/@RickAstleyYT would
// become a second node for the same channel the day the handle changes and a
// third the day somebody writes it in a different case. Anything that is not a
// UC id returns the empty URI, and the caller resolves it first.
func ChannelURI(id string) URI {
	id = strings.TrimSpace(id)
	if !ytid.IsChannel(id) {
		return ""
	}
	return Node(Channel, id)
}

// ArtistURI names a music artist. A UC id means the artist and the channel are
// one node; an MPLA id is its own node until something resolves it.
func ArtistURI(id string) URI {
	id = strings.TrimSpace(id)
	if ytid.IsChannel(id) {
		return Node(Channel, id)
	}
	if id == "" {
		return ""
	}
	return Node(Artist, id)
}

// HashtagURI names a hashtag, lowercased with the # off. YouTube's hashtag
// pages are case-insensitive and #RickRoll and #rickroll are one feed, so two
// URIs would be two nodes for one thing.
func HashtagURI(tag string) URI {
	tag = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(tag), "#")))
	if tag == "" {
		return ""
	}
	return Node(Hashtag, tag)
}

// ExternalURI names an off-site URL by the sha256 of its normalised form.
//
// The URL itself is a property of the node rather than its key. A key with a
// query string in it is a key nobody can quote in a shell, and normalising the
// scheme and the trailing slash first means http://x.com/ and https://x.com are
// one node rather than two, which is what a person means by "the same link".
func ExternalURI(raw string) URI {
	n := NormaliseExternal(raw)
	if n == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(n))
	return Node(External, hex.EncodeToString(sum[:]))
}

// NormaliseExternal is the form ExternalURI hashes, exported because a node's
// record carries it and the two must agree.
func NormaliseExternal(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	// http and https are one address in every case that matters here, and
	// YouTube's own markup writes http on pages served over TLS.
	u.Scheme = "https"
	u.Host = strings.ToLower(u.Host)
	u.Host = strings.TrimPrefix(u.Host, "www.")
	if u.Path == "/" {
		u.Path = ""
	}
	u.Fragment = ""
	return u.String()
}

// Fragment attaches a part name to a node URI: a format, a caption track, a
// chapter or a thumbnail rendition.
func Fragment(node URI, part, id string) URI {
	if node == "" {
		return ""
	}
	return URI(fmt.Sprintf("%s#%s/%s", node, part, id))
}

func FormatURI(videoID string, itag int) URI {
	return Fragment(VideoURI(videoID), FragFormat, fmt.Sprint(itag))
}

func CaptionURI(videoID, lang string) URI {
	return Fragment(VideoURI(videoID), FragCaption, lang)
}

// ChapterURI names a chapter by its start second, which is the only thing about
// a chapter that YouTube guarantees is unique within a video. The title is not:
// a video with two chapters called "Chorus" is a normal video.
func ChapterURI(videoID string, startSeconds int) URI {
	return Fragment(VideoURI(videoID), FragChapter, fmt.Sprint(startSeconds))
}

func ThumbURI(videoID, rendition string) URI {
	return Fragment(VideoURI(videoID), FragThumb, rendition)
}

// Parsed is a URI taken apart.
type Parsed struct {
	Kind Kind
	ID   string
	// Part and PartID are the fragment, empty on a plain node.
	Part   string
	PartID string
}

// IsFragment reports whether the URI names a part of a node rather than a node.
func (p Parsed) IsFragment() bool { return p.Part != "" }

// Parse takes a yt:// URI apart. It is the inverse of the constructors and it
// is what the store and the RDF writer use to decide how to treat a URI they
// were handed.
func Parse(u URI) (Parsed, bool) {
	s := string(u)
	rest, ok := strings.CutPrefix(s, Scheme+"://")
	if !ok {
		return Parsed{}, false
	}
	kind, rest, ok := strings.Cut(rest, "/")
	if !ok || kind == "" || rest == "" {
		return Parsed{}, false
	}
	p := Parsed{Kind: Kind(kind), ID: rest}
	if id, frag, ok := strings.Cut(rest, "#"); ok {
		p.ID = id
		part, partID, ok := strings.Cut(frag, "/")
		if !ok {
			return Parsed{}, false
		}
		p.Part, p.PartID = part, partID
	}
	if p.ID == "" {
		return Parsed{}, false
	}
	return p, true
}

// FromID builds the URI an id names, using pkg/ytid to decide what it is.
//
// This is the one place that turns a bare string found in a payload into a
// node, and it is deliberately strict: the alias shapes return the empty URI
// with needsRequest true, because a handle names a channel only after
// navigation/resolve_url has answered and guessing would create a node that a
// rename turns into a duplicate.
func FromID(s string) (uri URI, needsRequest bool) {
	info := ytid.Classify(s)
	switch info.Kind {
	case ytid.Video:
		return VideoURI(info.ID), false
	case ytid.Channel:
		return ChannelURI(info.ID), false
	case ytid.Handle, ytid.LegacyUser, ytid.LegacyCustom:
		return "", true
	case ytid.Playlist, ytid.Uploads, ytid.Videos, ytid.Shorts,
		ytid.Streams, ytid.Popular, ytid.Album:
		return PlaylistURI(info.ID), false
	case ytid.Comment, ytid.Reply:
		return CommentURI(info.ID), false
	case ytid.Hashtag:
		return HashtagURI(info.ID), false
	case ytid.MusicAlbum:
		return AlbumURI(info.ID), false
	case ytid.MusicArtist:
		return ArtistURI(info.ID), false
	default:
		// A mix, a feed, an itag on its own, or nothing recognised. None of those
		// are nodes: a mix is generated per viewer, a feed is a destination rather
		// than an entity, and an itag means nothing without a video.
		return "", false
	}
}

// WatchURL and the rest give the address a node is read at, for the source
// column and for anything that wants to open it. A URI is not a URL and these
// keep it that way: the conversion is explicit and one-directional.
func (u URI) URL() string {
	p, ok := Parse(u)
	if !ok {
		return ""
	}
	switch p.Kind {
	case Video:
		return "https://www.youtube.com/watch?v=" + p.ID
	case Channel:
		return "https://www.youtube.com/channel/" + p.ID
	case Playlist:
		return "https://www.youtube.com/playlist?list=" + p.ID
	case Hashtag:
		return "https://www.youtube.com/hashtag/" + p.ID
	case Post:
		return "https://www.youtube.com/post/" + p.ID
	case Album, Artist:
		return "https://music.youtube.com/browse/" + p.ID
	default:
		// A comment has no address of its own, and an external node's address is a
		// property rather than something derivable from its hash.
		return ""
	}
}
