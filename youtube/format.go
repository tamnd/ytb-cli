package youtube

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// format.go is the stream list: what a video's tracks are, whether they can be
// fetched, and for how long.
//
// The two players disagree about what a format is and neither is enough alone.
// Measured on dQw4w9WgXcQ:
//
//	watch page, WEB    every adaptiveFormats entry carries contentLength and
//	                   neither url nor signatureCipher, so there is nothing to
//	                   fetch. Only the muxed itag 18 carries a signatureCipher.
//	/player, ANDROID   every entry carries a plain url that needs no cipher, and
//	                   the muxed one carries no contentLength.
//
// So the list is read from s3, and a read that could not reach s3 says the list it
// is printing has no URLs behind it rather than letting the itags imply otherwise.
//
// The throughput note the command prints once is the useful part. Doc 01 section 8
// measured a plain GET of a googlevideo URL at 32 KiB/s and the same URL fetched in
// ranges at 4 MiB/s, 132 times faster, so whether a caller sends a Range header is
// the difference between a minute and two hours.

// FormatList is one read of a video's stream list.
type FormatList struct {
	VideoID string        `json:"video_id"`
	Formats []VideoFormat `json:"formats"`
	// Expires is when the URLs behind this list stop working. It is the deadline
	// on the URLs themselves, not the expiresInSeconds the response also carries:
	// those two disagree by a minute, measured 21540 against a URL expire exactly
	// 21600 seconds out, and the CDN enforces the one in the URL.
	Expires time.Time `json:"expires,omitzero"`
	// Fetchable says whether the read that answered carried URLs at all. False is a
	// metadata-only list: real itags, real sizes, nothing to download.
	Fetchable bool `json:"fetchable"`
	Envelope
}

// FormatList reads a video's stream list from the mobile player, falling back to
// the watch page when it will not answer.
func (c *Client) FormatList(ctx context.Context, idOrURL string) (*FormatList, error) {
	videoID := ExtractVideoID(idOrURL)
	if videoID == "" {
		videoID = idOrURL
	}
	list := &FormatList{VideoID: videoID, Envelope: newEnvelope("format_list")}

	if pr, err := NewInnerTube(c).AndroidPlayer(ctx, videoID); err == nil && pr != nil {
		if sd := mapValue(pr, "streamingData"); sd != nil {
			list.addSurface(SurfaceMobilePlayer)
			list.addClient("ANDROID")
			list.addSource(ClientANDROID().Endpoint("player"))
			list.Formats = ParseVideoFormats(pr, videoID)
			list.Expires = streamExpiry(sd, time.Now())
			list.Fetchable = streamingHasURLs(sd)
		}
	}
	if len(list.Formats) == 0 {
		pr := c.playerResponse(ctx, idOrURL, videoID)
		if pr == nil {
			return nil, fmt.Errorf("formats for %s: neither the mobile player nor the watch page answered with a player response", videoID)
		}
		list.addSurface(SurfaceWatchHTML)
		list.addClient("WEB")
		list.addSource(NormalizeVideoURL(videoID))
		list.Formats = ParseVideoFormats(pr, videoID)
		list.miss("the mobile player did not answer, so this list came off the watch page: the itags and sizes are real and there are no URLs behind them")
	}
	sortFormats(list.Formats)
	annotateFormats(list.Formats)
	if !list.Fetchable {
		list.setVia("fetchable", "no format in the response carried a url")
	}
	// One sentence for all of them, because it is one fact about the client: the
	// muxed formats come back without a contentLength and the adaptive ones do not.
	if unsized := unsizedITags(list.Formats); unsized != "" {
		list.miss("itag %s came back with no contentLength, so the size is unknown until a ranged request reports one", unsized)
	}
	c.stamp(list)
	return list, nil
}

// streamExpiry reads the deadline off the first URL in the response, falling back
// to expiresInSeconds from now when no format carried one.
func streamExpiry(sd map[string]any, now time.Time) time.Time {
	for _, name := range []string{"formats", "adaptiveFormats"} {
		for _, item := range arrayValue(sd[name]) {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if t := urlExpiry(stringValue(m["url"])); !t.IsZero() {
				return t
			}
		}
	}
	if sec, err := strconv.Atoi(stringValue(sd["expiresInSeconds"])); err == nil && sec > 0 {
		return now.Add(time.Duration(sec) * time.Second)
	}
	return time.Time{}
}

// streamingHasURLs reports whether anything in the response is actually fetchable.
func streamingHasURLs(sd map[string]any) bool {
	for _, name := range []string{"formats", "adaptiveFormats"} {
		for _, item := range arrayValue(sd[name]) {
			if m, ok := item.(map[string]any); ok && stringValue(m["url"]) != "" {
				return true
			}
		}
	}
	return false
}

// sortFormats puts the list in a stable, readable order: audio, then video, then
// muxed, and smallest bitrate first inside each group.
//
// Any order would do as long as it is the same one twice. ParseVideoFormats
// deduplicates through a map, so before this the command printed the same 27
// formats in a different order on every run, which makes two outputs impossible to
// diff and looks like the site changed.
func sortFormats(formats []VideoFormat) {
	sort.SliceStable(formats, func(i, j int) bool {
		a, b := formats[i], formats[j]
		if ra, rb := kindRank(a), kindRank(b); ra != rb {
			return ra < rb
		}
		if a.Bitrate != b.Bitrate {
			return a.Bitrate < b.Bitrate
		}
		return a.ITag < b.ITag
	})
}

func kindRank(f VideoFormat) int {
	switch f.Kind {
	case "audio":
		return 0
	case "video":
		return 1
	default:
		return 2
	}
}

// annotateFormats fills in the per-format note, which is empty for a format with
// nothing worth saying about it.
func annotateFormats(formats []VideoFormat) {
	for i := range formats {
		if formats[i].ContentLength == 0 {
			formats[i].Note = "size unknown"
		}
	}
}

// unsizedITags lists the itags with no contentLength, as "18" or "18, 22".
func unsizedITags(formats []VideoFormat) string {
	var out []string
	for _, f := range formats {
		if f.ContentLength == 0 {
			out = append(out, strconv.Itoa(f.ITag))
		}
	}
	return strings.Join(out, ", ")
}

// formatKind is audio, video or muxed. Muxed means one file with both tracks,
// which is the only thing a player can open without merging.
func formatKind(mime string, adaptive bool) string {
	switch {
	case strings.HasPrefix(mime, "audio/"):
		return "audio"
	case !adaptive:
		return "muxed"
	default:
		return "video"
	}
}

// formatContainer is mp4 or webm, from the mime type rather than from the itag,
// because the itag table is folklore and the mime type is what the response said.
func formatContainer(mime string) string {
	if i := strings.Index(mime, ";"); i >= 0 {
		mime = mime[:i]
	}
	if i := strings.Index(mime, "/"); i >= 0 {
		return mime[i+1:]
	}
	return mime
}

// formatCodec is the codecs parameter of the mime type, with the separator changed
// so a muxed format's two codecs read as one cell: avc1.42001E+mp4a.40.2.
func formatCodec(mime string) string {
	i := strings.Index(mime, `codecs="`)
	if i < 0 {
		return ""
	}
	rest := mime[i+len(`codecs="`):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return ""
	}
	parts := strings.Split(rest[:j], ",")
	for k := range parts {
		parts[k] = strings.TrimSpace(parts[k])
	}
	return strings.Join(parts, "+")
}

// QualityText is the one quality word for this format.
//
// The two kinds answer from different fields. A video format's qualityLabel is
// 1080p and its quality is hd1080, and the label is the one people use. An audio
// format has no label and its quality is "tiny" for every bitrate, so the answer
// is audioQuality: itag 140 and itag 251 are both tiny and both medium.
func (f VideoFormat) QualityText() string {
	if f.Kind == "audio" {
		if q := strings.TrimPrefix(f.AudioQuality, "AUDIO_QUALITY_"); q != "" && q != f.AudioQuality {
			return strings.ToLower(q)
		}
		return strings.ToLower(f.AudioQuality)
	}
	if f.QualityLabel != "" {
		return f.QualityLabel
	}
	return f.Quality
}
