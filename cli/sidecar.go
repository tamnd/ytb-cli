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

// newThumbnailCmd lists a video's renditions, or writes one.
//
// The list is HEADed before it is printed, because the five standard i.ytimg.com
// names can be constructed for any video id and only some of them exist. A video
// with no maxresdefault answers that URL with 404 and a 1097 byte body that is
// still Content-Type image/jpeg, so nothing but the status code separates a
// rendition from a placeholder, and a list printed without asking is a list of
// URLs some of which 404 for whoever tries them next. --unconfirmed skips the
// asking and says so in the source column.
func newThumbnailCmd() kit.Command {
	var (
		fetch       bool
		unconfirmed bool
		out         string
	)
	return kit.Command{
		Use:   "thumbnail <id|url>",
		Short: "List a video's thumbnail renditions, or fetch the best one",
		Args:  kit.ExactArgs(1),
		Flags: func(f *kit.FlagSet) {
			f.BoolVar(&fetch, "fetch", false, "write the best available rendition to disk")
			f.BoolVar(&unconfirmed, "unconfirmed", false, "list the constructed URLs without HEADing them")
			f.StringVar(&out, "out", "", "output path or directory for --fetch")
		},
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)
			videoID := youtube.ExtractVideoID(args[0])
			if videoID == "" {
				videoID = args[0]
			}
			if fetch {
				dst := out
				if dst == "" || isDir(dst) {
					dst = filepath.Join(dst, videoID+".jpg")
				}
				if app.dryRun {
					app.logf("would fetch thumbnail to %s", dst)
					return nil
				}
				t, err := app.Client.DownloadThumbnail(ctx, videoID, dst)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintf(cmdErr, "saved %s (%s, %dx%d)\n", dst, t.Name, t.Width, t.Height)
				return nil
			}
			thumbs := youtube.Thumbnails(videoID)
			if !unconfirmed {
				thumbs = app.Client.ConfirmThumbnails(ctx, thumbs)
			}
			if len(thumbs) == 0 {
				return noResults("no thumbnail renditions exist for " + videoID)
			}
			for _, t := range thumbs {
				if err := app.Out.Emit(Row{
					Cols: []string{"name", "width", "height", "size", "source", "url"},
					Vals: []string{
						t.Name, fmt.Sprint(t.Width), fmt.Sprint(t.Height),
						sizeText(t.Bytes), t.Source, t.URL,
					},
					Value: t,
				}); err != nil {
					return err
				}
			}
			return app.Out.Flush()
		},
	}
}

// newChaptersCmd lists a video's chapters with where each one came from.
//
// origin is a column rather than a footnote because a macro marker and a
// timestamped description are two different things that produce the same list.
// markers means YouTube served a macroMarkersListRenderer, so the site itself
// treats these as chapters, shows them on the scrubber and has a preview frame for
// each. description means somebody typed "1:23 Verse 2" and the list is only as
// good as their typing, with no thumbnail and no guarantee the numbers are in
// order. Printing them the same way makes the second look like the first.
func newChaptersCmd() kit.Command {
	return kit.Command{
		Use:   "chapters <id|url>",
		Short: "List a video's chapters, with where each one came from",
		Args:  kit.ExactArgs(1),
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)
			res, err := app.Client.FetchVideo(ctx, args[0], youtube.VideoOptions{Next: true})
			if err != nil {
				return err
			}
			if res == nil || len(res.Chapters) == 0 {
				return noResults("no chapters")
			}
			for _, c := range res.Chapters {
				if err := app.Out.Emit(Row{
					Cols:  []string{"position", "start", "title", "origin"},
					Vals:  []string{fmt.Sprint(c.Position), hms(c.StartSeconds), c.Title, c.Origin},
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
