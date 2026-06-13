package youtube

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

// ErrFFmpegMissing is returned when an operation needs ffmpeg but none was found.
// Callers map it to the missing-tool exit code.
var ErrFFmpegMissing = errors.New("ffmpeg not found: install it or pass --ffmpeg-bin (the ytb binary itself stays pure-Go)")

// FFmpeg locates the ffmpeg binary. The explicit path wins, then YTB_FFMPEG_BIN,
// then PATH. It returns "" when none is usable.
func FFmpeg(explicit string) string {
	candidates := []string{explicit, os.Getenv("YTB_FFMPEG_BIN")}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p
	}
	return ""
}

// MergeAV muxes a video-only and audio-only file into one container, copying
// both streams without re-encoding. The output container is taken from out's
// extension.
func MergeAV(ctx context.Context, ffmpeg, videoPath, audioPath, out string) error {
	if ffmpeg == "" {
		return ErrFFmpegMissing
	}
	args := []string{
		"-y",
		"-i", videoPath,
		"-i", audioPath,
		"-c", "copy",
		"-map", "0:v:0",
		"-map", "1:a:0",
	}
	if filepath.Ext(out) == ".mp4" {
		args = append(args, "-movflags", "+faststart")
	}
	args = append(args, out)
	return runFFmpeg(ctx, ffmpeg, args)
}

// ExtractAudio transcodes (or copies) an input into an audio-only file. When
// codec is "copy" the stream is remuxed; otherwise it is re-encoded (e.g. mp3,
// aac, opus, flac). quality, when non-empty, sets -q:a or -b:a as appropriate.
func ExtractAudio(ctx context.Context, ffmpeg, in, out, codec, quality string) error {
	if ffmpeg == "" {
		return ErrFFmpegMissing
	}
	args := []string{"-y", "-i", in, "-vn"}
	switch codec {
	case "", "copy":
		args = append(args, "-c:a", "copy")
	default:
		args = append(args, "-c:a", audioEncoder(codec))
		if quality != "" {
			args = append(args, "-b:a", quality)
		}
	}
	args = append(args, out)
	return runFFmpeg(ctx, ffmpeg, args)
}

// audioEncoder maps a friendly codec name to ffmpeg's encoder name.
func audioEncoder(codec string) string {
	switch codec {
	case "mp3":
		return "libmp3lame"
	case "aac", "m4a":
		return "aac"
	case "opus":
		return "libopus"
	case "vorbis", "ogg":
		return "libvorbis"
	default:
		return codec
	}
}

// EmbedThumbnail attaches an image to a media file as cover art (mp4/m4a only).
func EmbedThumbnail(ctx context.Context, ffmpeg, media, image, out string) error {
	if ffmpeg == "" {
		return ErrFFmpegMissing
	}
	args := []string{
		"-y", "-i", media, "-i", image,
		"-map", "0", "-map", "1",
		"-c", "copy",
		"-disposition:v:1", "attached_pic",
		out,
	}
	return runFFmpeg(ctx, ffmpeg, args)
}

func runFFmpeg(ctx context.Context, ffmpeg string, args []string) error {
	// Keep ffmpeg quiet (errors only) so its banner and per-frame progress do
	// not drown out the CLI's own output.
	full := append([]string{"-loglevel", "error", "-nostdin"}, args...)
	cmd := exec.CommandContext(ctx, ffmpeg, full...)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
