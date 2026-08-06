package youtube

import (
	"encoding/json"
	"strings"

	"github.com/tamnd/ytb-cli/pkg/graph"
	"github.com/tamnd/ytb-cli/pkg/ytid"
)

// edges.go turns records into claims. Spec 3005 doc 04 section 3.
//
// One function per record kind, each writing into a caller's set and returning
// how many claims it accepted. A set rather than a return slice because a real
// read is a video and its comments and its related shelf, and three sets a
// caller has to merge is three chances to lose the provenance on the way.
//
// Nothing here fetches. A claim is what a record already says, and the whole
// argument for the plane is that a record says far more than it was read for:
// one watch page names a channel, twenty related videos, a handful of links and
// a hashtag, and every one of those is a node the tool has heard of and never
// looked at.
//
// graph.Edge and the Edge in discover.go are different things with one name in
// two packages. This one is a claim that gets stored; that one is a traversal
// step that says which way a walk may go.

// Prov is the provenance every claim off this record carries.
//
// Sources[0] is the URL the read started at, which is the record's own address
// and the right answer for nearly every claim on it. A record merged from three
// surfaces has three sources and no per-field map from a field to the URL that
// filled it, so ProvFor uses via to at least get the surface right rather than
// inventing a URL that would look authoritative.
func (e Envelope) Prov() graph.Provenance {
	p := graph.Provenance{Tier: e.Tier}
	if len(e.Sources) > 0 {
		p.Source = e.Sources[0]
	}
	if len(e.Surfaces) > 0 {
		p.Surface = e.Surfaces[0]
	}
	if len(e.Client) > 0 {
		p.Client = e.Client[0]
	}
	return p
}

// ProvFor is Prov with the surface via names for one field, where it names one.
func (e Envelope) ProvFor(field string) graph.Provenance {
	p := e.Prov()
	if s := viaSurface(e.Via[field]); s != "" {
		p.Surface = s
	}
	return p
}

// viaSurface reads the surface id off the front of a via string. A via value is
// a surface id and then prose, "s2 lockup metadata, rounded as the site
// rendered it", and only the id is machine readable.
func viaSurface(via string) string {
	id, _, _ := strings.Cut(strings.TrimSpace(via), " ")
	if len(id) < 2 || id[0] != 's' {
		return ""
	}
	for _, r := range id[1:] {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return id
}

// derived is the provenance of a claim the id space asserts on its own. It
// keeps the tier and drops the surface, the source and the client, because
// there was no request and crediting one would be a lie a query could not see
// through.
func derived(p graph.Provenance) graph.Provenance {
	return graph.Provenance{Source: p.Source, Surface: graph.SurfaceDerived, Tier: p.Tier}
}

// VideoClaims writes what one Video record says. Doc 04 section 3.2.
func VideoClaims(set *graph.Set, v Video) int {
	if v.VideoID == "" {
		return 0
	}
	me := graph.VideoURI(v.VideoID)
	n := 0
	add := func(ok bool) {
		if ok {
			n++
		}
	}

	if ch := graph.ChannelURI(v.ChannelID); ch != "" {
		add(set.Claim(ch, graph.Published, me, v.ProvFor("channel_id"), noteFor(v.Title, v.ChannelTitle)))
		// The uploads playlist is the channel id with the prefix swapped, so a
		// video read is also a claim about a playlist nobody has fetched.
		if pl, ok := ytid.PlaylistsFor(v.ChannelID); ok {
			add(set.Claim(graph.PlaylistURI(pl.Uploads), graph.UploadsOf, ch, derived(v.Prov()), v.ChannelTitle))
		}
	}

	prov := v.ProvFor("description")
	for _, id := range v.Mentions {
		add(set.Claim(me, graph.Mentions, graph.ChannelURI(id), prov, ""))
	}
	for _, l := range v.Links {
		add(set.Claim(me, graph.LinksTo, graph.ExternalURI(l.URL), prov, linkNote(l.Text, l.URL)))
	}
	for _, tag := range v.Hashtags {
		add(set.Claim(me, graph.Tagged, graph.HashtagURI(tag), prov, ""))
	}

	capProv := v.ProvFor("caption_tracks")
	for _, t := range v.CaptionTracks {
		add(set.Claim(me, graph.HasCaptions, graph.CaptionURI(v.VideoID, captionPart(t)), capProv, t.Name))
	}

	chapProv := v.ProvFor("chapters")
	for _, c := range v.Chapters {
		add(set.ClaimAt(graph.ChapterURI(v.VideoID, c.StartSeconds), graph.ChapterOf, me, chapProv, c.Title, c.Position))
	}
	return n
}

// captionPart names a caption track inside its video.
//
// The language code alone is not unique: a video with a human English track and
// an auto-generated one has two tracks both saying en. The vss id is what tells
// them apart and it survives a rename, so it is used where there is one, with
// its leading dot off so the ordinary case still reads yt://video/<id>#captions/en.
func captionPart(t CaptionTrack) string {
	if t.VssID != "" {
		return strings.TrimPrefix(t.VssID, ".")
	}
	return t.LanguageCode
}

// linkNote is what the note column shows for an external link. The display text
// where there is one, the URL where there is not, because a row reading
// yt://external/8f3c... and nothing else tells nobody anything.
func linkNote(text, url string) string {
	if text != "" {
		return text
	}
	return graph.NormaliseExternal(url)
}

// noteFor picks the label a claim carries: the name of the end that is new,
// falling back to the other end's name where the first has none.
//
// A published claim off a lockup is the case that decided this. The row names a
// channel by id and a video by id, and the shelf gave the video a title and the
// channel nothing, so labelling it with the channel would leave the commonest
// claim in the store with an empty note.
func noteFor(preferred, fallback string) string {
	if strings.TrimSpace(preferred) != "" {
		return preferred
	}
	return fallback
}

// excerpt is a note for a claim about a thing with no title, which is what a
// comment and a community post are. One line and short, because the note column
// sits next to two URIs and a paragraph of comment would push them off the
// screen.
func excerpt(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\r", " "), "\n", " "))
	s = strings.Join(strings.Fields(s), " ")
	const max = 60
	if len([]rune(s)) <= max {
		return s
	}
	return string([]rune(s)[:max-1]) + "…"
}

// ChannelClaims writes what one Channel record says.
func ChannelClaims(set *graph.Set, c Channel) int {
	me := graph.ChannelURI(c.ChannelID)
	if me == "" {
		return 0
	}
	n := 0
	add := func(ok bool) {
		if ok {
			n++
		}
	}

	// All five derived playlists, not just uploads. They cost nothing, they are
	// real nodes with their own titles and counts, and they are the cheapest
	// entries the crawler's frontier ever gets.
	if pl, ok := ytid.PlaylistsFor(c.ChannelID); ok {
		for _, id := range []string{pl.Uploads, pl.Videos, pl.Shorts, pl.Streams, pl.Popular} {
			add(set.Claim(graph.PlaylistURI(id), graph.UploadsOf, me, derived(c.Prov()), c.Title))
		}
	}

	prov := c.ProvFor("links")
	for _, l := range c.Links {
		add(set.Claim(me, graph.LinksTo, graph.ExternalURI(l.URL), prov, linkNote(l.Title, l.URL)))
	}
	return n
}

// PlaylistClaims writes what one Playlist record says.
func PlaylistClaims(set *graph.Set, p Playlist) int {
	me := graph.PlaylistURI(p.PlaylistID)
	if p.PlaylistID == "" {
		return 0
	}
	n := 0
	add := func(ok bool) {
		if ok {
			n++
		}
	}
	if ch := graph.ChannelURI(p.ChannelID); ch != "" {
		add(set.Claim(ch, graph.Owns, me, p.ProvFor("channel_id"), noteFor(p.Title, p.ChannelTitle)))
	}
	// A UU-family id names its channel whether or not the header did, and it names
	// it exactly, where a byline gives a title that two channels can share.
	if id, ok := ytid.ChannelFor(p.PlaylistID); ok {
		add(set.Claim(me, graph.UploadsOf, graph.ChannelURI(id), derived(p.Prov()), p.ChannelTitle))
	}
	return n
}

// PlaylistItemClaims writes the membership of an ordered playlist read.
//
// Three claims per item, and only one of them is about the playlist: the item
// is also a video with a byline on it, so a playlist read is a pile of
// published claims about channels nobody fetched. The in_playlist_after chain
// is what keeps the order recoverable from the claims alone.
func PlaylistItemClaims(set *graph.Set, playlistID string, items []Video, prov graph.Provenance) int {
	pl := graph.PlaylistURI(playlistID)
	if playlistID == "" {
		return 0
	}
	n := 0
	add := func(ok bool) {
		if ok {
			n++
		}
	}
	var prev graph.URI
	var prevTitle string
	for i, v := range items {
		me := graph.VideoURI(v.VideoID)
		if v.VideoID == "" {
			continue
		}
		pos := v.Position
		if pos == 0 {
			pos = i + 1
		}
		add(set.ClaimAt(pl, graph.Contains, me, prov, v.Title, pos))
		if ch := graph.ChannelURI(v.ChannelID); ch != "" {
			add(set.Claim(ch, graph.Published, me, prov, noteFor(v.Title, v.ChannelTitle)))
		}
		if prev != "" && prev != me {
			add(set.ClaimAt(me, graph.InPlaylistAfter, prev, prov, prevTitle, pos))
		}
		prev, prevTitle = me, v.Title
	}
	return n
}

// RelatedClaims writes the related shelf of one watch page.
func RelatedClaims(set *graph.Set, videoID string, related []Video, prov graph.Provenance) int {
	me := graph.VideoURI(videoID)
	if videoID == "" {
		return 0
	}
	n := 0
	for i, v := range related {
		if v.VideoID == "" || v.VideoID == videoID {
			continue
		}
		if set.ClaimAt(me, graph.RelatedTo, graph.VideoURI(v.VideoID), prov, v.Title, i+1) {
			n++
		}
		// A related row carries its own byline, so the shelf is twenty published
		// claims as well as twenty related ones.
		if ch := graph.ChannelURI(v.ChannelID); ch != "" {
			if set.Claim(ch, graph.Published, graph.VideoURI(v.VideoID), prov, noteFor(v.Title, v.ChannelTitle)) {
				n++
			}
		}
	}
	return n
}

// CommentClaims writes a comment thread.
//
// replies_to comes off the dot in a reply id and costs no request, so a page of
// replies states its own tree even where the parent was never read.
func CommentClaims(set *graph.Set, comments []Comment, prov graph.Provenance) int {
	n := 0
	add := func(ok bool) {
		if ok {
			n++
		}
	}
	for _, c := range comments {
		if c.ID == "" {
			continue
		}
		me := graph.CommentURI(c.ID)
		if c.VideoID != "" {
			add(set.Claim(me, graph.CommentsOn, graph.VideoURI(c.VideoID), prov, excerpt(c.TextDisplay)))
		}
		if ch := graph.ChannelURI(c.AuthorChannelID); ch != "" {
			add(set.Claim(ch, graph.Commented, me, prov, noteFor(excerpt(c.TextDisplay), c.AuthorDisplayName)))
		}
		parent := c.ParentID
		if parent == "" {
			// The id says so on its own where the record did not carry it.
			parent, _ = ytid.ParentOf(c.ID)
		}
		if parent != "" && parent != c.ID {
			add(set.Claim(me, graph.RepliesTo, graph.CommentURI(parent), derived(prov), ""))
		}
	}
	return n
}

// PostAttachment is one attachment on a community post. It is the shape
// CommunityPost.Attachments holds, named here so the parser that writes it and
// the parser that reads it cannot drift apart.
type PostAttachment struct {
	// Type is image, poll, video, playlist or post.
	Type string `json:"type"`
	URL  string `json:"url,omitempty"`
	ID   string `json:"id,omitempty"`
}

// ParsePostAttachments reads the JSON array a CommunityPost carries.
func ParsePostAttachments(s string) []PostAttachment {
	s = strings.TrimSpace(s)
	if s == "" || s == "[]" {
		return nil
	}
	var out []PostAttachment
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}

// PostClaims writes what a community post says.
//
// A poll attachment produces nothing. It is a real attachment and it has no
// node: the options are text on the post, not things with ids, and a node per
// poll option would be a node nothing else ever names.
func PostClaims(set *graph.Set, posts []CommunityPost, prov graph.Provenance) int {
	n := 0
	add := func(ok bool) {
		if ok {
			n++
		}
	}
	for _, p := range posts {
		if p.PostID == "" {
			continue
		}
		me := graph.PostURI(p.PostID)
		if ch := graph.ChannelURI(p.ChannelID); ch != "" {
			add(set.Claim(ch, graph.Posted, me, prov, noteFor(excerpt(p.ContentText), p.AuthorName)))
		}
		for _, a := range ParsePostAttachments(p.Attachments) {
			switch a.Type {
			case "video":
				add(set.Claim(me, graph.Attaches, graph.VideoURI(a.ID), prov, "video"))
			case "playlist":
				add(set.Claim(me, graph.Attaches, graph.PlaylistURI(a.ID), prov, "playlist"))
			case "image":
				add(set.Claim(me, graph.Attaches, graph.ExternalURI(a.URL), prov, "image"))
			case "post":
				add(set.Claim(me, graph.Shares, graph.PostURI(a.ID), prov, "shared post"))
			}
		}
	}
	return n
}

// FeaturedClaims writes a channel's featured channels shelf.
func FeaturedClaims(set *graph.Set, channelID string, featured []Channel, prov graph.Provenance) int {
	me := graph.ChannelURI(channelID)
	if me == "" {
		return 0
	}
	n := 0
	for _, c := range featured {
		to := graph.ChannelURI(c.ChannelID)
		if to == "" || to == me {
			continue
		}
		if set.Claim(me, graph.Features, to, prov, c.Title) {
			n++
		}
	}
	return n
}

// AlbumClaims writes an album and its track list.
func AlbumClaims(set *graph.Set, a Album, tracks []Song, prov graph.Provenance) int {
	n := 0
	add := func(ok bool) {
		if ok {
			n++
		}
	}
	album := graph.AlbumURI(a.AlbumID)
	if a.AlbumID != "" {
		if artist := graph.ArtistURI(a.ArtistID); artist != "" {
			add(set.Claim(album, graph.ByArtist, artist, prov, a.ArtistName))
		}
	}
	for i, t := range tracks {
		n += songClaims(set, t, prov, i+1)
	}
	return n
}

// SongClaims writes one track.
func SongClaims(set *graph.Set, s Song, prov graph.Provenance) int {
	return songClaims(set, s, prov, 0)
}

func songClaims(set *graph.Set, s Song, prov graph.Provenance, position int) int {
	if s.VideoID == "" {
		return 0
	}
	me := graph.VideoURI(s.VideoID)
	n := 0
	add := func(ok bool) {
		if ok {
			n++
		}
	}
	if artist := graph.ArtistURI(s.ArtistID); artist != "" {
		add(set.Claim(me, graph.ByArtist, artist, prov, s.ArtistName))
	}
	if s.AlbumID != "" {
		add(set.ClaimAt(me, graph.InAlbum, graph.AlbumURI(s.AlbumID), prov, s.AlbumName, position))
	}
	return n
}

// SeenAsClaims joins the two ids one track can have.
//
// Nothing is written when the music app answers with the id it was asked for. A
// claim from a node to itself is not a claim, and the record already says which
// surfaces saw it. It is written when the ids differ, which is a song with both
// an official video and an art track, and that is the case the predicate is for.
func SeenAsClaims(set *graph.Set, watchID, musicID string, prov graph.Provenance, note string) int {
	if watchID == "" || musicID == "" || watchID == musicID {
		return 0
	}
	if set.Claim(graph.VideoURI(watchID), graph.SeenAs, graph.VideoURI(musicID), prov, note) {
		return 1
	}
	return 0
}
