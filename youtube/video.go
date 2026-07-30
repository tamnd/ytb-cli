package youtube

import (
	"context"
	"errors"
	"fmt"
)

// ErrStop is returned by a streaming emit function to signal that no more items
// are needed. Streaming functions treat ErrStop as a clean stop (return nil).
var ErrStop = errors.New("youtube: stop iteration")

// VideoResult holds the full output of FetchVideo.
type VideoResult struct {
	Video        Video
	Formats      []VideoFormat
	Captions     []CaptionTrack
	Chapters     []Chapter
	Related      []RelatedVideo
	CommentToken string
}

// FetchVideo reads one video.
//
// The zero VideoOptions is one request. The watch page carries the player
// response, the initial data, the chapter panel, the related videos and the
// schema.org microdata, so there is nothing to add for a plain read and the
// record comes back with surfaces ["s1"]. Every option costs a request that the
// envelope then names, which is the point of naming them.
func (c *Client) FetchVideo(ctx context.Context, idOrURL string, opt VideoOptions) (*VideoResult, error) {
	videoURL := NormalizeVideoURL(idOrURL)
	data, code, err := c.FetchPageData(ctx, videoURL)
	if err != nil {
		return nil, fmt.Errorf("fetch video page %q: %w", videoURL, err)
	}
	if code == 404 || data == nil {
		return nil, fmt.Errorf("video not found: %s", videoURL)
	}

	video, related, contToken, err := ParseVideoPage(data, videoURL)
	if err != nil {
		return nil, err
	}

	var formats []VideoFormat
	if pr, ok := data.PlayerResp.(map[string]any); ok {
		formats = ParseVideoFormats(pr, video.VideoID)
	}
	tracks := video.CaptionTracks

	// The page's own markup about itself, for checking the parser against the site.
	// It is already in the response, so this costs nothing and is still opt in: it
	// is a second opinion and printing it by default would read as agreement.
	if opt.Microdata {
		video.Microdata = ParseVideoMicrodata(data.HTML)
		if video.Microdata == nil {
			video.miss("page carried no schema.org VideoObject")
		}
	}

	it := NewInnerTube(c)

	// The mobile player is the only surface with stream URLs, and the only one whose
	// caption baseUrl values return bytes. Doc 01 section 3.
	wantMobile := (opt.Formats || opt.Captions || opt.Transcript) && !opt.NoPlayer
	if wantMobile {
		if pr, perr := it.AndroidPlayer(ctx, video.VideoID); perr == nil && pr != nil {
			video.addSurface(SurfaceMobilePlayer)
			video.addClient("ANDROID")
			if opt.Formats {
				if pf := ParseVideoFormats(pr, video.VideoID); len(pf) > 0 {
					formats = pf
					video.setVia("formats", "s3 streamingData")
				}
			}
			// The watch page lists the same tracks with baseUrl values that answer
			// empty, so a mobile track of the same language replaces rather than adds.
			if mt := ParseCaptionTracks(pr, video.VideoID); len(mt) > 0 {
				tracks = mt
				video.CaptionTracks = mt
				video.setVia("caption_tracks", "s3 playerCaptionsTracklistRenderer")
			}
		} else {
			video.miss("mobile player did not answer, so no stream URLs and captions may not fetch")
		}
	}
	if opt.NoPlayer && (opt.Formats || opt.Captions || opt.Transcript) {
		video.miss("--no-player suppressed the s3 read, so stream URLs are absent")
	}

	// The stream list goes on the record only when it was asked for. The watch page
	// carries one for free and the plain read still leaves it off, because a record
	// with 29 formats on it that nobody asked for buries the twelve fields the
	// caller wanted, and doc 03 section 2.7 says absent unless --formats.
	if opt.Formats {
		sortFormats(formats)
		annotateFormats(formats)
		video.Formats = formats
		if len(formats) == 0 {
			video.miss("no formats: the watch page lists them without URLs and s3 was not read")
		}
	}

	// Chapters come off the watch page's engagement panel. /next answers with the
	// same macroMarkersListRenderer, so it is only worth a request when the page
	// did not carry one and the description might.
	chapters := ParseChapters(pageRoot(data.InitialData), video.VideoID, video.DescriptionRuns, video.Description)

	if opt.Next {
		if nextResp, nextErr := it.Next(ctx, video.VideoID, ""); nextErr == nil && nextResp != nil {
			video.addSurface(SurfaceInnerTube)
			video.addClient("WEB")
			if len(chapters) == 0 {
				chapters = ParseChapters(nextResp, video.VideoID, video.DescriptionRuns, video.Description)
			}
			nextRelated, _ := ParseContinuationRelatedVideos(nextResp, video.VideoID)
			related = append(related, nextRelated...)
			if ct := extractCommentContinuationToken(nextResp); ct != "" {
				contToken = ct
			}
		}
	}
	video.Chapters = chapters

	// A constructed rendition is a hypothesis until a HEAD confirms it, and the 404
	// body is a real JPEG, so nothing but the status code answers. See thumbnail.go.
	if opt.Thumbnails {
		video.Thumbnails = c.ConfirmThumbnails(ctx, mergeThumbnails(video.Thumbnails, Thumbnails(video.VideoID)))
		video.addSurface(SurfaceThumbCDN)
		video.ThumbnailURL = largestThumbnail(video.Thumbnails)
	}

	if opt.Transcript && len(tracks) > 0 {
		if track := pickCaptionTrack(tracks, opt.Lang); track != nil {
			_, segs, txErr := c.Transcript(ctx, video.VideoID, track.LanguageCode)
			if txErr == nil && len(segs) > 0 {
				parts := make([]string, 0, len(segs))
				for _, s := range segs {
					if s.Text != "" {
						parts = append(parts, s.Text)
					}
				}
				video.TranscriptLanguage = track.LanguageCode
				video.Transcript = joinLines(parts)
			} else {
				video.miss("transcript did not fetch for %s", track.LanguageCode)
			}
		}
	}

	if video.Player == "" && data.YTCFG != nil {
		video.Player = stringValue(data.YTCFG["PLAYER_JS_URL"])
	}

	return &VideoResult{
		Video:        *video,
		Formats:      formats,
		Captions:     tracks,
		Chapters:     chapters,
		Related:      related,
		CommentToken: contToken,
	}, nil
}

// pageRoot narrows a page's ytInitialData to a map for the parsers that take one.
func pageRoot(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

// pickCaptionTrack selects the best caption track for a given language code.
// If lang is empty, the first non-auto track is preferred; otherwise the first
// track matching the lang prefix is returned.
func pickCaptionTrack(tracks []CaptionTrack, lang string) *CaptionTrack {
	if len(tracks) == 0 {
		return nil
	}
	if lang != "" {
		for i := range tracks {
			if tracks[i].LanguageCode == lang {
				return &tracks[i]
			}
		}
		// Partial match (e.g. "en" matches "en-US").
		for i := range tracks {
			if len(tracks[i].LanguageCode) >= len(lang) && tracks[i].LanguageCode[:len(lang)] == lang {
				return &tracks[i]
			}
		}
	}
	// Prefer non-auto-generated.
	for i := range tracks {
		if !tracks[i].IsAutoGenerated {
			return &tracks[i]
		}
	}
	return &tracks[0]
}

func joinLines(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += "\n"
		}
		result += p
	}
	return result
}
