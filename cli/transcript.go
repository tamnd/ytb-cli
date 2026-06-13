package cli

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tamnd/ytb-cli/youtube"
)

func newTranscriptCmd(app *App) *cobra.Command {
	var (
		lang       string
		list       bool
		timestamps bool
	)
	cmd := &cobra.Command{
		Use:   "transcript <video-id|url>",
		Short: "Captions as text",
		Long: `List caption tracks (--list), or fetch the chosen track's timed text and print
joined text (or --timestamps for {start, dur, text} segments). --lang picks the
language; auto-generated tracks are marked.

YouTube now gates the raw caption endpoints behind a proof-of-origin token, so
direct text fetches often come back empty. When that happens and yt-dlp is on
PATH, the transcript is recovered through it automatically.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			store, err := app.Store()
			if err != nil {
				return err
			}
			if list {
				tracks, err := app.Client.Captions(ctx, args[0])
				if err != nil {
					return err
				}
				if len(tracks) == 0 {
					return noResults("no caption tracks")
				}
				for _, t := range tracks {
					if store != nil {
						_ = store.UpsertCaptionTrack(t)
					}
					if err := app.Out.Emit(captionRow(t)); err != nil {
						return err
					}
				}
				return app.Out.Flush()
			}

			text, segments, err := app.Client.Transcript(ctx, args[0], lang)
			if err != nil {
				return err
			}
			if text == "" && len(segments) == 0 {
				// The direct endpoint was gated (empty body). Recover via yt-dlp.
				segments, err = app.transcriptViaYtDlp(cmd, args[0], lang)
				if err != nil {
					return err
				}
				text = joinSegmentText(segments)
			}
			if text == "" && len(segments) == 0 {
				return noResults("no transcript available")
			}
			if timestamps {
				for _, s := range segments {
					if err := app.Out.Emit(segmentRow(s)); err != nil {
						return err
					}
				}
				return app.Out.Flush()
			}
			return app.Out.Line(text)
		},
	}
	f := cmd.Flags()
	f.StringVar(&lang, "lang", "", "preferred caption language")
	f.BoolVar(&list, "list", false, "list available caption tracks")
	f.BoolVar(&timestamps, "timestamps", false, "emit timed segments instead of joined text")
	return cmd
}

// transcriptViaYtDlp recovers a transcript through yt-dlp's subtitle writer,
// which negotiates the proof-of-origin token the bare endpoints now require.
func (a *App) transcriptViaYtDlp(cmd *cobra.Command, target, lang string) ([]youtube.TranscriptSegment, error) {
	bin, err := a.resolveYtDlp()
	if err != nil {
		return nil, missingTool("transcript endpoint is gated and yt-dlp is not on PATH; install yt-dlp or pass --yt-dlp-bin to recover captions")
	}
	dir, err := os.MkdirTemp("", "youtube-transcript-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	subLangs := lang
	if subLangs == "" {
		subLangs = "en.*,en"
	}
	ytArgs := []string{
		"--skip-download",
		"--write-subs", "--write-auto-subs",
		"--sub-format", "vtt",
		"--sub-langs", subLangs,
		"-o", filepath.Join(dir, "%(id)s.%(ext)s"),
		target,
	}
	if a.dryRun {
		a.logf("would run: %s %v", bin, ytArgs)
		return nil, nil
	}
	c := exec.CommandContext(cmd.Context(), bin, ytArgs...)
	c.Stdout, c.Stderr = cmdErr, cmdErr // yt-dlp progress goes to stderr, never stdout
	// yt-dlp exits non-zero when any one of the requested language variants
	// fails, even if the track we want downloaded fine. Treat the presence of a
	// usable .vtt as success and only surface the error when nothing landed.
	runErr := c.Run()

	matches, _ := filepath.Glob(filepath.Join(dir, "*.vtt"))
	vtt := pickVTT(matches, lang)
	if vtt == "" {
		if runErr != nil {
			return nil, runErr
		}
		return nil, nil
	}
	f, err := os.Open(vtt)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return parseVTT(f), nil
}

// pickVTT chooses the subtitle file best matching lang. With no lang, it
// prefers the plainest track (shortest language tag, e.g. "en" over the
// auto-translated "en-de-DE") so the result is the original captions.
func pickVTT(paths []string, lang string) string {
	if len(paths) == 0 {
		return ""
	}
	if lang != "" {
		for _, p := range paths {
			if strings.Contains(filepath.Base(p), "."+lang) {
				return p
			}
		}
	}
	best := paths[0]
	for _, p := range paths[1:] {
		if len(vttLangTag(p)) < len(vttLangTag(best)) {
			best = p
		}
	}
	return best
}

// vttLangTag returns the language segment of a "<id>.<lang>.vtt" filename.
func vttLangTag(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), ".vtt")
	if i := strings.LastIndex(base, "."); i >= 0 {
		return base[i+1:]
	}
	return base
}

// parseVTT turns a WebVTT cue stream into timed segments, dropping the styling
// tags and consecutive duplicate lines that auto-captions emit for rollup.
func parseVTT(r interface{ Read([]byte) (int, error) }) []youtube.TranscriptSegment {
	var segs []youtube.TranscriptSegment
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var start, dur float64
	var inCue bool
	var text []string
	var lastText string
	flush := func() {
		if !inCue {
			return
		}
		joined := vttClean(strings.Join(text, " "))
		if joined != "" && joined != lastText {
			segs = append(segs, youtube.TranscriptSegment{StartSeconds: start, DurSeconds: dur, Text: joined})
			lastText = joined
		}
		inCue = false
		text = text[:0]
	}
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.Contains(line, "-->") {
			flush()
			s, e := parseVTTTimes(line)
			start, dur = s, e-s
			inCue = true
			continue
		}
		if line == "" {
			flush()
			continue
		}
		if inCue {
			text = append(text, line)
		}
	}
	flush()
	return segs
}

func parseVTTTimes(line string) (start, end float64) {
	parts := strings.SplitN(line, "-->", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	start = vttStamp(strings.TrimSpace(parts[0]))
	rhs := strings.Fields(strings.TrimSpace(parts[1]))
	if len(rhs) > 0 {
		end = vttStamp(rhs[0])
	}
	return start, end
}

// vttStamp parses HH:MM:SS.mmm or MM:SS.mmm into seconds.
func vttStamp(s string) float64 {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ":")
	var secs float64
	for _, p := range parts {
		v, _ := strconv.ParseFloat(p, 64)
		secs = secs*60 + v
	}
	return secs
}

// vttClean strips inline VTT tags (<00:00:00.000>, <c>, &nbsp;) and collapses space.
func vttClean(s string) string {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch {
		case r == '<':
			depth++
		case r == '>' && depth > 0:
			depth--
		case depth == 0:
			b.WriteRune(r)
		}
	}
	out := strings.ReplaceAll(b.String(), " ", " ")
	return strings.Join(strings.Fields(out), " ")
}

func joinSegmentText(segs []youtube.TranscriptSegment) string {
	var lines []string
	for _, s := range segs {
		if t := strings.TrimSpace(s.Text); t != "" {
			lines = append(lines, t)
		}
	}
	return strings.Join(lines, "\n")
}
