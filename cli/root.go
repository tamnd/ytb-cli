// Package cli builds the ytb command tree on top of the youtube library.
package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/tamnd/ytb-cli/youtube"
)

// Build metadata, set via -ldflags at release time.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// App carries the resolved configuration and shared clients for a command run.
type App struct {
	Cfg      youtube.Config
	Client   *youtube.Client
	Out      *Output
	DBPath   string
	store    *youtube.Store // lazily opened when DBPath is set
	Limit    int
	MaxPages int
	Workers  int
	quiet    bool
	verbose  int
	yes      bool
	dryRun   bool
	YtDlpBin string
}

// globalFlags holds the persistent flag values before they are folded into Cfg.
type globalFlags struct {
	output   string
	fields   string
	limit    int
	maxPages int
	workers  int
	rate     time.Duration
	retries  int
	timeout  time.Duration
	hl       string
	gl       string
	db       string
	quiet    bool
	verbose  int
	color    string
	template string
	noHeader bool
	config   string
	yes      bool
	dryRun   bool
	ytDlpBin string
}

// Root builds the root command and its whole subtree.
func Root() *cobra.Command {
	g := &globalFlags{}
	app := &App{}

	root := &cobra.Command{
		Use:   "ytb",
		Short: "A delightful command line for YouTube",
		Long: `ytb is the fastest way to work with YouTube from your terminal.

Resolve a video to its full metadata, stream a channel's uploads, page a
playlist, search with the full filter grid, pull comments and transcripts,
follow hashtags and community posts, and optionally persist everything into a
local SQLite store, all from one binary and with no API key.

Quick start:
  ytb video dQw4w9WgXcQ                  full metadata for a video
  ytb channel @MrBeast --videos          stream a channel's uploads
  ytb search "lofi hip hop" -n 50        search with continuation paging
  ytb transcript dQw4w9WgXcQ             the video's transcript as text
  ytb search "go" -o id | ytb video -    batch from stdin`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			return app.init(g)
		},
	}

	pf := root.PersistentFlags()
	def := youtube.DefaultConfig()
	pf.StringVarP(&g.output, "output", "o", "auto", "table|json|jsonl|csv|tsv|url|id|raw")
	pf.StringVar(&g.fields, "fields", "", "comma-separated columns to show")
	pf.IntVarP(&g.limit, "limit", "n", 0, "max rows emitted (0 = unlimited)")
	pf.IntVar(&g.maxPages, "max-pages", 0, "max continuation pages fetched (0 = unlimited)")
	pf.IntVarP(&g.workers, "workers", "j", def.Workers, "concurrency for detail fetches")
	pf.DurationVar(&g.rate, "rate", def.Delay, "minimum delay between requests")
	pf.IntVar(&g.retries, "retries", def.Retries, "retry attempts on 429/5xx")
	pf.DurationVar(&g.timeout, "timeout", def.Timeout, "per-request timeout")
	pf.StringVar(&g.hl, "hl", def.HL, "InnerTube interface language")
	pf.StringVar(&g.gl, "gl", def.GL, "InnerTube content country")
	pf.StringVar(&g.db, "db", "", "optional SQLite store; persist everything fetched")
	pf.BoolVarP(&g.quiet, "quiet", "q", false, "suppress progress output")
	pf.CountVarP(&g.verbose, "verbose", "v", "increase verbosity (repeatable)")
	pf.StringVar(&g.color, "color", "auto", "color output: auto|always|never")
	pf.StringVar(&g.template, "template", "", "Go text/template applied per row")
	pf.BoolVar(&g.noHeader, "no-header", false, "omit the header row in table/csv/tsv")
	pf.StringVar(&g.config, "config", "", "config file (default: XDG config)")
	pf.BoolVarP(&g.yes, "yes", "y", false, "assume yes to prompts")
	pf.BoolVar(&g.dryRun, "dry-run", false, "print actions without performing them")
	pf.StringVar(&g.ytDlpBin, "yt-dlp-bin", "", "path to the yt-dlp binary (download/extract)")

	root.AddCommand(
		newVideoCmd(app),
		newChannelCmd(app),
		newPlaylistCmd(app),
		newSearchCmd(app),
		newTrendingCmd(app),
		newCommentsCmd(app),
		newCommunityCmd(app),
		newHashtagCmd(app),
		newSuggestCmd(app),
		newFormatsCmd(app),
		newTranscriptCmd(app),
		newRelatedCmd(app),
		newSeedCmd(app),
		newCrawlCmd(app),
		newQueueCmd(app),
		newJobsCmd(app),
		newDBCmd(app),
		newExportCmd(app),
		newMusicCmd(app),
		newDownloadCmd(app),
		newExtractCmd(app),
		newConfigCmd(app),
		newVersionCmd(),
	)
	return root
}

func (a *App) init(g *globalFlags) error {
	cfg := youtube.DefaultConfig()
	cfg.Workers = g.workers
	cfg.Delay = g.rate
	cfg.Retries = g.retries
	cfg.Timeout = g.timeout
	if g.hl != "" {
		cfg.HL = g.hl
	}
	if g.gl != "" {
		cfg.GL = g.gl
	}

	a.Cfg = cfg
	a.Client = youtube.NewClient(cfg)
	a.Limit = g.limit
	a.MaxPages = g.maxPages
	a.Workers = g.workers
	a.quiet = g.quiet
	a.verbose = g.verbose
	a.yes = g.yes
	a.dryRun = g.dryRun
	a.YtDlpBin = g.ytDlpBin
	a.Out = newOutput(g)

	// --db wins over YTB_DB.
	a.DBPath = g.db
	if a.DBPath == "" {
		a.DBPath = os.Getenv("YTB_DB")
	}
	return nil
}

// Store opens (once) and returns the optional SQLite store, or nil if --db is unset.
func (a *App) Store() (*youtube.Store, error) {
	if a.DBPath == "" {
		return nil, nil
	}
	if a.store != nil {
		return a.store, nil
	}
	s, err := youtube.OpenStore(a.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open store %q: %w", a.DBPath, err)
	}
	a.store = s
	return s, nil
}

// RequireStore returns the store or a usage error if --db is unset.
func (a *App) RequireStore() (*youtube.Store, error) {
	s, err := a.Store()
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, usageErr("this command needs a store: pass --db <path> (or set YTB_DB)")
	}
	return s, nil
}

// PageOptions builds a PageOptions from the global -n / --max-pages flags.
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

// Execute runs the root command, mapping errors to exit codes.
func Execute(ctx context.Context, cmd *cobra.Command) int {
	if err := cmd.ExecuteContext(ctx); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "youtube: "+err.Error())
		return ExitCode(err)
	}
	return 0
}

// ExitCode maps an error to a process exit code per the documented table.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	if ec, ok := err.(exitCoder); ok {
		return ec.ExitCode()
	}
	return 1
}

type exitCoder interface{ ExitCode() int }

type codedError struct {
	err  error
	code int
}

func (e codedError) Error() string { return e.err.Error() }
func (e codedError) ExitCode() int { return e.code }
func (e codedError) Unwrap() error { return e.err }

func noResults(msg string) error   { return codedError{fmt.Errorf("%s", msg), 3} }
func usageErr(msg string) error    { return codedError{fmt.Errorf("%s", msg), 2} }
func partialErr(msg string) error  { return codedError{fmt.Errorf("%s", msg), 4} }
func missingTool(msg string) error { return codedError{fmt.Errorf("%s", msg), 6} }
