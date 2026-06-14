// Package cli assembles the ytb command tree on top of the youtube library and
// the any-cli/kit framework. The record operations are declared once in the
// youtube domain (so the same definitions drive the CLI, the serve and mcp
// surfaces, and an ant host); the media, transcript, local-store, and config
// commands are escape-hatch kit.Command commands that share the run state
// through the context with appFromCtx.
package cli

import (
	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/ytb-cli/youtube"
)

// Build metadata, set via -ldflags at release time.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// builder holds the youtube-specific globals and defaults while a kit.App is
// assembled. The globals hook binds the flags to it; the finalize hook reads
// them back onto the resolved Config so the client factory and escape hatches
// see them.
type builder struct {
	def      youtube.Config
	workers  int
	maxPages int
	hl       string
	gl       string
	yes      bool
	ytDlpBin string
	ffmpeg   string
}

// NewApp builds the kit application: identity, the youtube global flags, the
// record operations and client factory (installed by the domain, the same as an
// ant host gets), and the escape-hatch commands.
func NewApp() *kit.App {
	b := &builder{def: youtube.DefaultConfig()}

	app := kit.New(kit.Identity{
		Binary:  "ytb",
		Version: Version,
		Short:   "A delightful command line for YouTube",
		Long: `ytb is the fastest way to work with YouTube from your terminal.

Resolve a video to its full metadata, stream a channel's uploads, page a
playlist, search with the full filter grid, pull comments and transcripts,
follow hashtags and community posts, download media with a pure-Go engine, and
build a local dataset, all from one binary and with no API key.

Quick start:
  ytb video dQw4w9WgXcQ                  full metadata for a video
  ytb uploads @MrBeast -n 50             stream a channel's uploads
  ytb search "lofi hip hop" -n 50        search with continuation paging
  ytb transcript dQw4w9WgXcQ             the video's transcript as text
  ytb search "go" -o url | ytb video -   batch from stdin`,
		Site: "https://www.youtube.com",
		Repo: "https://github.com/tamnd/ytb-cli",
	}, kit.WithDefaults(b.defaults))

	app.GlobalFlags(b.globals)
	app.Finalize(b.finalize)

	// The domain installs the client factory and every record operation, exactly
	// as it does inside an ant host. The escape hatches are the binary's own.
	(youtube.Domain{}).Register(app)
	registerEscapeHatches(app)
	return app
}

// defaults seeds the framework baseline from the youtube defaults, so an unset
// --rate/--retries/--timeout keeps youtube's own values.
func (b *builder) defaults(c *kit.Config) {
	c.Rate = b.def.Delay
	c.Retries = b.def.Retries
	c.Timeout = b.def.Timeout
	c.Workers = b.def.Workers
}

// globals registers the youtube-specific persistent flags on top of the kit
// framework globals (-o, --fields, -n, --rate, --retries, --timeout, --db, and
// the rest).
func (b *builder) globals(f *kit.FlagSet) {
	f.IntVarP(&b.workers, "workers", "j", b.def.Workers, "concurrency for detail fetches")
	f.IntVar(&b.maxPages, "max-pages", 0, "max continuation pages fetched (0 = unlimited)")
	f.StringVar(&b.hl, "hl", b.def.HL, "InnerTube interface language")
	f.StringVar(&b.gl, "gl", b.def.GL, "InnerTube content country")
	f.BoolVarP(&b.yes, "yes", "y", false, "assume yes to prompts")
	f.StringVar(&b.ytDlpBin, "yt-dlp-bin", "", "path to the yt-dlp binary (download --use-yt-dlp, transcript fallback)")
	f.StringVar(&b.ffmpeg, "ffmpeg-bin", "", "path to ffmpeg (used to merge and convert when present)")
}

// finalize folds the youtube globals onto the resolved Config: the worker count
// becomes the framework worker count, and the site and tool settings travel in
// Config.Extra, where both the client factory and the escape hatches read them.
func (b *builder) finalize(c *kit.Config) {
	if b.workers > 0 {
		c.Workers = b.workers
	}
	if c.Extra == nil {
		c.Extra = map[string]string{}
	}
	c.Extra["hl"] = b.hl
	c.Extra["gl"] = b.gl
	c.Extra["max-pages"] = itoa(b.maxPages)
	c.Extra["yt-dlp-bin"] = b.ytDlpBin
	c.Extra["ffmpeg-bin"] = b.ffmpeg
	if b.yes {
		c.Extra["yes"] = "true"
	}
}
