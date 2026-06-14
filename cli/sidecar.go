package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/ytb-cli/youtube"
)

func newSponsorBlockCmd() kit.Command {
	var categories []string
	return kit.Command{
		Use:   "sponsorblock <id|url>",
		Short: "List community SponsorBlock segments for a video",
		Args:  kit.ExactArgs(1),
		Flags: func(f *kit.FlagSet) {
			f.StringSliceVar(&categories, "categories", nil,
				"segment categories to fetch (default all): sponsor,selfpromo,intro,outro,...")
		},
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)
			videoID := youtube.ExtractVideoID(args[0])
			if videoID == "" {
				videoID = args[0]
			}
			segs, err := app.Client.SponsorSegments(ctx, videoID, categories)
			if err != nil {
				return err
			}
			if len(segs) == 0 {
				return noResults("no SponsorBlock segments")
			}
			for _, s := range segs {
				if err := app.Out.Emit(Row{
					Cols: []string{"category", "start", "end", "action"},
					Vals: []string{
						s.Category,
						fmt.Sprintf("%.1f", s.Start),
						fmt.Sprintf("%.1f", s.End),
						s.Action,
					},
					Value: s,
				}); err != nil {
					return err
				}
			}
			return app.Out.Flush()
		},
	}
}

func newThumbnailCmd() kit.Command {
	var (
		download bool
		out      string
	)
	return kit.Command{
		Use:   "thumbnail <id|url>",
		Short: "List or download a video's thumbnail renditions",
		Args:  kit.ExactArgs(1),
		Flags: func(f *kit.FlagSet) {
			f.BoolVar(&download, "download", false, "download the best available rendition")
			f.StringVar(&out, "out", "", "output path or directory for --download")
		},
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)
			videoID := youtube.ExtractVideoID(args[0])
			if videoID == "" {
				videoID = args[0]
			}
			if download {
				dst := out
				if dst == "" || isDir(dst) {
					dst = filepath.Join(dst, videoID+".jpg")
				}
				if app.dryRun {
					app.logf("would download thumbnail to %s", dst)
					return nil
				}
				t, err := app.Client.DownloadThumbnail(ctx, videoID, dst)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintf(cmdErr, "saved %s (%s, %dx%d)\n", dst, t.Name, t.Width, t.Height)
				return nil
			}
			for _, t := range youtube.Thumbnails(videoID) {
				if err := app.Out.Emit(Row{
					Cols:  []string{"name", "width", "height", "url"},
					Vals:  []string{t.Name, fmt.Sprint(t.Width), fmt.Sprint(t.Height), t.URL},
					Value: t,
				}); err != nil {
					return err
				}
			}
			return app.Out.Flush()
		},
	}
}

func newChaptersCmd() kit.Command {
	return kit.Command{
		Use:   "chapters <id|url>",
		Short: "List a video's chapter markers",
		Args:  kit.ExactArgs(1),
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)
			res, err := app.Client.FetchVideo(ctx, args[0], youtube.VideoOptions{
				Player: true,
				Next:   true,
			})
			if err != nil {
				return err
			}
			if res == nil || len(res.Chapters) == 0 {
				return noResults("no chapters")
			}
			for _, c := range res.Chapters {
				if err := app.Out.Emit(Row{
					Cols:  []string{"position", "start", "title"},
					Vals:  []string{fmt.Sprint(c.Position), hms(c.StartSeconds), c.Title},
					Value: c,
				}); err != nil {
					return err
				}
			}
			return app.Out.Flush()
		},
	}
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// hms formats whole seconds as H:MM:SS or M:SS.
func hms(sec int) string {
	h := sec / 3600
	m := (sec % 3600) / 60
	s := sec % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}
