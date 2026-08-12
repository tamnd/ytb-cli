package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/ytb-cli/ytb"
)

// newCrawlCmd is the budgeted walk of doc 04 section 3.3.
//
// There is no queue table and no seed command any more. The frontier is a query
// over the store: every node a claim named that nobody has fetched. That is why
// a crawl can be resumed by a later run with no state passed between them, and
// why `ytb crawl --resume` with no seeds at all is a sentence that means
// something.
func newCrawlCmd() kit.Command {
	var (
		depth    int
		budget   int
		items    int
		comments int
		posts    int
		captions bool
		featured bool
		music    bool
		uploads  bool
		resume   bool
		manifest string
	)
	return kit.Command{
		Use:   "crawl <seed>...",
		Short: "Walk the graph from seeds and write it into the store",
		Long: `Read each seed, write its claims into the store, and read what those claims
named, hop by hop, until the depth or the budget runs out.

The budget is in requests, and it is counted rather than estimated: every
request the client makes ticks it down, and a cache hit makes no request so it
costs nothing. When the budget runs out the crawl stops where it is and says so
in the manifest.

The frontier is ordered cheapest first. A playlist browse came back with 98
items in one response and a channel tab with about thirty, while a watch page is
1.3 MB for one video, so listings are read before watch pages.

Four things are off the frontier. A mix (RD...) is generated per viewer and
browses to nothing. A channel's popular playlist (UULP) is its uploads in
another order, so reading it spends a request on videos the crawl already has.
Comments are refused at tier 0 on most addresses, and --comments turns them back
on. Formats need a player call per video and produce no claim beyond the caption
tracks. None of them is refused as a seed: asking for one by name is a different
question.

With --resume and no seeds the crawl starts from the frontier itself, which is
every node in the store that a claim named and nobody has read.

Every crawl writes a manifest: the seeds, the budget, what it spent, which
surfaces and clients answered, and every refusal with the reason quoted. A crawl
that hit a wall and a crawl that ran out of budget leave the same store and
different manifests.`,
		Args: kit.MinimumNArgs(0),
		Flags: func(f *kit.FlagSet) {
			f.IntVar(&depth, "depth", 1, "hops to follow from each seed (0 = seeds only)")
			f.IntVar(&budget, "budget", 200, "request budget for the whole crawl (0 = no limit)")
			f.IntVar(&items, "items", 100, "playlist items to read per playlist (0 = header only)")
			f.IntVar(&comments, "comments", 0, "comments to read per video (0 = none)")
			f.IntVar(&posts, "posts", 0, "community posts to read per channel (0 = none)")
			f.BoolVar(&captions, "captions", false, "read each video's caption list (one ANDROID player call per video)")
			f.BoolVar(&featured, "featured", false, "read each channel's featured channels shelf")
			f.BoolVar(&music, "music", false, "read each video through YouTube Music too")
			f.BoolVar(&uploads, "uploads", true, "put each channel's derived playlists on the frontier")
			f.BoolVar(&resume, "resume", false, "start from the nodes the store has heard of and not read")
			f.StringVar(&manifest, "manifest", "", "where to write the manifest (default <data-dir>/crawls/crawl-<unix>.json)")
		},
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)
			if len(args) == 0 && !resume {
				return usageErr("crawl needs a seed, or --resume to pick up the nodes the store has heard of and not read")
			}
			store, err := app.RequireStore()
			if err != nil {
				return err
			}
			opt := ytb.CrawlOptions{
				Depth:   depth,
				Budget:  budget,
				Uploads: uploads,
				Resume:  resume,
				Claims: ytb.ClaimOptions{
					Items:    items,
					Comments: comments,
					Posts:    posts,
					Captions: captions,
					Featured: featured,
					Music:    music,
				},
			}
			if app.dryRun {
				app.logf("would crawl %d seeds to depth %d on a budget of %d requests", len(args), depth, budget)
				return nil
			}
			var logf func(string)
			if !app.quiet {
				logf = func(s string) { app.logf("%s", s) }
			}
			m, crawlErr := ytb.Crawl(ctx, app.Client, store, args, opt, logf)
			// The manifest is written even when the crawl was cancelled, because a
			// store with rows in it and nothing to say where they came from is the
			// thing the manifest exists to prevent.
			path, writeErr := writeManifest(app, manifest, m)
			if crawlErr != nil {
				return crawlErr
			}
			if writeErr != nil {
				return writeErr
			}
			if err := app.Out.Emit(manifestRow(m, path)); err != nil {
				return err
			}
			return app.Out.Flush()
		},
	}
}

// writeManifest puts the manifest next to the store, under crawls/, named by the
// second it started. Nothing overwrites anything: two crawls of the same seeds
// are two records of two different days.
func writeManifest(app *App, path string, m *ytb.Manifest) (string, error) {
	if m == nil {
		return "", nil
	}
	if path == "" {
		dir := filepath.Join(filepath.Dir(app.StorePath()), "crawls")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
		path = filepath.Join(dir, fmt.Sprintf("crawl-%d.json", m.StartedAt.Unix()))
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func manifestRow(m *ytb.Manifest, path string) Row {
	if m == nil {
		return Row{Cols: []string{"manifest"}, Vals: []string{path}}
	}
	took := m.FinishedAt.Sub(m.StartedAt).Round(time.Second)
	return Row{
		Cols: []string{"spent", "reads", "bytes", "records", "claims", "nodes", "took", "stopped_by", "manifest"},
		Vals: []string{
			itoa(m.Spent), itoa(m.Reads), humanBytes(m.Bytes),
			itoa(m.Records), itoa(m.Claims), itoa(m.Nodes),
			took.String(), m.StoppedBy, path,
		},
		Value: struct {
			*ytb.Manifest
			Path string `json:"manifest_path"`
		}{m, path},
	}
}
