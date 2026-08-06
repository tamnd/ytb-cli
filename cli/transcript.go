package cli

import (
	"context"
	"os"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/ytb-cli/pkg/srv3"
	"github.com/tamnd/ytb-cli/youtube"
)

// transcript.go is the text read. Doc 05 section 4.
//
// This used to shell out to yt-dlp, which shelled out to Deno, for text that
// arrives in one request. The reason it did is worth keeping written down: the
// watch page lists caption tracks whose baseUrl answers HTTP 200 with zero
// bytes, forever, so the read looked gated when it was only pointed at the wrong
// client. A baseUrl off the ANDROID player returns the file. There is no
// external process here now and no PATH lookup.

func newTranscriptCmd() kit.Command {
	var (
		lang      string
		auto      bool
		translate string
		format    string
		out       string
	)
	return kit.Command{
		Use:   "transcript <video-id|url>",
		Short: "A video's captions as text, srt, vtt or json",
		Long: `Fetch one caption track and write it out.

--format picks the serialization: text (the default), srt, vtt, or json. All
four come off one parse, so the timings in the srt and the json are the same
timings.

--lang picks the track by language code, and "en" will match "en-GB" when there
is no plain "en". With no --lang the human track wins over the auto-generated
one, because a person's punctuation is worth more than a machine's word list.
--auto asks for the auto track even when a human one exists; it is the only one
that carries per-word timings, which json keeps.

--translate asks YouTube to machine-translate the chosen track into a language
code. The translation is the site's own.

Use "ytb captions" to see what a video has.`,
		Args: kit.ExactArgs(1),
		Flags: func(f *kit.FlagSet) {
			f.StringVar(&lang, "lang", "", "caption language code")
			f.BoolVar(&auto, "auto", false, "take the auto-generated track")
			f.StringVar(&translate, "translate", "", "machine-translate into this language code")
			f.StringVar(&format, "format", "text", "text|srt|vtt|json")
			f.StringVar(&out, "out", "", "write to this file instead of stdout")
		},
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)
			// Checked before the fetch, so a typo costs no request and exits 2 rather
			// than arriving as a plain error after the transcript is already in hand.
			if !srv3.Format(format).Valid() {
				return usageErr("unknown --format " + format + ": want text, srt, vtt or json")
			}
			doc, _, err := app.Client.Transcript(ctx, args[0], youtube.TranscriptOptions{
				Lang: lang, Auto: auto, TranslateTo: translate,
			})
			if err != nil {
				// This command holds the client itself, so nothing has classified the
				// error yet. Without this a video with no captions exits 1.
				return youtube.ExitError(err)
			}
			if len(doc.Cues) == 0 {
				return noResults("the track parsed to no lines")
			}
			rendered, err := doc.Render(srv3.Format(format))
			if err != nil {
				return err
			}
			if out != "" {
				if err := os.WriteFile(out, []byte(rendered), 0o644); err != nil {
					return err
				}
				_, _ = cmdErr.Write([]byte("saved " + out + "\n"))
				return nil
			}
			return app.Line(rendered)
		},
	}
}

func newCaptionsCmd() kit.Command {
	return kit.Command{
		Use:   "captions <video-id|url>",
		Short: "List a video's caption tracks",
		Long: `List the caption tracks a video has, one record each.

The tracks come off the ANDROID player. The watch page lists the same tracks
with URLs that answer HTTP 200 and an empty body, so they are not listed here.

vss_id is the track's own name for itself: ".en" is the human English track and
"a.en" is the machine one, which is what tells two same-language tracks apart.`,
		Args: kit.ExactArgs(1),
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)
			tracks, err := app.Client.Captions(ctx, args[0])
			if err != nil {
				return youtube.ExitError(err)
			}
			if len(tracks) == 0 {
				return noResults("this video has no caption track")
			}
			for _, t := range tracks {
				if err := app.Out.Emit(captionRow(t)); err != nil {
					return err
				}
			}
			return app.Out.Flush()
		},
	}
}
