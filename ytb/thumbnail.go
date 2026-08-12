package ytb

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/tamnd/any-cli/kit/errs"
)

// thumbnail.go is the preview image plane: what the payload said, what the CDN can
// be asked for, and which of the two a given URL came from.
//
// The distinction is the whole point of the Source field. A watch page's
// microformat carries exactly one thumbnail, maxresdefault, and the five standard
// i.ytimg.com renditions are a naming convention rather than a promise: every one
// of them can be constructed for any video id, and a video with no maxres answers
// 404 with a 1097 byte body that is still Content-Type image/jpeg. So a
// constructed rendition is a hypothesis until a HEAD confirms it, and reporting
// one without that is how a list of five working URLs turns out to contain two
// that 404 for a caller who tries them later.

// Thumbnail is one preview image for a video.
type Thumbnail struct {
	URL    string `json:"url"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	// Name is the rendition, e.g. maxresdefault, and is empty for a payload
	// thumbnail whose URL is not one of the standard five.
	Name string `json:"name,omitempty"`
	// Source says where this URL came from, which is the difference between a fact
	// and a guess. See the Thumbnail* constants.
	Source string `json:"source,omitempty"`
	// Bytes is the Content-Length a HEAD reported, set only on a confirmed
	// rendition. It is worth keeping: it is the one signal that says which of two
	// present renditions is actually the bigger image.
	Bytes int64 `json:"bytes,omitempty"`
}

// Where a thumbnail URL came from.
const (
	// ThumbnailFromPayload means YouTube's own JSON listed this URL.
	ThumbnailFromPayload = "payload"
	// ThumbnailFromMicrodata means the page's schema.org ImageObject listed it.
	ThumbnailFromMicrodata = "microdata"
	// ThumbnailConstructed means the URL was built from the naming convention and
	// nothing has confirmed it exists.
	ThumbnailConstructed = "constructed"
	// ThumbnailConfirmed means it was built and then a HEAD answered 200.
	ThumbnailConfirmed = "confirmed"
)

// thumbnailRenditions are the standard i.ytimg.com names, largest first, with the
// dimensions the CDN serves them at.
var thumbnailRenditions = []struct {
	name string
	w, h int
}{
	{"maxresdefault", 1280, 720},
	{"sddefault", 640, 480},
	{"hqdefault", 480, 360},
	{"mqdefault", 320, 180},
	{"default", 120, 90},
}

// Thumbnails returns the constructed rendition URLs for a video, largest first.
// Every one is marked ThumbnailConstructed, because nothing here has checked that
// any of them exists. ConfirmThumbnails is what checks.
func Thumbnails(videoID string) []Thumbnail {
	out := make([]Thumbnail, 0, len(thumbnailRenditions))
	for _, r := range thumbnailRenditions {
		out = append(out, Thumbnail{
			URL:    fmt.Sprintf("https://i.ytimg.com/vi/%s/%s.jpg", videoID, r.name),
			Width:  r.w,
			Height: r.h,
			Name:   r.name,
			Source: ThumbnailConstructed,
		})
	}
	return out
}

// ParseThumbnails reads a thumbnails array out of a payload node, largest last as
// YouTube orders them, and marks each one ThumbnailFromPayload.
func ParseThumbnails(v any) []Thumbnail {
	var out []Thumbnail
	for _, item := range arrayValue(v) {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		u := stringValue(m["url"])
		if u == "" {
			continue
		}
		out = append(out, Thumbnail{
			URL:    absoluteThumbURL(u),
			Width:  int(int64Value(m["width"])),
			Height: int(int64Value(m["height"])),
			Name:   renditionName(u),
			Source: ThumbnailFromPayload,
		})
	}
	return out
}

// absoluteThumbURL fixes the protocol relative URLs some renderers still emit.
func absoluteThumbURL(u string) string {
	if strings.HasPrefix(u, "//") {
		return "https:" + u
	}
	return u
}

// renditionName reports which standard rendition a URL is, or "" when it is not
// one of them.
//
// The name is only given to a plain .jpg with no query, because the five standard
// renditions are a .jpg naming convention and nothing else. A watch page lists two
// other shapes: hqdefault.jpg?sqp=... four times, which are cropped derivatives
// that come back 168x94, 196x110, 246x138 and 336x188 rather than hqdefault's
// 480x360, and vi_webp/maxresdefault.webp, which is a different format at a
// different size. Naming any of them would make a caller that filters on
// name == "hqdefault" get five different images, none of them the right one.
func renditionName(u string) string {
	if strings.Contains(u, "?") || !strings.HasSuffix(u, ".jpg") {
		return ""
	}
	for _, r := range thumbnailRenditions {
		if strings.Contains(u, "/"+r.name+".") {
			return r.name
		}
	}
	return ""
}

// ConfirmThumbnails HEADs each constructed rendition and returns the ones the CDN
// actually has, marked ThumbnailConfirmed with the byte size it reported.
//
// The 404 body is a real JPEG placeholder, so the status code is the only signal
// and the Content-Type is not. Renditions already sourced from a payload are passed
// through untouched: YouTube naming a URL is better evidence than a HEAD, and it
// costs no request.
func (c *Client) ConfirmThumbnails(ctx context.Context, thumbs []Thumbnail) []Thumbnail {
	out := make([]Thumbnail, 0, len(thumbs))
	for _, t := range thumbs {
		if t.Source != ThumbnailConstructed {
			out = append(out, t)
			continue
		}
		size, ok := c.headThumbnail(ctx, t.URL)
		if !ok {
			continue
		}
		t.Source = ThumbnailConfirmed
		t.Bytes = size
		out = append(out, t)
	}
	return out
}

// headThumbnail reports whether the CDN has a URL, and how big it is.
func (c *Client) headThumbnail(ctx context.Context, url string) (int64, bool) {
	c.noteRequest(http.MethodHead, url)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return 0, false
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, false
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, false
	}
	return resp.ContentLength, true
}

// DownloadThumbnail fetches the best available rendition for a video to dst,
// trying renditions largest first and skipping any that 404.
func (c *Client) DownloadThumbnail(ctx context.Context, videoID, dst string) (Thumbnail, error) {
	for _, t := range Thumbnails(videoID) {
		c.noteRequest(http.MethodGet, t.URL)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.URL, nil)
		if err != nil {
			return Thumbnail{}, err
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return Thumbnail{}, err
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			continue
		}
		f, err := os.Create(dst)
		if err != nil {
			_ = resp.Body.Close()
			return Thumbnail{}, err
		}
		written, copyErr := io.Copy(f, resp.Body)
		_ = resp.Body.Close()
		if cerr := f.Close(); cerr != nil && copyErr == nil {
			copyErr = cerr
		}
		if copyErr != nil {
			return Thumbnail{}, copyErr
		}
		t.Source = ThumbnailConfirmed
		t.Bytes = written
		return t, nil
	}
	return Thumbnail{}, errs.NotFound("no thumbnail available for %s", videoID)
}
