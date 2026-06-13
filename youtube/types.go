package youtube

import "time"

// Video is the central record: one YouTube video with full metadata.
type Video struct {
	VideoID            string    `json:"video_id"`
	Title              string    `json:"title"`
	Description        string    `json:"description"`
	ChannelID          string    `json:"channel_id"`
	ChannelName        string    `json:"channel_name"`
	DurationSeconds    int       `json:"duration_seconds"`
	DurationText       string    `json:"duration_text"`
	ViewCount          int64     `json:"view_count"`
	CommentCount       int64     `json:"comment_count"`
	LikeCount          int64     `json:"like_count"`
	PublishedText      string    `json:"published_text"`
	PublishedAt        time.Time `json:"published_at"`
	UploadDate         string    `json:"upload_date"`
	IsLive             bool      `json:"is_live"`
	IsShort            bool      `json:"is_short"`
	Category           string    `json:"category"`
	Tags               []string  `json:"tags"`
	ThumbnailURL       string    `json:"thumbnail_url"`
	URL                string    `json:"url"`
	EmbedURL           string    `json:"embed_url"`
	Transcript         string    `json:"transcript"`
	TranscriptLanguage string    `json:"transcript_language"`
	// Extended metadata from microformat / videoDetails.
	AvailableCountries  []string  `json:"available_countries"`
	IsFamilySafe        bool      `json:"is_family_safe"`
	AllowRatings        bool      `json:"allow_ratings"`
	AgeRestricted       bool      `json:"age_restricted"`
	LocationDescription string    `json:"location_description"`
	Hashtags            []string  `json:"hashtags"`
	FetchedAt           time.Time `json:"fetched_at"`
}

// Channel is one YouTube channel.
type Channel struct {
	ChannelID         string    `json:"channel_id"`
	Handle            string    `json:"handle"`
	Title             string    `json:"title"`
	Description       string    `json:"description"`
	AvatarURL         string    `json:"avatar_url"`
	BannerURL         string    `json:"banner_url"`
	SubscribersText   string    `json:"subscribers_text"`
	VideosText        string    `json:"videos_text"`
	ViewsText         string    `json:"views_text"`
	Country           string    `json:"country"`
	JoinedDateText    string    `json:"joined_date_text"`
	UploadsPlaylistID string    `json:"uploads_playlist_id"`
	URL               string    `json:"url"`
	SubscriberCount   int64     `json:"subscriber_count"`
	VideoCount        int64     `json:"video_count"`
	ViewCount         int64     `json:"view_count"`
	Keywords          []string  `json:"keywords"`
	TrailerVideoID    string    `json:"trailer_video_id"`
	IsVerified        bool      `json:"is_verified"`
	FetchedAt         time.Time `json:"fetched_at"`
}

// Comment is one comment or reply. Replies carry the parent comment id in ParentID.
type Comment struct {
	ID                 string    `json:"id"`
	VideoID            string    `json:"video_id"`
	ParentID           string    `json:"parent_id"`
	AuthorChannelID    string    `json:"author_channel_id"`
	AuthorDisplayName  string    `json:"author_display_name"`
	AuthorProfileImage string    `json:"author_profile_image_url"`
	TextDisplay        string    `json:"text_display"`
	LikeCount          int64     `json:"like_count"`
	ReplyCount         int       `json:"reply_count"`
	IsOwnerComment     bool      `json:"is_owner_comment"`
	PublishedText      string    `json:"published_text"`
	PublishedAt        time.Time `json:"published_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	FetchedAt          time.Time `json:"fetched_at"`
}

// Chapter is one chapter marker on a video.
type Chapter struct {
	VideoID      string `json:"video_id"`
	Title        string `json:"title"`
	StartSeconds int    `json:"start_seconds"`
	ThumbnailURL string `json:"thumbnail_url"`
	Position     int    `json:"position"`
}

// CommunityPost is one community/posts-tab post. Attachments is a JSON array.
type CommunityPost struct {
	PostID        string    `json:"post_id"`
	ChannelID     string    `json:"channel_id"`
	AuthorName    string    `json:"author_name"`
	AuthorAvatar  string    `json:"author_avatar_url"`
	ContentText   string    `json:"content_text"`
	LikeCount     int64     `json:"like_count"`
	ReplyCount    int       `json:"reply_count"`
	VoteCount     string    `json:"vote_count_text"`
	PublishedText string    `json:"published_text"`
	Attachments   string    `json:"attachments"`
	FetchedAt     time.Time `json:"fetched_at"`
}

// VideoFormat is one streaming format (muxed or adaptive) of a video.
type VideoFormat struct {
	VideoID       string `json:"video_id"`
	ITag          int    `json:"itag"`
	MimeType      string `json:"mime_type"`
	Quality       string `json:"quality"`
	QualityLabel  string `json:"quality_label"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	FPS           int    `json:"fps"`
	Bitrate       int64  `json:"bitrate"`
	ContentLength int64  `json:"content_length"`
	IsAdaptive    bool   `json:"is_adaptive"`
	AudioQuality  string `json:"audio_quality"`
}

// Playlist is one playlist's header.
type Playlist struct {
	PlaylistID      string    `json:"playlist_id"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	ChannelID       string    `json:"channel_id"`
	ChannelName     string    `json:"channel_name"`
	VideoCount      int       `json:"video_count"`
	ViewCountText   string    `json:"view_count_text"`
	LastUpdatedText string    `json:"last_updated_text"`
	URL             string    `json:"url"`
	FetchedAt       time.Time `json:"fetched_at"`
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

// CaptionTrack is one available caption track for a video.
type CaptionTrack struct {
	VideoID         string    `json:"video_id"`
	LanguageCode    string    `json:"language_code"`
	Name            string    `json:"name"`
	BaseURL         string    `json:"base_url"`
	Kind            string    `json:"kind"`
	IsAutoGenerated bool      `json:"is_auto_generated"`
	FetchedAt       time.Time `json:"fetched_at"`
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
type VideoOptions struct {
	Player     bool // call /player (default true)
	Next       bool // call /next for chapters/related/comment token
	Transcript bool // fetch the transcript text
	Lang       string
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

// --- YouTube Music ---

// Artist is a YouTube Music artist.
type Artist struct {
	ArtistID        string    `json:"artist_id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	SubscribersText string    `json:"subscribers_text"`
	ThumbnailURL    string    `json:"thumbnail_url"`
	URL             string    `json:"url"`
	FetchedAt       time.Time `json:"fetched_at"`
}

// Album is a YouTube Music album.
type Album struct {
	AlbumID         string    `json:"album_id"`
	Title           string    `json:"title"`
	ArtistID        string    `json:"artist_id"`
	ArtistName      string    `json:"artist_name"`
	AlbumType       string    `json:"album_type"`
	Year            string    `json:"year"`
	TrackCount      int       `json:"track_count"`
	DurationText    string    `json:"duration_text"`
	ThumbnailURL    string    `json:"thumbnail_url"`
	AudioPlaylistID string    `json:"audio_playlist_id"`
	Description     string    `json:"description"`
	URL             string    `json:"url"`
	FetchedAt       time.Time `json:"fetched_at"`
}

// Song is a YouTube Music song.
type Song struct {
	VideoID         string    `json:"video_id"`
	Title           string    `json:"title"`
	ArtistID        string    `json:"artist_id"`
	ArtistName      string    `json:"artist_name"`
	AlbumID         string    `json:"album_id"`
	AlbumName       string    `json:"album_name"`
	DurationSeconds int       `json:"duration_seconds"`
	DurationText    string    `json:"duration_text"`
	PlaysText       string    `json:"plays_text"`
	IsExplicit      bool      `json:"is_explicit"`
	VideoType       string    `json:"video_type"`
	ThumbnailURL    string    `json:"thumbnail_url"`
	Lyrics          string    `json:"lyrics"`
	URL             string    `json:"url"`
	FetchedAt       time.Time `json:"fetched_at"`
}

// AlbumTrack is the album↔song join.
type AlbumTrack struct {
	AlbumID  string `json:"album_id"`
	VideoID  string `json:"video_id"`
	Position int    `json:"position"`
}

// ArtistAlbum is the artist↔album join.
type ArtistAlbum struct {
	ArtistID  string `json:"artist_id"`
	AlbumID   string `json:"album_id"`
	AlbumType string `json:"album_type"`
}
