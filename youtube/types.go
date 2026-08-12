package youtube

import (
	"fmt"
	"strings"
	"time"
)

// Video is one YouTube video: the fullest record in the model and the one whose
// shape varies most by which surface answered. Doc 03 section 2.
//
// A Video from `ytb video` is complete. A Video from `ytb uploads` or `ytb search`
// is a listing row, and the difference is stated rather than hidden: the envelope's
// surfaces name the reads that contributed, a field the listing had no answer for
// is absent from the JSON instead of zero, and missed says in a sentence what a
// full read would add.
//
// Every optional field is omitempty or omitzero, and the flags are *bool. Absent
// and false are different claims: is_family_safe missing means nobody asked,
// is_family_safe false means YouTube answered no. Writing false for both is the
// single defect this record model exists to remove, because a dataset cannot tell
// them apart afterwards.
//
// The kit tags make the record addressable by a host (id, body, links) and the
// table tags pick the columns the aligned table shows. Everything else is JSON only.
type Video struct {
	// --- identity, doc 03 section 2.1 ---

	VideoID string `json:"id" kit:"id" table:"id"`
	// URL is the watch URL, and ShortURL the youtu.be form. Both are derived.
	URL      string `json:"url,omitempty" table:"url,url"`
	ShortURL string `json:"short_url,omitempty" table:"-"`
	// EmbedURL and CanonicalURL come off microformat rather than being built, so
	// canonical_url is /shorts/<id> on a short and /watch?v= on everything else,
	// which is YouTube stating what kind of thing this is.
	EmbedURL     string `json:"embed_url,omitempty" table:"-"`
	CanonicalURL string `json:"canonical_url,omitempty" table:"-"`

	// --- content, doc 03 section 2.2 ---

	Title string `json:"title,omitempty" table:"title,truncate"`
	// Description is videoDetails.shortDescription: plain text, authoritative,
	// already unescaped.
	Description string `json:"description,omitempty" kit:"body" table:"-"`
	// DescriptionRuns is the same description with its endpoints intact, which is
	// where Links, Mentions and Hashtags come from. See runs.go.
	DescriptionRuns []TextRun `json:"description_runs,omitempty" table:"-"`
	// Keywords is the uploader's own tag list, absent on most videos.
	Keywords []string `json:"keywords,omitempty" table:"-"`
	// Category is English whatever language the page was fetched in.
	Category string   `json:"category,omitempty" table:"-"`
	Hashtags []string `json:"hashtags,omitempty" table:"-"`
	// Mentions are channel ids the description linked to.
	Mentions []string `json:"mentions,omitempty" table:"-"`
	Links    []Link   `json:"links,omitempty" table:"-"`

	// --- counts, doc 03 section 2.3 ---

	// ViewCount is exact and ViewCountText is what a lockup rendered, e.g.
	// "1.8B views". Both are kept: rounding is lossy and irreversible.
	ViewCount     int64  `json:"view_count,omitempty" table:"views"`
	ViewCountText string `json:"view_count_text,omitempty" table:"-"`
	LikeCount     int64  `json:"like_count,omitempty" table:"-"`
	// CommentCount is absent rather than zero when the comment section was not
	// read, and missed says so.
	CommentCount int64 `json:"comment_count,omitempty" table:"-"`
	// DislikeCount only ever comes from a third party and is an estimate. Via says
	// where it came from.
	DislikeCount int64 `json:"dislike_count,omitempty" table:"-"`

	// --- time, doc 03 section 2.4 ---

	// DurationSeconds comes from microformat, which agrees with the page's own
	// microdata where videoDetails is a second out. Via records which block won.
	DurationSeconds int    `json:"duration_seconds,omitempty" table:"-"`
	DurationText    string `json:"duration_text,omitempty" table:"duration"`
	// PublishedAt is exact to the second with the uploader's UTC offset.
	// PublishedText is the rendered "1 month ago", and it is never parsed into
	// PublishedAt because "1 month ago" is not a timestamp.
	PublishedText string    `json:"published_text,omitempty" table:"published"`
	PublishedAt   time.Time `json:"published_at,omitzero" table:"-"`
	// UpdatedAt only ever comes from the Atom feed.
	UpdatedAt time.Time `json:"updated_at,omitzero" table:"-"`
	// UploadDate differs from PublishedAt on a premiere, which is why it is a field
	// of its own rather than the same value twice.
	UploadDate time.Time `json:"upload_date,omitzero" table:"-"`

	// --- channel, doc 03 section 2.5 ---

	// ChannelID is mandatory on a Video from every surface.
	ChannelID     string `json:"channel_id,omitempty" kit:"link,kind=youtube/channel" table:"-"`
	ChannelTitle  string `json:"channel_title,omitempty" table:"channel,truncate"`
	ChannelHandle string `json:"channel_handle,omitempty" table:"-"`
	ChannelURL    string `json:"channel_url,omitempty" table:"-"`

	// --- flags and availability, doc 03 section 2.6 ---

	// IsShort means this is a short, not that it is eligible to be one. Those are
	// different fields on the payload and only one of them is an answer.
	IsShort *bool `json:"is_short,omitempty" table:"-"`
	// IsLiveContent is true for a past stream too, so it is not "is live now".
	// LiveState is the badge, and it is the field that says now: live, upcoming,
	// ended, or absent on a normal upload.
	IsLiveContent *bool  `json:"is_live_content,omitempty" table:"-"`
	LiveState     string `json:"live_state,omitempty" table:"-"`
	IsPrivate     *bool  `json:"is_private,omitempty" table:"-"`
	IsUnlisted    *bool  `json:"is_unlisted,omitempty" table:"-"`
	IsFamilySafe  *bool  `json:"is_family_safe,omitempty" table:"-"`
	AgeRestricted *bool  `json:"age_restricted,omitempty" table:"-"`
	AllowRatings  *bool  `json:"allow_ratings,omitempty" table:"-"`
	IsCrawlable   *bool  `json:"is_crawlable,omitempty" table:"-"`
	// AvailableCountries is 249 ISO codes on an unrestricted video and is kept
	// whole. It is the only tier 0 answer to "is this blocked where I am", and a
	// count instead of the list would destroy that.
	AvailableCountries []string `json:"available_countries,omitempty" table:"-"`
	// Playability is playabilityStatus verbatim: the status, and YouTube's own
	// sentence when the answer is no.
	Playability *Playability `json:"playability,omitempty" table:"-"`
	// LocationDescription is microformat's own field, set on the videos that
	// carry a place.
	LocationDescription string `json:"location_description,omitempty" table:"-"`

	// --- media and text pointers, doc 03 section 2.7 ---

	// Thumbnails are the ones the payload named plus any constructed rendition a
	// HEAD confirmed. Each one says which it was.
	Thumbnails []Thumbnail `json:"thumbnails,omitempty" table:"-"`
	// ThumbnailURL is the largest thumbnail this read saw, kept because a row in a
	// table, a Markdown export and a SQL column all want one URL rather than a list.
	ThumbnailURL string `json:"thumbnail_url,omitempty" table:"-"`
	// Formats is the stream list, and it is absent unless --formats asked for it,
	// because it costs an s3 request per video and a crawl of a thousand videos
	// should not make a thousand extra calls nobody asked for.
	Formats []VideoFormat `json:"formats,omitempty" table:"-"`
	// CaptionTracks is name and language only. The text is a separate read.
	CaptionTracks []CaptionTrack `json:"caption_tracks,omitempty" table:"-"`
	Chapters      []Chapter      `json:"chapters,omitempty" table:"-"`
	// Player is the base.js URL from ytcfg, kept for provenance: it names the
	// player build that served this response.
	Player string `json:"player,omitempty" table:"-"`
	// Transcript and TranscriptLanguage are filled only by --transcript. The
	// transcript is its own record kind and this is the attached copy.
	Transcript         string `json:"transcript,omitempty" table:"-"`
	TranscriptLanguage string `json:"transcript_language,omitempty" table:"-"`

	// Microdata is what the page's own schema.org markup says about this video,
	// filled only by --microdata. It is a second opinion and never a replacement:
	// the markup is generated for crawlers and rounds where the payload does not.
	Microdata *Microdata `json:"microdata,omitempty" table:"-"`

	// --- listing rows, doc 02 section 4.2 ---

	// Badges are the thumbnail overlay badges a listing row carries, verbatim:
	// LIVE, New, 4K, CC, a members-only marker. The duration badge is not in here,
	// it is DurationText.
	Badges []string `json:"badges,omitempty" table:"-"`
	// MetadataParts is the catch-all. A lockup's metadata is a list of rendered
	// fragments with no field names on them, and every fragment this read could not
	// classify is kept here verbatim rather than dropped. A new fragment shows up in
	// the output as an unclassified string instead of vanishing.
	MetadataParts []string `json:"metadata_parts,omitempty" table:"-"`

	// --- playlist membership, doc 03 section 4 ---

	// Position is where this video sits in the playlist that was read, one based,
	// and is only set by ytb items. The same video can appear twice in one playlist
	// and position is the only thing telling the two apart.
	Position int `json:"position,omitempty" table:"pos"`
	// SetVideoID is the per-membership id YouTube needs to remove or reorder an
	// entry. The old playlistVideoRenderer carried it and the lockupViewModel that
	// replaced it does not, so at tier 0 it is now always empty and missed says so.
	SetVideoID string `json:"set_video_id,omitempty" table:"-"`

	Envelope
}

// Playability is playabilityStatus, verbatim. It is a struct rather than three
// fields on Video because the three only mean anything together: a Reason with no
// Status is a sentence with nothing to attach it to.
type Playability struct {
	Status string `json:"status,omitempty"`
	// Reason is YouTube's own sentence, in the language of the read.
	Reason string `json:"reason,omitempty"`
	// ReasonDetail is the second line the error screen adds, e.g. the sign-in
	// prompt under an age gate.
	ReasonDetail    string `json:"reason_detail,omitempty"`
	PlayableInEmbed *bool  `json:"playable_in_embed,omitempty"`
}

// Channel is one YouTube channel. Doc 03 section 3.
//
// Three blocks of the channel page answer, and they answer different questions.
// channelMetadataRenderer is the machine record: the id, the keywords, the rss
// url, the country codes the channel is available in. microformatDataRenderer is
// what the page tells crawlers, and it carries the ProfilePage that lists every
// external link. pageHeaderViewModel is what a person sees: the handle, the two
// rounded counts and the badge next to the title. The about panel is a fourth
// read, one continuation, and it is the only source of the join date, the
// lifetime view count and the country in words.
//
// Like Video, every optional field is omitempty or omitzero, because absent and
// zero are different claims. A channel with no banner has no banner key on its
// header, measured on @RickAstleyYT, and printing "banner_url": "" for it would
// be this record inventing an answer.
type Channel struct {
	// --- identity ---

	ChannelID string `json:"id" kit:"id" table:"id"`
	// Handle is the @name, and HandleURL the address it is reached at. YouTube
	// spells vanityChannelUrl with an http scheme, which is normalised here.
	Handle    string `json:"handle,omitempty" table:"handle"`
	HandleURL string `json:"handle_url,omitempty" table:"-"`
	// URL is the address that was read and CanonicalURL the /channel/UC form
	// YouTube itself states, which is not always the one a caller typed.
	URL          string `json:"url,omitempty" table:"url,url"`
	CanonicalURL string `json:"canonical_url,omitempty" table:"-"`

	// --- content ---

	Title string `json:"title,omitempty" table:"title,truncate"`
	// Description is the channel's own words.
	Description string `json:"description,omitempty" kit:"body" table:"-"`
	// ArtistBio is a third party biography that only a music channel has. It is not
	// the description and is never merged into it.
	ArtistBio string `json:"artist_bio,omitempty" table:"-"`
	// Keywords is the owner's keyword list, which the page carries as one
	// space separated string with quoted phrases in it.
	Keywords []string `json:"keywords,omitempty" table:"-"`
	// Links are the channel's external links. The about panel gives all of them
	// with their titles; without it the ld+json still gives every destination.
	Links []ChannelLink `json:"links,omitempty" table:"-"`

	// --- counts, doc 02 section 6 ---

	// SubscriberCount is rounded to three significant figures by YouTube itself,
	// which is what SubscriberCountIsApproximate says. It is written even though it
	// is always true, because a consumer reading 4520000 has no other way to know
	// it is not a count.
	SubscriberCount              int64  `json:"subscriber_count,omitempty" table:"subscribers"`
	SubscriberCountText          string `json:"subscriber_count_text,omitempty" table:"-"`
	SubscriberCountIsApproximate bool   `json:"subscriber_count_is_approximate" table:"-"`
	// VideoCount and VideoCountText are the header's, and the two sources disagree:
	// the header said 433 videos on a channel whose uploads playlist holds 435. Via
	// says which one answered.
	VideoCount     int64  `json:"video_count,omitempty" table:"videos"`
	VideoCountText string `json:"video_count_text,omitempty" table:"-"`
	// ViewCount is the channel's lifetime views, exact, and only the about panel
	// carries it.
	ViewCount int64 `json:"view_count,omitempty" table:"-"`

	// --- time ---

	// JoinedAt is the join date parsed, JoinedText the panel's own sentence. The
	// panel says "Joined 12 Aug 2009" with no time in it, so JoinedAt is a date at
	// midnight UTC and not a moment.
	JoinedAt   time.Time `json:"joined_at,omitzero" table:"-"`
	JoinedText string    `json:"joined_text,omitempty" table:"-"`

	// --- images ---

	// Avatar and Banner are the rendition lists YouTube supplied, not one URL
	// picked here, because which size a consumer wants is not this tool's call.
	Avatar []Thumbnail `json:"avatar,omitempty" table:"-"`
	Banner []Thumbnail `json:"banner,omitempty" table:"-"`

	// --- structure ---

	// Tabs is the tab strip, so the tab list is data rather than a table compiled
	// into this tool. It is what lets a read of a missing tab exit 3 naming the
	// tabs the channel really has. Doc 01 section 2.1.
	Tabs []Tab `json:"tabs,omitempty" table:"-"`
	// UploadsPlaylistID is derived from the id, and Counts is the four derived
	// playlists actually read, which only --counts asks for.
	UploadsPlaylistID string         `json:"uploads_playlist_id,omitempty" table:"-"`
	Counts            *ChannelCounts `json:"counts,omitempty" table:"counts"`
	// RSSURL is the Atom feed, named by the site rather than built here.
	RSSURL string `json:"rss_url,omitempty" table:"-"`

	// --- flags ---

	// IsFamilySafe is a pointer for the same reason Video's flags are: false is
	// YouTube's answer and absent is nobody having asked.
	IsFamilySafe *bool `json:"is_family_safe,omitempty" table:"-"`
	// AvailableCountryCodes is where the channel can be watched, 249 entries on a
	// channel with no restriction at all.
	AvailableCountryCodes []string `json:"available_country_codes,omitempty" table:"-"`
	// IsVerified and IsArtist come off the badge beside the title: a verified
	// channel carries CHECK_CIRCLE_FILLED and an official artist channel carries
	// AUDIO_BADGE. They are plain bools because the header always answers.
	IsVerified bool `json:"is_verified" table:"-"`
	IsArtist   bool `json:"is_artist" table:"-"`

	// Country is where the channel says it is, in words, from the about panel.
	Country string `json:"country,omitempty" table:"-"`
	// FacebookProfileID is the channel's Facebook page name, which
	// channelMetadataRenderer states on some channels and not others: @BBCNews has
	// "bbcnews" and @RickAstleyYT has no such key. It is kept as the id and not
	// turned into a facebook.com url, because the page said an id and a url would
	// be this record constructing one.
	FacebookProfileID string `json:"facebook_profile_id,omitempty" table:"-"`

	Envelope
}

// ChannelCounts is the four derived playlists read for real, which --counts asks
// for at a cost of four requests. Doc 01 section 2.5.
//
// The three kinds partition the uploads playlist, so the sum is a check on the
// whole derivation: measured on @RickAstleyYT, 139 videos + 294 shorts + 2
// streams = 435 uploads. When that stops agreeing something about the derived ids
// changed, which is worth finding out from a printed line rather than from a
// dataset that quietly lost a third of a channel.
type ChannelCounts struct {
	Uploads int `json:"uploads"`
	Videos  int `json:"videos"`
	Shorts  int `json:"shorts"`
	Streams int `json:"streams"`
	// The playlist ids read, so the numbers can be checked by hand.
	UploadsID string `json:"uploads_id"`
	VideosID  string `json:"videos_id"`
	ShortsID  string `json:"shorts_id"`
	StreamsID string `json:"streams_id"`
	// Agrees is whether videos + shorts + streams came to uploads.
	Agrees bool `json:"agrees"`
}

// String is the one line the table cell shows, and it states the arithmetic
// rather than the verdict alone, because "agrees" with no numbers behind it is
// not something a reader can check.
func (c ChannelCounts) String() string {
	verdict := "does not agree"
	if c.Agrees {
		verdict = "agrees"
	}
	return fmt.Sprintf("%d + %d + %d = %d uploads, %s", c.Videos, c.Shorts, c.Streams, c.Uploads, verdict)
}

// Comment is one comment or reply. Replies carry the parent comment id in ParentID.
type Comment struct {
	ID                 string `json:"id" kit:"id" table:"id"`
	VideoID            string `json:"video_id" kit:"link,kind=youtube/video" table:"-"`
	ParentID           string `json:"parent_id" table:"-"`
	AuthorChannelID    string `json:"author_channel_id" kit:"link,kind=youtube/channel" table:"-"`
	AuthorDisplayName  string `json:"author_display_name" table:"author,truncate"`
	AuthorProfileImage string `json:"author_profile_image_url" table:"-"`
	TextDisplay        string `json:"text_display" kit:"body" table:"text,truncate"`
	LikeCount          int64  `json:"like_count" table:"likes"`
	ReplyCount         int    `json:"reply_count" table:"replies"`
	IsOwnerComment     bool   `json:"is_owner_comment" table:"-"`
	// PublishedText is what YouTube says, and it is all YouTube says: "3 years
	// ago", rounded, with no exact time behind it at any tier. There is no
	// published_at on this record on purpose. A zero timestamp in the json is a
	// claim about when the comment was written, and it would be a false one.
	PublishedText string `json:"published_text" table:"published"`

	Envelope
}

// Chapter is one chapter marker on a video.
//
// Origin is the field that stops two different things being reported as one. A
// macro marker is a chapter YouTube itself recognised, with a title and a preview
// frame, and it appears in the player's scrubber. A timestamped line in the
// description is a convention some uploaders follow and YouTube did not act on,
// and reading it produces a list that looks identical and is not the same claim. A
// consumer that wants only real chapters can filter on origin; one that wants
// everything gets everything and knows which is which.
type Chapter struct {
	VideoID string `json:"video_id"`
	Title   string `json:"title"`
	// StartSeconds is where the chapter begins. The first chapter starts at 0.
	StartSeconds int `json:"start_seconds"`
	// ThumbnailURL is the preview frame, which only a macro marker has.
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	Position     int    `json:"position"`
	// Origin is ChapterFromMarkers or ChapterFromDescription.
	Origin string `json:"origin,omitempty"`
}

// Where a chapter came from.
const (
	// ChapterFromMarkers means YouTube served a macroMarkersListRenderer, so the
	// site itself treats these as chapters.
	ChapterFromMarkers = "markers"
	// ChapterFromDescription means the list was read off timestamp links in the
	// description, which YouTube linked but did not turn into chapters.
	ChapterFromDescription = "description"
)

// CommunityPost is one community/posts-tab post. Attachments is a JSON array.
type CommunityPost struct {
	PostID        string `json:"post_id" kit:"id" table:"id"`
	ChannelID     string `json:"channel_id" kit:"link,kind=youtube/channel" table:"-"`
	AuthorName    string `json:"author_name" table:"author,truncate"`
	AuthorAvatar  string `json:"author_avatar_url" table:"-"`
	ContentText   string `json:"content_text" kit:"body" table:"text,truncate"`
	LikeCount     int64  `json:"like_count" table:"likes"`
	ReplyCount    int    `json:"reply_count" table:"-"`
	VoteCount     string `json:"vote_count_text" table:"-"`
	PublishedText string `json:"published_text" table:"published"`
	Attachments   string `json:"attachments" table:"-"`

	Envelope
}

// VideoFormat is one streaming format (muxed or adaptive) of a video.
type VideoFormat struct {
	VideoID  string `json:"video_id"`
	ITag     int    `json:"itag"`
	MimeType string `json:"mime_type"`
	// Kind, Container and Codec are parsed out of MimeType at read time rather
	// than derived by whoever consumes the record, because -o json is the record
	// and a consumer should not have to re-implement the same three splits.
	// Container comes off the mime type and never off an itag table: the table is
	// folklore, the mime type is what the response said.
	Kind      string `json:"kind,omitempty"`
	Container string `json:"container,omitempty"`
	Codec     string `json:"codec,omitempty"`
	Quality   string `json:"quality"`
	// The video-only and audio-only fields are omitempty because an audio format
	// with "width": 0 and "fps": 0 on it is a record claiming to know things it
	// never read. Absent means this kind of format has no such field.
	QualityLabel string `json:"quality_label,omitempty"`
	Width        int    `json:"width,omitempty"`
	Height       int    `json:"height,omitempty"`
	FPS          int    `json:"fps,omitempty"`
	Bitrate      int64  `json:"bitrate"`
	// AverageBitrate is only on the adaptive formats. The muxed one carries a peak
	// bitrate and nothing else, measured on dQw4w9WgXcQ.
	AverageBitrate  int64 `json:"average_bitrate,omitempty"`
	AudioChannels   int   `json:"audio_channels,omitempty"`
	AudioSampleRate int   `json:"audio_sample_rate,omitempty"`
	// ContentLength is absent rather than zero when the response did not carry one,
	// which the ANDROID player does on every muxed format. Zero is a claim of an
	// empty file and Note says what absent means.
	ContentLength int64  `json:"content_length,omitempty"`
	IsAdaptive    bool   `json:"is_adaptive"`
	AudioQuality  string `json:"audio_quality,omitempty"`
	// ApproxDurationMS is the format's own duration, and it disagrees with the
	// video's by a few tens of milliseconds between formats: 213089 on itag 18
	// against 213040 on itag 313. It is per stream, not per video.
	ApproxDurationMS int64 `json:"approx_duration_ms,omitempty"`
	// InitRange and IndexRange are the two byte ranges a DASH player reads before
	// it reads any media, and they are absent on a muxed format.
	InitRange  *ByteRange `json:"init_range,omitempty"`
	IndexRange *ByteRange `json:"index_range,omitempty"`
	// LastModified is when this rendition was encoded, off lastModified, which the
	// response gives in microseconds and the URL repeats as lmt.
	LastModified time.Time `json:"last_modified,omitzero"`
	// URL is only ever set from a mobile player. The watch page lists the same
	// itags with no url and no signatureCipher, so there is nothing to fetch.
	// Doc 01 section 3.2.
	URL string `json:"url,omitempty"`
	// ExpiresAt is the deadline the CDN enforces, off the URL's own expire, not the
	// response's expiresInSeconds: those two disagree by a minute.
	ExpiresAt time.Time `json:"expires_at,omitzero"`
	// IsThrottledUnranged is true on every format, which is the point of it. It is
	// the one fact a consumer of ytb formats -o json most needs and cannot discover
	// from the payload: a plain GET of this URL runs at 32 KiB/s and the same URL
	// fetched in ranges runs at 4 MiB/s. Doc 01 section 8.
	IsThrottledUnranged bool `json:"is_throttled_unranged"`
	// Note is what is unusual about this one format, empty for a format with
	// nothing worth saying about it. The ANDROID player answers with no
	// contentLength on a muxed format, so its row says "size unknown" rather than
	// showing a 0 that reads as an empty file.
	Note string `json:"note,omitempty"`
}

// ByteRange is one of the DASH ranges, which the payload spells as strings.
type ByteRange struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

// Playlist is one playlist's header. Doc 03 section 4.
//
// The header is not one block. YouTube serves a playlist page under one of two
// shapes depending on how the playlist was made, and they share almost no key
// names, so playlistparse.go reads each by name and this record is what both of
// them fill in.
type Playlist struct {
	// --- identity ---

	// PlaylistID is the bare id with no VL prefix. The prefix is a browse
	// addressing detail and is added on the wire, not stored.
	PlaylistID string `json:"id" kit:"id" table:"id"`
	URL        string `json:"url,omitempty" table:"url,url"`

	// --- content ---

	Title       string `json:"title,omitempty" table:"title,truncate"`
	Description string `json:"description,omitempty" kit:"body" table:"-"`
	// ChannelID and ChannelTitle are the owner. An auto-generated mix has neither,
	// because nobody made it.
	ChannelID    string `json:"channel_id,omitempty" kit:"link,kind=youtube/channel" table:"-"`
	ChannelTitle string `json:"channel_title,omitempty" table:"channel,truncate"`

	// --- counts ---

	// VideoCount is the header's number and VideoCountText what it rendered. It is
	// not the number of items ytb items yields: a playlist holding deleted or
	// private videos states the full count here and serves fewer rows, and the
	// difference is what missed explains.
	VideoCount     int64  `json:"video_count,omitempty" table:"videos"`
	VideoCountText string `json:"video_count_text,omitempty" table:"-"`
	ViewCount      int64  `json:"view_count,omitempty" table:"-"`
	ViewCountText  string `json:"view_count_text,omitempty" table:"-"`

	// --- time ---

	// UpdatedText is "Updated 4 days ago", rendered and relative. Only one of the
	// two header shapes carries it, so its absence is a fact about the shape and
	// not about the playlist.
	UpdatedText string `json:"updated_text,omitempty" table:"-"`

	// --- structure ---

	// IsGenerated says YouTube built this playlist rather than a person. It is
	// derived from the id prefix, which is the only thing that states it.
	IsGenerated bool `json:"is_generated" table:"-"`
	// Visibility is public or unlisted where the header says so, and empty where it
	// does not, because guessing public is a claim this read cannot make.
	Visibility string      `json:"visibility,omitempty" table:"-"`
	Thumbnails []Thumbnail `json:"thumbnails,omitempty" table:"-"`
	// MetadataParts is the same catch-all a Video carries: a rendered fragment this
	// read did not recognise, kept verbatim rather than dropped.
	MetadataParts []string `json:"metadata_parts,omitempty" table:"-"`

	Envelope
}

// newPlaylist starts a playlist record. is_generated is set here because the id
// is the only thing that answers it and every construction path has the id.
func newPlaylist(id string, surfaces ...string) Playlist {
	return Playlist{
		PlaylistID:  id,
		URL:         NormalizePlaylistURL(id),
		IsGenerated: isGeneratedPlaylistID(id),
		Envelope:    newEnvelope("playlist", surfaces...),
	}
}

// isGeneratedPlaylistID reports whether YouTube built this playlist rather than a
// person. UU and its UUSH and UULV variants are a channel's derived uploads, OL
// is an album, RD is a mix or radio, and LL is the signed in user's likes. A PL
// id is somebody's playlist.
func isGeneratedPlaylistID(id string) bool {
	for _, prefix := range []string{"UU", "OL", "RD", "LL", "FL"} {
		if strings.HasPrefix(id, prefix) {
			return true
		}
	}
	return false
}

// PlaylistVideo is the playlist↔video membership join with position.
type PlaylistVideo struct {
	PlaylistID string `json:"playlist_id"`
	VideoID    string `json:"video_id"`
	Position   int    `json:"position"`
}

// RelatedVideo is the related-videos graph edge.
type RelatedVideo struct {
	VideoID        string `json:"video_id"`
	RelatedVideoID string `json:"related_video_id"`
	Position       int    `json:"position"`
}

// CaptionTrack is one available caption track for a video. The text is a separate
// read, so this is the index and not the captions.
type CaptionTrack struct {
	VideoID      string `json:"video_id"`
	LanguageCode string `json:"language_code"`
	Name         string `json:"name"`
	// BaseURL only returns bytes when it came from a mobile player. The watch page
	// lists the same tracks with URLs that answer 200 and empty. Doc 01 section 3.
	BaseURL string `json:"base_url"`
	// Kind is "asr" on an auto-generated track and empty on a human one, which is
	// why IsAutoGenerated exists: an empty string is not an answer to a question.
	Kind            string `json:"kind,omitempty"`
	IsAutoGenerated bool   `json:"is_auto_generated"`
	// VssID is the track's own name for itself, ".en" for the human English track
	// and "a.en" for the machine one. It is the only field that tells two tracks
	// in the same language apart in a way that survives a rename.
	VssID string `json:"vss_id,omitempty"`
	// TrackName names a second track in the same language, e.g. a commentary. It
	// is empty on nearly everything.
	TrackName string `json:"track_name,omitempty"`
	// IsTranslatable says whether YouTube will machine-translate this track, and
	// TranslatableTo is the list it offers. The translation is the site's own, so
	// asking for one is a read and not a thing we compute.
	IsTranslatable bool      `json:"is_translatable,omitempty"`
	TranslatableTo []string  `json:"translatable_to,omitempty" table:"-"`
	FetchedAt      time.Time `json:"fetched_at,omitzero"`
}

// TranscriptSegment is one timed line of a transcript.
type TranscriptSegment struct {
	StartSeconds float64 `json:"start"`
	DurSeconds   float64 `json:"dur"`
	Text         string  `json:"text"`
}

// SearchResult is the thin polymorphic row for mixed search output.
type SearchResult struct {
	EntityType string `json:"entity_type"`
	ID         string `json:"id"`
	Title      string `json:"title"`
	URL        string `json:"url"`
}

// Suggestion is one search-autocomplete suggestion, wrapped so the suggest
// operation emits a record the renderer and a host can both address.
type Suggestion struct {
	Text string `json:"suggestion" kit:"id" table:"suggestion"`
}

// QueueItem is one pending crawl-queue entry.
type QueueItem struct {
	ID         int64  `json:"id"`
	URL        string `json:"url"`
	EntityType string `json:"entity_type"`
	Status     string `json:"status"`
	Priority   int    `json:"priority"`
}

// JobRecord is one crawl job's history row.
type JobRecord struct {
	JobID       string    `json:"job_id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Status      string    `json:"status"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
}

// PageData holds the JSON blobs scraped from a YouTube HTML page.
type PageData struct {
	HTML          string
	InitialData   any
	PlayerResp    any
	YTCFG         map[string]any
	APIKey        string
	ClientVersion string
	VisitorData   string
}

// VideoOptions controls what FetchVideo gathers.
//
// The zero value is one request. The watch page carries the player response, the
// initial data, the chapter panel, the related videos and the microdata in one
// 1.3 MB read, so the default is s1 alone and every field below costs a request
// that the envelope's surfaces then name. Doc 05 section 2.
type VideoOptions struct {
	// Formats calls the mobile player for the stream list, which no other surface
	// has: the watch page's twenty six formats carry no URL.
	Formats bool
	// Captions calls the mobile player for caption tracks whose baseUrl returns
	// bytes. The watch page lists the same tracks with URLs that answer empty.
	Captions bool
	// Transcript fetches and attaches the transcript text, which implies Captions.
	Transcript bool
	// Lang is the preferred transcript language.
	Lang string
	// Microdata parses the page's own schema.org markup, for comparing the parser
	// against the site rather than against a fixture. No extra request.
	Microdata bool
	// Thumbnails HEADs the constructed renditions so the list only reports the ones
	// the CDN actually has. Up to five requests to i.ytimg.com.
	Thumbnails bool
	// NoPlayer forbids the mobile player call whatever else was asked for, and the
	// envelope records what that cost.
	NoPlayer bool
	// Next calls /next for the related videos past the first twenty and the comment
	// continuation token.
	Next bool
}

// PageOptions bounds a paginated stream.
type PageOptions struct {
	Max      int // max rows to emit (0 = unlimited)
	MaxPages int // max continuation pages (0 = unlimited)
	Enrich   bool
}

// CommentOptions controls the comment stream.
type CommentOptions struct {
	Max      int
	MaxPages int
	Replies  bool
	Sort     string // "top" | "new"
}

// --- YouTube Music, doc 03 section 12 ---
//
// music.youtube.com is a second app over the same catalogue, on surface s10 with
// its own renderers and its own answers, so it gets its own record kinds. The
// same id is "1.8B views" on www and "2B plays" on music, with different artist
// attribution, which is why a track is not folded into the video record.
//
// Nothing here is classified by a rendered word. Every endpoint the payload
// hands out is typed: a browse endpoint carries a pageType and a watch endpoint
// carries a musicVideoType, so an album is still an album under --hl ja, where
// the shelf is not headed "Albums" any more.

// MusicItem is one lockup on a music page: a shelf entry that names something
// else rather than describing itself. Kind is read off the endpoint, so it is
// what the payload says the thing is and not what the shelf it sat in was
// called.
type MusicItem struct {
	// Kind is album, track, playlist or artist.
	Kind string `json:"kind"`
	ID   string `json:"id"`
	URL  string `json:"url,omitempty"`

	Title string `json:"title,omitempty"`
	// Subtitle is the second line as rendered, kept whole because its parts are
	// language-dependent and the fields below already hold the ones with a shape.
	Subtitle string `json:"subtitle,omitempty"`
	// Shelf is the rendered heading this sat under: "Albums", "Live performances".
	// It is a label for a reader, not a type. The type is Kind.
	Shelf string `json:"shelf,omitempty"`

	// PlaylistID is the OLAK5uy_ playlist that plays an album lockup.
	PlaylistID string `json:"playlist_id,omitempty"`
	// AlbumType is the word an album lockup states for itself, "Single" or "EP".
	// An entry in the albums shelf states nothing, so empty means album.
	AlbumType string `json:"album_type,omitempty"`
	Year      string `json:"year,omitempty"`
	// MusicVideoType is ATV for an art track and OMV, UGC or OFFICIAL_SOURCE_MUSIC
	// for a video, off the watch endpoint.
	MusicVideoType string `json:"music_video_type,omitempty"`
	// CountText is the rendered number the lockup carried, "1.8B views" on a video
	// and "2B plays" on a track. Which one it is is the lockup's business.
	CountText string `json:"count_text,omitempty"`

	ArtistNames []string    `json:"artist_names,omitempty"`
	ArtistIDs   []string    `json:"artist_ids,omitempty"`
	Thumbnails  []Thumbnail `json:"thumbnails,omitempty"`
}

// Artist is a YouTube Music artist page.
type Artist struct {
	// ArtistID is the UC channel id, or an MPLA browse id for an artist with no
	// channel of their own.
	ArtistID string `json:"id" kit:"id" table:"id"`
	URL      string `json:"url,omitempty" table:"url,url"`

	Name        string `json:"name,omitempty" table:"name,truncate"`
	Description string `json:"description,omitempty" kit:"body" table:"-"`
	// ChannelID is the channel the subscribe button names. It is the same id as
	// ArtistID whenever the artist has a channel, and browsing a person's user
	// channel can land on the artist page of their official artist channel, in
	// which case the two differ and this is the one that was asked for.
	ChannelID string `json:"channel_id,omitempty" kit:"link,kind=youtube/channel" table:"-"`
	// Handle is the @name, which a person's own channel renders where an official
	// artist channel renders a listener count.
	Handle string `json:"handle,omitempty" table:"-"`

	SubscriberCount     int64  `json:"subscriber_count,omitempty" table:"subscribers"`
	SubscriberCountText string `json:"subscriber_count_text,omitempty" table:"-"`
	// MonthlyListenersText is music's own number and has no counterpart on www.
	MonthlyListenersText string `json:"monthly_listeners_text,omitempty" table:"-"`
	// MetadataParts is a rendered fragment this read did not recognise, kept
	// verbatim rather than dropped. A lockup's count lands here: a search row
	// renders subscribers and a top result card renders a monthly audience, only
	// the unit word separates them, and both of the fields above mean one of the
	// two. The artist page states them properly and that read fills them in.
	MetadataParts []string `json:"metadata_parts,omitempty" table:"-"`

	Thumbnails []Thumbnail `json:"thumbnails,omitempty" table:"-"`

	// TopTracks is the songs shelf, which is a list of rows rather than lockups
	// and carries a play count per row.
	TopTracks []Track `json:"top_tracks,omitempty" table:"-"`
	// Albums and Singles split the discography the way the page does. A singles
	// entry names its own type where an albums entry says only the year, so the
	// split survives a language change.
	Albums  []MusicItem `json:"albums,omitempty" table:"-"`
	Singles []MusicItem `json:"singles,omitempty" table:"-"`
	// Videos holds every video lockup on the page, live performances included,
	// each carrying the shelf it came from.
	Videos         []MusicItem `json:"videos,omitempty" table:"-"`
	Playlists      []MusicItem `json:"playlists,omitempty" table:"-"`
	RelatedArtists []MusicItem `json:"related_artists,omitempty" table:"-"`

	Envelope
}

// Album is a YouTube Music album, single or EP.
type Album struct {
	// AlbumID is the MPREb browse id. PlaylistID is the OLAK5uy_ playlist that
	// holds the same tracks and is what www knows the album by.
	AlbumID    string `json:"id" kit:"id" table:"id"`
	PlaylistID string `json:"playlist_id,omitempty" kit:"link,kind=youtube/playlist" table:"-"`
	URL        string `json:"url,omitempty" table:"url,url"`

	Title string `json:"title,omitempty" table:"title,truncate"`
	// ArtistNames and ArtistIDs are plural because a compilation or a collab is
	// credited to more than one artist, and an id is present only for a credit the
	// page linked.
	ArtistNames []string `json:"artist_names,omitempty" table:"artist,truncate"`
	ArtistIDs   []string `json:"artist_ids,omitempty" table:"-"`

	// AlbumType is the page's own word: Album, Single, EP.
	AlbumType string `json:"album_type,omitempty" table:"-"`
	Year      string `json:"year,omitempty" table:"year"`

	// TrackCount is the number of rows this read saw. TrackCountText is what the
	// header said, and the two disagree when a track is not available here.
	TrackCount     int    `json:"track_count,omitempty" table:"tracks"`
	TrackCountText string `json:"track_count_text,omitempty" table:"-"`
	DurationText   string `json:"duration_text,omitempty" table:"-"`

	Description string      `json:"description,omitempty" kit:"body" table:"-"`
	Thumbnails  []Thumbnail `json:"thumbnails,omitempty" table:"-"`

	Envelope
}

// Track is a song as YouTube Music describes it.
//
// The id is a video id, so yt://track/<id> and yt://video/<id> would name one
// thing seen through two apps, and doc 04 links the two records with a claim
// rather than merging them.
type Track struct {
	VideoID string `json:"id" kit:"id" table:"id"`
	URL     string `json:"url,omitempty" table:"url,url"`

	Title       string   `json:"title,omitempty" table:"title,truncate"`
	ArtistNames []string `json:"artist_names,omitempty" table:"artist,truncate"`
	ArtistIDs   []string `json:"artist_ids,omitempty" table:"-"`
	AlbumID     string   `json:"album_id,omitempty" table:"-"`
	AlbumTitle  string   `json:"album_title,omitempty" table:"album,truncate"`

	DurationSeconds int    `json:"duration_seconds,omitempty" table:"-"`
	DurationText    string `json:"duration_text,omitempty" table:"duration"`
	// PlaysText is music's count and is not the view count: the art track and the
	// official video of one song are two ids with two numbers.
	PlaysText  string `json:"plays_text,omitempty" table:"-"`
	IsExplicit bool   `json:"is_explicit,omitempty" table:"-"`
	// MusicVideoType is ATV for an art track and OMV, UGC or OFFICIAL_SOURCE_MUSIC
	// for a video. It is the field that says which of the two a row is.
	MusicVideoType string `json:"music_video_type,omitempty" table:"-"`
	Year           string `json:"year,omitempty" table:"-"`
	// Position is the track number on an album page and zero everywhere else.
	Position int `json:"position,omitempty" table:"-"`
	// MetadataParts is a rendered fragment this read did not recognise, kept
	// verbatim rather than dropped. A podcast episode renders its publish date
	// and the show it is from where a song renders its artist, and neither of
	// those has a field here.
	MetadataParts []string `json:"metadata_parts,omitempty" table:"-"`

	Thumbnails []Thumbnail `json:"thumbnails,omitempty" table:"-"`
	Lyrics     string      `json:"lyrics,omitempty" kit:"body" table:"-"`
	// LyricsSource is the footer credit, "Source: Musixmatch". Lyrics without it
	// are still lyrics, but the credit is part of what the page served.
	LyricsSource string `json:"lyrics_source,omitempty" table:"-"`

	Envelope
}
