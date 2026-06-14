package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
	"github.com/tamnd/any-cli/kit/render"
	"github.com/tamnd/ytb-cli/youtube"
)

// Row is one output record: an ordered set of named columns plus the original
// value used by json/jsonl and templates. It is kit's render.Record, so the row
// builders feed straight into the shared renderer with no per-format code.
type Row = render.Record

// App is the run state an escape-hatch command works through. The record
// operations live in the youtube domain and receive the *youtube.Client by
// injection; the escape-hatch commands (download, transcript text, the local
// store, config) need more than the client, so they rebuild this state from the
// run context with appFromCtx and share the same renderer, limit, and pacing.
type App struct {
	Cfg       youtube.Config
	Client    *youtube.Client
	Out       *render.Renderer
	st        *kit.State
	DataDir   string
	store     *youtube.Store // the typed crawl store, opened once on demand
	Limit     int
	MaxPages  int
	Workers   int
	quiet     bool
	dryRun    bool
	yes       bool
	YtDlpBin  string
	FFmpegBin string
}

// appFromCtx assembles the run's App for an escape-hatch command. The client was
// built once by the domain factory (newClient) and is shared here; the renderer,
// limit, and the youtube-specific globals come from the resolved run state, so an
// escape hatch behaves like every operation.
//
// The client factory cannot fail, so a missing or mistyped client is a wiring
// bug, surfaced as a panic rather than threaded through every command.
func appFromCtx(ctx context.Context) *App {
	st := kit.FromContext(ctx)
	yc := kit.MustClient[*youtube.Client](ctx)
	kc := st.Config
	a := &App{
		Cfg:       ytConfig(kc),
		Client:    yc,
		st:        st,
		DataDir:   kc.DataDir,
		Limit:     st.Globals.Limit,
		MaxPages:  atoi(kc.Extra["max-pages"]),
		Workers:   kc.Workers,
		quiet:     kc.Quiet,
		dryRun:    kc.DryRun,
		yes:       kc.Extra["yes"] == "true",
		YtDlpBin:  kc.Extra["yt-dlp-bin"],
		FFmpegBin: kc.Extra["ffmpeg-bin"],
	}
	a.Out = a.renderTo(os.Stdout)
	return a
}

// ytConfig folds the resolved framework config and the youtube globals (carried
// in Config.Extra) into a youtube.Config. It mirrors the domain client factory,
// so the standalone binary and an ant host build the same client.
func ytConfig(kc kit.Config) youtube.Config {
	yc := youtube.DefaultConfig()
	if kc.Workers > 0 {
		yc.Workers = kc.Workers
	}
	if kc.Rate > 0 {
		yc.Delay = kc.Rate
	}
	if kc.Retries > 0 {
		yc.Retries = kc.Retries
	}
	if kc.Timeout > 0 {
		yc.Timeout = kc.Timeout
	}
	if hl := kc.Extra["hl"]; hl != "" {
		yc.HL = hl
	}
	if gl := kc.Extra["gl"]; gl != "" {
		yc.GL = gl
	}
	return yc
}

// renderTo builds a renderer over w using the run's resolved output settings. The
// --template was validated when the run state was built, so a renderer over a
// valid writer cannot fail here.
func (a *App) renderTo(w io.Writer) *render.Renderer {
	r, err := a.st.Renderer(w)
	if err != nil {
		panic(err)
	}
	return r
}

// Line prints a raw line of text to stdout, for the transcript body, lyrics, and
// the store path, which are plain text rather than records.
func (a *App) Line(s string) error {
	_, err := fmt.Fprintln(cmdOut, s)
	return err
}

// StorePath is the fixed location of the typed crawl store, under the data dir.
// Unlike kit's generic --db record tee, this store carries the rich youtube
// schema the crawl, queue, export, and db commands read and write.
func (a *App) StorePath() string {
	dir := a.DataDir
	if dir == "" {
		dir = "."
	}
	return filepath.Join(dir, "ytb.db")
}

// Store opens (once) and returns the typed crawl store, creating the data dir.
func (a *App) Store() (*youtube.Store, error) {
	if a.store != nil {
		return a.store, nil
	}
	path := a.StorePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	s, err := youtube.OpenStore(path)
	if err != nil {
		return nil, fmt.Errorf("open store %q: %w", path, err)
	}
	a.store = s
	return s, nil
}

// RequireStore returns the typed crawl store. It exists for the commands whose
// whole job is the store; the store always opens at the fixed path, so this no
// longer fails for a missing flag.
func (a *App) RequireStore() (*youtube.Store, error) { return a.Store() }

// PageOptions builds a PageOptions from the resolved -n / --max-pages values.
func (a *App) PageOptions(enrich bool) youtube.PageOptions {
	return youtube.PageOptions{Max: a.Limit, MaxPages: a.MaxPages, Enrich: enrich}
}

// logf writes a progress line to stderr unless --quiet.
func (a *App) logf(format string, args ...any) {
	if a.quiet {
		return
	}
	_, _ = fmt.Fprintf(cmdErr, format+"\n", args...)
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// noResults, usageErr, missingTool, and partialErr classify the common command
// failures so kit maps them to the stable exit codes on every surface: no
// results is 3, a usage problem is 2, a missing external tool is 7 (unsupported
// here), and a partial failure stays a plain error (1).
func noResults(msg string) error   { return errs.NoResults("%s", msg) }
func usageErr(msg string) error    { return errs.Usage("%s", msg) }
func missingTool(msg string) error { return errs.Unsupported("%s", msg) }
func partialErr(msg string) error  { return fmt.Errorf("%s", msg) }
