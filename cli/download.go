package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/ytb-cli/youtube"
)

// resolveYtDlp returns the yt-dlp binary path or a coded error (exit 7) if absent.
func (a *App) resolveYtDlp() (string, error) {
	bin := a.YtDlpBin
	if bin == "" {
		bin = os.Getenv("YTB_YT_DLP_BIN")
	}
	if bin == "" {
		bin = "yt-dlp"
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		return "", missingTool("yt-dlp not found on PATH; install it or pass --yt-dlp-bin")
	}
	return path, nil
}

// runYtDlp prints the ToS note, then execs yt-dlp with args, streaming its output.
func (a *App) runYtDlp(ctx context.Context, args []string) error {
	bin, err := a.resolveYtDlp()
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(cmdErr, "note: media download is your responsibility; respect YouTube's Terms of Service and copyright.")
	if a.dryRun {
		_, _ = fmt.Fprintf(cmdErr, "would run: %s %v\n", bin, args)
		return nil
	}
	c := exec.CommandContext(ctx, bin, args...)
	c.Stdout = cmdOut
	c.Stderr = cmdErr
	c.Stdin = os.Stdin
	if err := c.Run(); err != nil {
		return fmt.Errorf("yt-dlp: %w", err)
	}
	return nil
}

// downloadOpts collects the native downloader flags.
type downloadOpts struct {
	audio       bool
	audioFormat string
	out         string
	tmpl        string
	format      string
	quality     string
	items       string
	subLangs    string
	subFormat   string
	writeSubs   bool
	concurrency int
	embedThumb  bool
	archivePath string
	useYtDlp    bool
}

func newDownloadCmd() kit.Command {
	var o downloadOpts
	return kit.Command{
		Use:   "download <id|url>...",
		Short: "Download media with the native engine (or yt-dlp via --use-yt-dlp)",
		Long: `Download videos with the built-in pure-Go engine.

The native engine fetches streams through the ANDROID_VR client (no API key,
no token), deciphers signatures and the throttling parameter, and downloads in
parallel byte ranges. Merging separate video+audio tracks, audio conversion,
and thumbnail embedding use ffmpeg when it is available; without ffmpeg the
engine still downloads any single progressive or adaptive stream.

Pass --use-yt-dlp to delegate to a yt-dlp binary instead.`,
		Args: kit.MinimumNArgs(1),
		Flags: func(f *kit.FlagSet) {
			f.BoolVarP(&o.audio, "audio", "x", false, "download audio only")
			f.StringVar(&o.audioFormat, "audio-format", "", "convert audio to this codec (mp3|m4a|opus|flac), needs ffmpeg")
			f.StringVar(&o.out, "out", ".", "output directory")
			f.StringVar(&o.tmpl, "output-template", "%(title)s [%(id)s].%(ext)s", "yt-dlp-style output filename template")
			f.StringVarP(&o.format, "format", "f", "", "format selector (e.g. best, 22, bv*+ba, bv[height<=720]+ba)")
			f.StringVar(&o.quality, "quality", "", "max video height shorthand (e.g. 1080)")
			f.StringVar(&o.items, "playlist-items", "", "playlist item selection (e.g. 1,3,5-7,10-)")
			f.StringVar(&o.subLangs, "sub-langs", "", "subtitle language to write (e.g. en)")
			f.StringVar(&o.subFormat, "sub-format", "srt", "subtitle format to write (srt|vtt|txt)")
			f.BoolVar(&o.writeSubs, "write-subs", false, "write the subtitle sidecar file")
			f.IntVar(&o.concurrency, "concurrent-fragments", 4, "parallel byte-range workers")
			f.BoolVar(&o.embedThumb, "embed-thumbnail", false, "embed the thumbnail as cover art (mp4/m4a, needs ffmpeg)")
			f.StringVar(&o.archivePath, "download-archive", "", "record downloaded ids here and skip ones already present")
			f.BoolVar(&o.useYtDlp, "use-yt-dlp", false, "delegate to a yt-dlp binary instead of the native engine")
		},
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)
			if o.useYtDlp {
				return app.runYtDlpDownload(ctx, o, args)
			}
			return app.runNativeDownload(ctx, o, args)
		},
	}
}

// runNativeDownload expands inputs (videos and playlists) and downloads each.
func (a *App) runNativeDownload(ctx context.Context, o downloadOpts, args []string) error {
	_, _ = fmt.Fprintln(cmdErr, "note: media download is your responsibility; respect YouTube's Terms of Service and copyright.")

	archive, err := youtube.OpenArchive(o.archivePath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	sel, err := youtube.ParseItemSelector(o.items)
	if err != nil {
		return usageErr(err.Error())
	}

	var targets []downloadTarget
	for _, arg := range args {
		if pid := youtube.ExtractPlaylistID(arg); pid != "" && youtube.ExtractVideoID(arg) == "" {
			items, err := a.expandPlaylist(ctx, arg, sel)
			if err != nil {
				return err
			}
			targets = append(targets, items...)
			continue
		}
		targets = append(targets, downloadTarget{idOrURL: arg})
	}

	var failures int
	for _, t := range targets {
		if vid := youtube.ExtractVideoID(t.idOrURL); vid != "" && archive.Has(vid) {
			a.logf("skip %s: already in archive", vid)
			continue
		}
		if err := a.downloadOne(ctx, o, t, archive); err != nil {
			failures++
			_, _ = fmt.Fprintf(cmdErr, "error: %s: %v\n", t.idOrURL, err)
		}
	}
	if failures > 0 {
		return partialErr(fmt.Sprintf("%d of %d downloads failed", failures, len(targets)))
	}
	return nil
}

type downloadTarget struct {
	idOrURL       string
	playlistTitle string
	playlistIndex int
}

func (a *App) expandPlaylist(ctx context.Context, arg string, sel *youtube.ItemSelector) ([]downloadTarget, error) {
	pl, err := a.Client.FetchPlaylist(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("fetch playlist: %w", err)
	}
	var out []downloadTarget
	index := 0
	opt := a.PageOptions(false)
	err = a.Client.StreamPlaylistItems(ctx, arg, opt, func(pv youtube.PlaylistVideo, _ youtube.Video) error {
		index++
		if !sel.Selects(index, pl.VideoCount) {
			return nil
		}
		out = append(out, downloadTarget{
			idOrURL:       pv.VideoID,
			playlistTitle: pl.Title,
			playlistIndex: index,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list playlist items: %w", err)
	}
	return out, nil
}

// downloadOne resolves, selects, downloads, and post-processes a single video.
func (a *App) downloadOne(ctx context.Context, o downloadOpts, t downloadTarget, archive *youtube.DownloadArchive) error {
	manifest, err := a.Client.StreamManifest(ctx, t.idOrURL)
	if err != nil {
		return err
	}
	if len(manifest.Streams) == 0 {
		return noResults("no downloadable streams")
	}

	ffmpeg := youtube.FFmpeg(a.FFmpegBin)
	spec := a.formatSpec(o, ffmpeg != "")
	selection, err := youtube.SelectFormat(manifest.Streams, spec)
	if err != nil {
		return err
	}
	if selection.NeedsMerge() && ffmpeg == "" {
		return missingTool(youtube.ErrFFmpegMissing.Error())
	}

	fields := youtube.OutputFields{
		ID:            manifest.VideoID,
		Title:         manifest.Title,
		Author:        manifest.Author,
		PlaylistTitle: t.playlistTitle,
		PlaylistIndex: t.playlistIndex,
		Duration:      manifest.Duration,
	}

	if a.dryRun {
		for _, s := range selection.Streams() {
			a.logf("would download itag %d (%s %s) for %s", s.ITag, s.QualityLabel, s.Ext(), manifest.VideoID)
		}
		return nil
	}

	finalPath, err := a.fetchAndAssemble(ctx, o, manifest, selection, fields, ffmpeg)
	if err != nil {
		return err
	}

	if o.embedThumb {
		if err := a.embedThumbnail(ctx, manifest.VideoID, finalPath, ffmpeg); err != nil {
			a.logf("embed-thumbnail: %v", err)
		}
	}
	_, _ = fmt.Fprintf(cmdErr, "saved %s\n", finalPath)

	if o.writeSubs && o.subLangs != "" {
		if err := a.writeSubtitle(ctx, manifest.VideoID, o, fields, finalPath); err != nil {
			a.logf("subtitles: %v", err)
		}
	}
	if vid := youtube.ExtractVideoID(t.idOrURL); vid != "" {
		_ = archive.Add(vid)
	} else {
		_ = archive.Add(manifest.VideoID)
	}
	return nil
}

// formatSpec picks the effective selector, honoring --audio, --quality, an
// explicit -f, and whether ffmpeg is present to merge adaptive tracks.
func (a *App) formatSpec(o downloadOpts, haveFFmpeg bool) string {
	if o.format != "" {
		return o.format
	}
	if o.audio {
		return "bestaudio/best"
	}
	if o.quality != "" {
		if haveFFmpeg {
			return fmt.Sprintf("bv*[height<=%s]+ba/b[height<=%s]/b", o.quality, o.quality)
		}
		return fmt.Sprintf("b[height<=%s]/b", o.quality)
	}
	if haveFFmpeg {
		return "bv*+ba/b"
	}
	return "b"
}

// fetchAndAssemble downloads the selected streams to the output directory and,
// when two adaptive tracks were chosen, merges them with ffmpeg.
func (a *App) fetchAndAssemble(ctx context.Context, o downloadOpts, m *youtube.StreamManifest, sel youtube.Selection, fields youtube.OutputFields, ffmpeg string) (string, error) {
	if err := os.MkdirAll(o.out, 0o755); err != nil {
		return "", err
	}

	if !sel.NeedsMerge() {
		s := sel.Streams()[0]
		ext := s.Ext()
		if o.audio && o.audioFormat != "" {
			ext = o.audioFormat
		}
		fields.Ext = ext
		fields.Resolution = resolutionLabel(s)
		dst := filepath.Join(o.out, youtube.RenderOutputTemplate(o.tmpl, fields))

		if o.audio && o.audioFormat != "" {
			raw := dst + ".src"
			if err := a.downloadStream(ctx, o, m, &s, raw, fields.Title); err != nil {
				return "", err
			}
			defer func() { _ = os.Remove(raw) }()
			if err := youtube.ExtractAudio(ctx, ffmpeg, raw, dst, o.audioFormat, ""); err != nil {
				return "", err
			}
			return dst, nil
		}
		if err := a.downloadStream(ctx, o, m, &s, dst, fields.Title); err != nil {
			return "", err
		}
		return dst, nil
	}

	v, au := sel.Video, sel.Audio
	fields.Resolution = resolutionLabel(*v)
	container := mergeContainer(*v)
	fields.Ext = container
	dst := filepath.Join(o.out, youtube.RenderOutputTemplate(o.tmpl, fields))

	vpath := dst + ".video"
	apath := dst + ".audio"
	defer func() { _ = os.Remove(vpath); _ = os.Remove(apath) }()
	if err := a.downloadStream(ctx, o, m, v, vpath, fields.Title+" (video)"); err != nil {
		return "", err
	}
	if err := a.downloadStream(ctx, o, m, au, apath, fields.Title+" (audio)"); err != nil {
		return "", err
	}
	if err := youtube.MergeAV(ctx, ffmpeg, vpath, apath, dst); err != nil {
		return "", fmt.Errorf("merge: %w", err)
	}
	return dst, nil
}

// downloadStream resolves a stream's URL and downloads it with a progress bar.
func (a *App) downloadStream(ctx context.Context, o downloadOpts, m *youtube.StreamManifest, s *youtube.Stream, dst, label string) error {
	url, err := a.Client.ResolveStreamURL(ctx, m, s)
	if err != nil {
		return err
	}
	prog := a.progressReporter(label)
	return a.Client.DownloadToFile(ctx, url, dst, s.ContentLength, o.concurrency, prog)
}

// progressReporter returns a throttled stderr progress callback, or nil when
// output is quiet.
func (a *App) progressReporter(label string) func(youtube.DownloadProgress) {
	if a.quiet {
		return nil
	}
	lastPct := -1
	return func(p youtube.DownloadProgress) {
		if p.Total <= 0 {
			return
		}
		pct := int(p.Downloaded * 100 / p.Total)
		if pct == lastPct {
			return
		}
		lastPct = pct
		_, _ = fmt.Fprintf(cmdErr, "\r%s: %3d%% (%s/%s)", label, pct,
			humanBytes(p.Downloaded), humanBytes(p.Total))
		if pct >= 100 {
			_, _ = fmt.Fprintln(cmdErr)
		}
	}
}

// embedThumbnail downloads the video thumbnail and muxes it into the media file
// as cover art (mp4/m4a only), replacing the file in place.
func (a *App) embedThumbnail(ctx context.Context, videoID, mediaPath, ffmpeg string) error {
	ext := strings.ToLower(filepath.Ext(mediaPath))
	if ext != ".mp4" && ext != ".m4a" {
		return fmt.Errorf("cover art only supported for mp4/m4a, not %s", ext)
	}
	thumb := mediaPath + ".thumb.jpg"
	if _, err := a.Client.DownloadThumbnail(ctx, videoID, thumb); err != nil {
		return err
	}
	defer func() { _ = os.Remove(thumb) }()
	tmp := mediaPath + ".thumbed" + ext
	if err := youtube.EmbedThumbnail(ctx, ffmpeg, mediaPath, thumb, tmp); err != nil {
		return err
	}
	return os.Rename(tmp, mediaPath)
}

func (a *App) writeSubtitle(ctx context.Context, videoID string, o downloadOpts, fields youtube.OutputFields, mediaPath string) error {
	_, segs, err := a.Client.Transcript(ctx, videoID, o.subLangs)
	if err != nil {
		return err
	}
	if len(segs) == 0 {
		return fmt.Errorf("no subtitle segments for %q", o.subLangs)
	}
	out := youtube.RenderSubtitles(segs, youtube.SubtitleFormat(o.subFormat))
	base := strings.TrimSuffix(mediaPath, filepath.Ext(mediaPath))
	subPath := fmt.Sprintf("%s.%s.%s", base, o.subLangs, o.subFormat)
	if err := os.WriteFile(subPath, []byte(out), 0o644); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(cmdErr, "saved %s\n", subPath)
	return nil
}

// runYtDlpDownload preserves the original yt-dlp delegation path.
func (a *App) runYtDlpDownload(ctx context.Context, o downloadOpts, args []string) error {
	var ytArgs []string
	if o.audio {
		ytArgs = append(ytArgs, "-x")
		if o.audioFormat != "" {
			ytArgs = append(ytArgs, "--audio-format", o.audioFormat)
		}
	}
	if o.format != "" {
		ytArgs = append(ytArgs, "-f", o.format)
	} else if o.quality != "" {
		ytArgs = append(ytArgs, "-f", fmt.Sprintf("bv*[height<=%s]+ba/b[height<=%s]", o.quality, o.quality))
	}
	if o.out != "" {
		ytArgs = append(ytArgs, "-o", filepath.Join(o.out, o.tmpl))
	}
	if o.items != "" {
		ytArgs = append(ytArgs, "--playlist-items", o.items)
	}
	if o.subLangs != "" {
		ytArgs = append(ytArgs, "--sub-langs", o.subLangs, "--write-subs")
	}
	if o.concurrency > 0 {
		ytArgs = append(ytArgs, "--concurrent-fragments", fmt.Sprint(o.concurrency))
	}
	if o.embedThumb {
		ytArgs = append(ytArgs, "--embed-thumbnail")
	}
	if o.archivePath != "" {
		ytArgs = append(ytArgs, "--download-archive", o.archivePath)
	}
	ytArgs = append(ytArgs, args...)
	return a.runYtDlp(ctx, ytArgs)
}

func newExtractCmd() kit.Command {
	var (
		out     string
		format  string
		quality string
	)
	return kit.Command{
		Use:   "extract <audio|video|transcript|all> <id|url>",
		Short: "Extract a specific stream via yt-dlp",
		Args:  kit.ExactArgs(2),
		Flags: func(f *kit.FlagSet) {
			f.StringVar(&out, "out", ".", "output directory")
			f.StringVar(&format, "format", "", "audio format for extract audio (e.g. mp3)")
			f.StringVar(&quality, "quality", "", "max video height for extract video (e.g. 1080)")
		},
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)
			kind, target := args[0], args[1]
			var ytArgs []string
			switch kind {
			case "audio":
				ytArgs = append(ytArgs, "-x")
				if format != "" {
					ytArgs = append(ytArgs, "--audio-format", format)
				}
			case "video":
				sel := "bestvideo+bestaudio/best"
				if quality != "" {
					sel = fmt.Sprintf("bestvideo[height<=%s]+bestaudio/best[height<=%s]", quality, quality)
				}
				ytArgs = append(ytArgs, "-f", sel)
			case "transcript":
				ytArgs = append(ytArgs, "--write-auto-subs", "--write-subs", "--skip-download")
			case "all":
				ytArgs = append(ytArgs, "--write-subs", "--write-auto-subs", "--add-metadata")
			default:
				return usageErr("extract kind must be audio|video|transcript|all")
			}
			if out != "" {
				ytArgs = append(ytArgs, "-o", out+"/%(title)s [%(id)s].%(ext)s")
			}
			ytArgs = append(ytArgs, target)
			return app.runYtDlp(ctx, ytArgs)
		},
	}
}

func resolutionLabel(s youtube.Stream) string {
	if s.QualityLabel != "" {
		return s.QualityLabel
	}
	if s.Height > 0 {
		return fmt.Sprintf("%dp", s.Height)
	}
	return s.Quality
}

// mergeContainer picks an output container for a merged file: mp4 unless the
// video track is webm/VP9/AV1, which lives more naturally in mkv/webm.
func mergeContainer(v youtube.Stream) string {
	if v.Container == "webm" {
		return "webm"
	}
	return "mp4"
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(n)/float64(div), "KMGTPE"[exp])
}
