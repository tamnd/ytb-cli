package youtube

import (
	"fmt"

	"github.com/tamnd/ytb-cli/pkg/srv3"
)

// records.go is the six kinds of doc 03 that are a row inside something else as
// well as a record in their own right.
//
// A thumbnail is a field of a video and it is also what `ytb thumbnail` reads.
// A chapter, a caption track, a format and a transcript cue are the same: nested
// in one read, the whole answer in another. Doc 03 lists `thumbnail` twice for
// exactly this reason, once in the twelve kinds and once in section 13's shared
// types, and the two are not the same thing.
//
// Putting an envelope on the shared type would put eight provenance fields on
// every one of a video's five thumbnails and twenty chapters, repeated, saying
// what the video record already said. Leaving it off entirely means `ytb formats
// -o json` hands back rows with no tier on them, which is the promise in doc 01
// section 12 broken for a third of the commands.
//
// So the shared type stays lean and the record wraps it. The embedded struct
// flattens in JSON, so a wrapped row serialises as its own fields plus the
// envelope's, which is the record doc 03 describes and not a row with a envelope
// key hanging off it.

// FormatRecord is one streaming format as `ytb formats` hands it back.
// Doc 03 section 7. URI: yt://video/<id>#format/<itag>.
type FormatRecord struct {
	VideoFormat
	Envelope
}

// ThumbnailRecord is one rendition as `ytb thumbnail` hands it back.
// Doc 03 section 8. URI: yt://video/<id>#thumb/<rendition>.
type ThumbnailRecord struct {
	// VideoID is on the record and not on the shared type, because a thumbnail
	// nested in a video already knows which video it belongs to and one on its own
	// does not.
	VideoID string `json:"video_id"`
	Thumbnail
	Envelope
}

// ChapterRecord is one chapter as `ytb chapters` hands it back.
// Doc 03 section 9. URI: yt://video/<id>#chapter/<seconds>.
type ChapterRecord struct {
	Chapter
	Envelope
}

// CaptionRecord is one caption track as `ytb captions` hands it back.
// Doc 03 section 6. URI: yt://video/<id>#captions/<lang>.
type CaptionRecord struct {
	CaptionTrack
	Envelope
}

// TranscriptRecord is one cue of a transcript.
//
// Doc 03 section 6 models `transcript` as one record holding every segment, and
// that is what the store and `ytb serve` want. A terminal wants a line per cue,
// because a transcript is the one record people read with their eyes and 900
// segments in a single JSON object is not something anybody reads.
//
// Both exist. This is the row; Transcript below is the whole thing, and they
// carry the same envelope.
type TranscriptRecord struct {
	VideoID      string `json:"video_id"`
	LanguageCode string `json:"language_code,omitempty"`
	// Kind is "asr" when the track was machine-generated, matching CaptionTrack.
	Kind string `json:"kind,omitempty"`
	srv3.Cue
	Envelope
}

// Transcript is a whole caption track as text. Doc 03 section 6.
// URI: yt://video/<id>#transcript/<lang>.
type Transcript struct {
	VideoID      string `json:"video_id"`
	LanguageCode string `json:"language_code,omitempty"`
	Kind         string `json:"kind,omitempty"`
	// Name is the track's own label, e.g. "English (auto-generated)".
	Name string `json:"name,omitempty"`
	// Segments is the timeline, parsed once from srv3 and never re-fetched per
	// output format. Doc 01 section 3.2: asking the server for a second format is
	// a second request and it is ignored anyway.
	Segments []srv3.Cue `json:"segments"`
	// Text is the segments joined, which is what most callers want and what a
	// caller would otherwise write the same join for.
	Text string `json:"text"`
	Envelope
}

// HashtagRecord is a hashtag feed's own header. Doc 03 section 11.
// URI: yt://hashtag/<tag>.
type HashtagRecord struct {
	// Tag is without the leading #, which is how it appears in a URL and in a run.
	Tag   string `json:"tag" kit:"id" table:"tag"`
	URL   string `json:"url" table:"-"`
	Title string `json:"title,omitempty" table:"title"`
	// VideoCountText is the rendered count and never a parsed number, because the
	// header carries "6.7M videos" and rounding is not something to undo.
	VideoCountText string `json:"video_count_text,omitempty" table:"videos"`
	// ChannelCountText is the other half of the same line. Doc 03 section 11 does
	// not list it and the header carries it, so it is kept rather than dropped: a
	// hashtag with 6.7M videos from 1.3M channels is a different thing from one
	// with 6.7M videos from four.
	ChannelCountText string `json:"channel_count_text,omitempty" table:"channels"`
	Envelope
}

// newFormatRecord and the rest are the one place a shared type becomes a record,
// so the kind string and the URI shape are written down once each.

func newFormatRecord(f VideoFormat, e Envelope) FormatRecord {
	e.Kind = "format"
	return FormatRecord{VideoFormat: f, Envelope: e}
}

func newThumbnailRecord(videoID string, t Thumbnail, e Envelope) ThumbnailRecord {
	e.Kind = "thumbnail"
	return ThumbnailRecord{VideoID: videoID, Thumbnail: t, Envelope: e}
}

func newChapterRecord(ch Chapter, e Envelope) ChapterRecord {
	e.Kind = "chapter"
	return ChapterRecord{Chapter: ch, Envelope: e}
}

func newCaptionRecord(t CaptionTrack, e Envelope) CaptionRecord {
	e.Kind = "caption_track"
	return CaptionRecord{CaptionTrack: t, Envelope: e}
}

// FragmentURI is the address of a record that lives inside a video rather than
// beside one. Doc 03's table gives four of these and they are all the video's URI
// with a fragment on the end.
//
// They are not node URIs and nothing files them in the store: a chapter is not a
// thing that can be read on its own, so a node for one would be a node nothing
// could ever fetch. They exist so that a record printed on its own can say what
// it is the record of.
func FragmentURI(videoID, part, name string) string {
	return fmt.Sprintf("yt://video/%s#%s/%s", videoID, part, name)
}
