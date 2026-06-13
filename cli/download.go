package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// resolveYtDlp returns the yt-dlp binary path or a coded error (exit 6) if absent.
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
func (a *App) runYtDlp(cmd *cobra.Command, args []string) error {
	bin, err := a.resolveYtDlp()
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(cmdErr, "note: media download is your responsibility; respect YouTube's Terms of Service and copyright.")
	if a.dryRun {
		_, _ = fmt.Fprintf(cmdErr, "would run: %s %v\n", bin, args)
		return nil
	}
	c := exec.CommandContext(cmd.Context(), bin, args...)
	c.Stdout = cmdOut
	c.Stderr = cmdErr
	c.Stdin = os.Stdin
	if err := c.Run(); err != nil {
		return fmt.Errorf("yt-dlp: %w", err)
	}
	return nil
}

func newDownloadCmd(app *App) *cobra.Command {
	var (
		audio    bool
		out      string
		format   string
		quality  string
		items    string
		subLangs string
		concurr  int
		metadata bool
	)
	cmd := &cobra.Command{
		Use:   "download <id|url>...",
		Short: "Download media via yt-dlp",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var ytArgs []string
			if audio {
				ytArgs = append(ytArgs, "-f", "bestaudio")
			}
			if format != "" {
				ytArgs = append(ytArgs, "-f", format)
			}
			if out != "" {
				ytArgs = append(ytArgs, "-o", out+"/%(title)s [%(id)s].%(ext)s")
			}
			if items != "" {
				ytArgs = append(ytArgs, "--playlist-items", items)
			}
			if subLangs != "" {
				ytArgs = append(ytArgs, "--sub-langs", subLangs, "--write-subs")
			}
			if concurr > 0 {
				ytArgs = append(ytArgs, "--concurrent-fragments", fmt.Sprint(concurr))
			}
			if metadata {
				ytArgs = append(ytArgs, "--add-metadata")
			}
			_ = quality
			ytArgs = append(ytArgs, args...)
			return app.runYtDlp(cmd, ytArgs)
		},
	}
	f := cmd.Flags()
	f.BoolVar(&audio, "audio", false, "download best audio only")
	f.StringVar(&out, "out", ".", "output directory")
	f.StringVar(&format, "format", "", "yt-dlp format selector")
	f.StringVar(&quality, "quality", "", "preferred video quality (e.g. 1080)")
	f.StringVar(&items, "playlist-items", "", "playlist item selection")
	f.StringVar(&subLangs, "sub-langs", "", "subtitle languages to write")
	f.IntVar(&concurr, "concurrent-fragments", 0, "parallel fragment downloads")
	f.BoolVar(&metadata, "add-metadata", false, "embed metadata in the output file")
	return cmd
}

func newExtractCmd(app *App) *cobra.Command {
	var (
		out     string
		format  string
		quality string
	)
	cmd := &cobra.Command{
		Use:   "extract <audio|video|transcript|all> <id|url>",
		Short: "Extract a specific stream via yt-dlp",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
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
			return app.runYtDlp(cmd, ytArgs)
		},
	}
	f := cmd.Flags()
	f.StringVar(&out, "out", ".", "output directory")
	f.StringVar(&format, "format", "", "audio format for extract audio (e.g. mp3)")
	f.StringVar(&quality, "quality", "", "max video height for extract video (e.g. 1080)")
	return cmd
}
