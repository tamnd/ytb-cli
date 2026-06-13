package cli

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/tamnd/ytb-cli/youtube"
)

func newFormatsCmd(app *App) *cobra.Command {
	var (
		audio bool
		video bool
		muxed bool
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
	return cmd
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
