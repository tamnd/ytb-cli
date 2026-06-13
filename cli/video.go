package cli

import (
	"github.com/spf13/cobra"
	"github.com/tamnd/ytb-cli/youtube"
)

func newVideoCmd(app *App) *cobra.Command {
	var (
		transcript bool
		lang       string
		chapters   bool
		related    bool
		captions   bool
		formats    bool
		noPlayer   bool
		raw        bool
	)
	cmd := &cobra.Command{
		Use:   "video <id|url>...",
		Short: "Resolve one or more videos to full metadata",
		Long: `Resolve a video to its full metadata in one shot: HTML bootstrap plus
/player (description, dates, category, tags, formats, captions) plus /next
(chapters, related, comment token). Pass "-" to read ids/urls from stdin.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := argsOrStdin(args)
			if err != nil {
				return err
			}
			if len(ids) == 0 {
				return noResults("no video ids given")
			}
			opt := youtube.VideoOptions{
				Player:     !noPlayer,
				Next:       !noPlayer && (chapters || related || true),
				Transcript: transcript,
				Lang:       lang,
			}
			store, err := app.Store()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			var failed int
			for _, id := range ids {
				res, err := app.Client.FetchVideo(ctx, id, opt)
				if err != nil {
					app.logf("video %s: %v", id, err)
					failed++
					continue
				}
				if res == nil {
					app.logf("video %s: not found", id)
					failed++
					continue
				}
				persistVideoResult(store, res)
				if raw {
					_ = app.Out.Emit(Row{Cols: []string{"id"}, Vals: []string{res.Video.VideoID}, Value: res})
				} else {
					_ = app.Out.Emit(videoRow(res.Video))
				}
				if err := emitVideoExtras(app, res, chapters, related, captions, formats, transcript); err != nil {
					return err
				}
			}
			if err := app.Out.Flush(); err != nil {
				return err
			}
			if failed > 0 && failed == len(ids) {
				return noResults("no videos resolved")
			}
			if failed > 0 {
				return partialErr("some videos failed to resolve")
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.BoolVar(&transcript, "transcript", false, "fetch and attach the transcript text")
	f.StringVar(&lang, "lang", "", "preferred caption language (default auto/English)")
	f.BoolVar(&chapters, "chapters", false, "list chapters")
	f.BoolVar(&related, "related", false, "list related videos")
	f.BoolVar(&captions, "captions", false, "list available caption tracks")
	f.BoolVar(&formats, "formats", false, "list streaming formats")
	f.BoolVar(&noPlayer, "no-player", false, "skip /player (HTML-only, faster)")
	f.BoolVar(&raw, "raw", false, "emit the full VideoResult as the value")
	return cmd
}

func emitVideoExtras(app *App, res *youtube.VideoResult, chapters, related, captions, formats, transcript bool) error {
	if chapters {
		for _, c := range res.Chapters {
			if err := app.Out.Emit(chapterRow(c)); err != nil {
				return err
			}
		}
	}
	if related {
		for _, r := range res.Related {
			if err := app.Out.Emit(relatedRow(r)); err != nil {
				return err
			}
		}
	}
	if captions {
		for _, t := range res.Captions {
			if err := app.Out.Emit(captionRow(t)); err != nil {
				return err
			}
		}
	}
	if formats {
		for _, fm := range res.Formats {
			if err := app.Out.Emit(formatRow(fm)); err != nil {
				return err
			}
		}
	}
	if transcript && res.Video.Transcript != "" {
		_ = app.Out.Line(res.Video.Transcript)
	}
	return nil
}

func persistVideoResult(store *youtube.Store, res *youtube.VideoResult) {
	if store == nil || res == nil {
		return
	}
	_ = store.UpsertVideo(res.Video)
	for _, f := range res.Formats {
		_ = store.UpsertVideoFormat(f)
	}
	for _, t := range res.Captions {
		_ = store.UpsertCaptionTrack(t)
	}
	for _, c := range res.Chapters {
		_ = store.UpsertChapter(c)
	}
	for _, r := range res.Related {
		_ = store.UpsertRelatedVideo(r)
	}
}
