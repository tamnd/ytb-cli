package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/ytb-cli/ytb"
)

func newFormatsCmd() kit.Command {
	var (
		audio bool
		video bool
		muxed bool
		urls  bool
	)
	return kit.Command{
		Use:   "formats <video-id|url>",
		Short: "Streaming formats (metadata only)",
		Long: `List the muxed and adaptive formats from /player streamingData, deduped by
itag, audio first then video then muxed. --audio/--video filter by track type,
--muxed shows only progressive formats. This lists metadata only; it does not
resolve playable URLs unless you pass --urls.

The note printed at the end says which client answered and when its URLs expire,
and it says that fetching any of them without a Range header runs at 32 KiB/s.
That last part is the most useful line the command prints: ranged, the same URL
runs at 4 MiB/s.`,
		Args: kit.ExactArgs(1),
		Flags: func(f *kit.FlagSet) {
			f.BoolVar(&audio, "audio", false, "audio-only adaptive formats")
			f.BoolVar(&video, "video", false, "video-only adaptive formats")
			f.BoolVar(&muxed, "muxed", false, "progressive (muxed) formats only")
			f.BoolVar(&urls, "urls", false, "resolve playable stream URLs via the native engine (deciphered)")
		},
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)
			if urls {
				return emitStreamURLs(ctx, app, args[0], audio, video, muxed)
			}
			list, err := app.Client.FormatList(ctx, args[0])
			if err != nil {
				return err
			}
			if list == nil || len(list.Formats) == 0 {
				return noResults("no formats available")
			}
			var n int
			for _, f := range list.Formats {
				if !ytb.FormatMatches(f, audio, video, muxed) {
					continue
				}
				stop, err := app.Emit(formatRow(f))
				if err != nil {
					return err
				}
				n++
				if stop {
					break
				}
			}
			if n == 0 {
				return noResults("no formats matched the filter")
			}
			if err := app.Out.Flush(); err != nil {
				return err
			}
			app.logf("%s", formatsNote(list))
			return nil
		},
	}
}

// formatsNote is the two line note under the table, printed once per read.
//
// It goes to stderr so that -o json stays a clean stream, and it is one note for
// the whole list rather than a column, because the two facts on it are facts about
// the read and not about any one format: which client answered, and when the URLs
// it handed out stop working.
func formatsNote(list *ytb.FormatList) string {
	var b strings.Builder
	b.WriteString("note  fetching any of these without a Range header runs at 32 KiB/s; ytb download\n")
	b.WriteString("      always ranges.")
	if len(list.Client) > 0 {
		fmt.Fprintf(&b, " client %s,", strings.Join(list.Client, "+"))
	}
	switch {
	case !list.Fetchable:
		b.WriteString(" no urls in this list: the mobile player did not answer")
	case !list.Expires.IsZero():
		fmt.Fprintf(&b, " urls expire %s", list.Expires.Format(time.RFC3339))
	default:
		b.WriteString(" urls carry no expiry")
	}
	return b.String()
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
		stop, err := app.Emit(Row{
			Cols: []string{"itag", "ext", "resolution", "url"},
			Vals: []string{fmt.Sprint(s.ITag), s.Ext(), resolutionLabel(s), url},
			Value: struct {
				ytb.Stream
				URL string `json:"url"`
			}{s, url},
		})
		if err != nil {
			return err
		}
		n++
		if stop {
			break
		}
	}
	if n == 0 {
		return noResults("no streams matched the filter")
	}
	return app.Out.Flush()
}

func streamMatches(s ytb.Stream, audio, video, muxed bool) bool {
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
