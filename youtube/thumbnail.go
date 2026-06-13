package youtube

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
)

// Thumbnail is one available preview image for a video.
type Thumbnail struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Name   string `json:"name"`
}

// ThumbnailNames are the standard i.ytimg.com renditions, largest first.
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

// Thumbnails returns the standard rendition URLs for a video, largest first.
// Not every video has every rendition (maxres in particular); Download probes
// availability.
func Thumbnails(videoID string) []Thumbnail {
	out := make([]Thumbnail, 0, len(thumbnailRenditions))
	for _, r := range thumbnailRenditions {
		out = append(out, Thumbnail{
			URL:    fmt.Sprintf("https://i.ytimg.com/vi/%s/%s.jpg", videoID, r.name),
			Width:  r.w,
			Height: r.h,
			Name:   r.name,
		})
	}
	return out
}

// DownloadThumbnail fetches the best available rendition for a video to dst,
// trying renditions largest-first and skipping any that 404.
func (c *Client) DownloadThumbnail(ctx context.Context, videoID, dst string) (Thumbnail, error) {
	for _, t := range Thumbnails(videoID) {
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
		_, copyErr := io.Copy(f, resp.Body)
		_ = resp.Body.Close()
		if cerr := f.Close(); cerr != nil && copyErr == nil {
			copyErr = cerr
		}
		if copyErr != nil {
			return Thumbnail{}, copyErr
		}
		return t, nil
	}
	return Thumbnail{}, fmt.Errorf("no thumbnail available for %s", videoID)
}
