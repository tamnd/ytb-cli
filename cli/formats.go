package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tamnd/ytb-cli/youtube"
)

func newFormatsCmd(app *App) *cobra.Command {
	var (
		audio bool
		video bool
		muxed bool
		urls  bool
	)
	cmd := &cobra.Command{
		Use:   "formats <video-id|url>",
		Short: "Streaming formats (metadata only)",
		Long: `List the muxed and adaptive formats from /player streamingData, deduped by
itag. --audio/--video filter by track type, --muxed shows only progressive
formats. This lists metadata only; it does not resolve playable URLs.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if urls {
				return emitStreamURLs(ctx, app, args[0], audio, video, muxed)
			}
			store, err := app.Store()
			if err != nil {
				return err
			}
			formats, err := app.Client.Formats(ctx, args[0])
			if err != nil {
				return err
			}
			if len(formats) == 0 {
				return noResults("no formats available")
			}
			var n int
			for _, f := range formats {
				if !formatMatches(f, audio, video, muxed) {
					continue
				}
				if store != nil {
					_ = store.UpsertVideoFormat(f)
				}
				if err := app.Out.Emit(formatRow(f)); err != nil {
					return err
				}
				n++
				if app.Limit > 0 && n >= app.Limit {
					break
				}
			}
			if n == 0 {
				return noResults("no formats matched the filter")
			}
			return app.Out.Flush()
		},
	}
	f := cmd.Flags()
	f.BoolVar(&audio, "audio", false, "audio-only adaptive formats")
	f.BoolVar(&video, "video", false, "video-only adaptive formats")
	f.BoolVar(&muxed, "muxed", false, "progressive (muxed) formats only")
	f.BoolVar(&urls, "urls", false, "resolve playable stream URLs via the native engine (deciphered)")
	return cmd
}

// emitStreamURLs resolves and prints the deciphered, directly-fetchable URL for
// each stream, applying the same audio/video/muxed track filters.
func emitStreamURLs(ctx context.Context, app *App, idOrURL string, audio, video, muxed bool) error {
	m, err := app.Client.StreamManifest(ctx, idOrURL)
	if err != nil {
		return err
	}
	if len(m.Streams) == 0 {
		return noResults("no streams available")
	}
	var n int
	for i := range m.Streams {
		s := m.Streams[i]
		if !streamMatches(s, audio, video, muxed) {
			continue
		}
		url, err := app.Client.ResolveStreamURL(ctx, m, &s)
		if err != nil {
			app.logf("itag %d: %v", s.ITag, err)
			continue
		}
		if err := app.Out.Emit(Row{
			Cols: []string{"itag", "ext", "resolution", "url"},
			Vals: []string{fmt.Sprint(s.ITag), s.Ext(), resolutionLabel(s), url},
			Value: struct {
				youtube.Stream
				URL string `json:"url"`
			}{s, url},
		}); err != nil {
			return err
		}
		n++
		if app.Limit > 0 && n >= app.Limit {
			break
		}
	}
	if n == 0 {
		return noResults("no streams matched the filter")
	}
	return app.Out.Flush()
}

func streamMatches(s youtube.Stream, audio, video, muxed bool) bool {
	switch {
	case muxed:
		return s.Muxed()
	case audio:
		return s.AudioOnly()
	case video:
		return s.VideoOnly()
	default:
		return true
	}
}

func formatMatches(f youtube.VideoFormat, audio, video, muxed bool) bool {
	isAudio := strings.HasPrefix(f.MimeType, "audio/")
	isVideoOnly := f.IsAdaptive && strings.HasPrefix(f.MimeType, "video/")
	switch {
	case muxed:
		return !f.IsAdaptive
	case audio:
		return isAudio
	case video:
		return isVideoOnly
	default:
		return true
	}
}
