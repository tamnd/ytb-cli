package cli

import (
	"context"
	"os"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/ytb-cli/youtube"
)

func newSeedCmd() kit.Command {
	var (
		file     string
		entity   string
		priority int
	)
	return kit.Command{
		Use:   "seed [item]",
		Short: "Load a worklist into the crawl queue",
		Long: `Enqueue items for the crawler. Pass a single item argument, or --file to load a
newline-delimited worklist. --entity tags the kind
(video|channel|playlist|search|hashtag|community).`,
		Args: kit.MaximumNArgs(1),
		Flags: func(f *kit.FlagSet) {
			f.StringVar(&file, "file", "", "newline-delimited worklist file")
			f.StringVar(&entity, "entity", "", "entity kind for the seeded items")
			f.IntVar(&priority, "priority", 0, "queue priority (higher runs first)")
		},
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)
			store, err := app.RequireStore()
			if err != nil {
				return err
			}
			if entity == "" {
				return usageErr("--entity is required (video|channel|playlist|search|hashtag|community)")
			}
			var n int
			enqueue := func(item string) error {
				if app.dryRun {
					app.logf("would enqueue %s (%s)", item, entity)
					return nil
				}
				if err := store.Enqueue(item, entity, priority); err != nil {
					return err
				}
				n++
				return nil
			}
			if file != "" {
				f, err := os.Open(file)
				if err != nil {
					return err
				}
				defer func() { _ = f.Close() }()
				if err := readLines(f, enqueue); err != nil {
					return err
				}
			}
			for _, a := range args {
				if err := enqueue(a); err != nil {
					return err
				}
			}
			if file == "" && len(args) == 0 {
				if err := readLines(cmdIn, enqueue); err != nil {
					return err
				}
			}
			app.logf("enqueued %d items", n)
			return nil
		},
	}
}

func newCrawlCmd() kit.Command {
	var (
		entity     string
		maxPerItem int
	)
	return kit.Command{
		Use:   "crawl",
		Short: "Process the crawl queue with workers",
		Args:  kit.NoArgs,
		Flags: func(f *kit.FlagSet) {
			f.StringVar(&entity, "entity", "", "only crawl one entity kind")
			f.IntVar(&maxPerItem, "max-per-item", 0, "cap items fetched per queue entry")
		},
		Run: func(ctx context.Context, _ []string) error {
			app := appFromCtx(ctx)
			store, err := app.RequireStore()
			if err != nil {
				return err
			}
			opt := youtube.CrawlOptions{
				Workers:    app.Workers,
				Entity:     entity,
				MaxPerItem: maxPerItem,
			}
			var logf func(string)
			if !app.quiet {
				logf = func(s string) { app.logf("%s", s) }
			}
			return youtube.Crawl(ctx, app.Client, store, opt, logf)
		},
	}
}

func newQueueCmd() kit.Command {
	var status string
	return kit.Command{
		Use:   "queue",
		Short: "Inspect the crawl queue",
		Args:  kit.NoArgs,
		Flags: func(f *kit.FlagSet) {
			f.StringVar(&status, "status", "", "filter by status (pending|done|failed)")
		},
		Run: func(ctx context.Context, _ []string) error {
			app := appFromCtx(ctx)
			store, err := app.RequireStore()
			if err != nil {
				return err
			}
			limit := app.Limit
			items, err := store.ListQueue(status, limit)
			if err != nil {
				return err
			}
			if len(items) == 0 {
				return noResults("queue is empty")
			}
			for _, it := range items {
				if err := app.Out.Emit(queueRow(it)); err != nil {
					return err
				}
			}
			return app.Out.Flush()
		},
	}
}

func newJobsCmd() kit.Command {
	return kit.Command{
		Use:   "jobs",
		Short: "Recent crawl job history",
		Args:  kit.NoArgs,
		Run: func(ctx context.Context, _ []string) error {
			app := appFromCtx(ctx)
			store, err := app.RequireStore()
			if err != nil {
				return err
			}
			limit := app.Limit
			if limit == 0 {
				limit = 50
			}
			jobs, err := store.ListJobs(limit)
			if err != nil {
				return err
			}
			if len(jobs) == 0 {
				return noResults("no jobs recorded")
			}
			for _, j := range jobs {
				if err := app.Out.Emit(jobRow(j)); err != nil {
					return err
				}
			}
			return app.Out.Flush()
		},
	}
}
