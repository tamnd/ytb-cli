package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/tamnd/any-cli/kit"
)

// cache.go is the three subcommands for the on-disk response cache.
//
// The cache is what makes a repeat call instant and what makes a second crawl
// get further on the same budget, and until now the only way to look at it or
// get rid of it was to know where it lives and reach for rm. --no-cache
// bypasses it for one run and does not help with the run that already poisoned
// it.

func newCacheCmd() kit.Command {
	return kit.Command{
		Use:   "cache",
		Short: "The on-disk response cache",
		Long: `Inspect or clear the cache under <data-dir>/cache/innertube.

Every read goes through it keyed by the URL plus the client that claimed it, so
a WEB player response is never served to a caption read that asked as ANDROID.
An entry older than --cache-ttl is not served and is not deleted either, so a
cache can be almost entirely stale and still take up the whole of its space.

ytb archive writes outside this cache and never expires.`,
		Sub: []kit.Command{
			newCachePathCmd(),
			newCacheInfoCmd(),
			newCacheClearCmd(),
		},
	}
}

func newCachePathCmd() kit.Command {
	return kit.Command{
		Use:   "path",
		Short: "Print the cache directory",
		Args:  kit.NoArgs,
		Run: func(ctx context.Context, _ []string) error {
			app := appFromCtx(ctx)
			dir := app.Client.Cache().Dir()
			if dir == "" {
				return errNoCache()
			}
			return app.Line(dir)
		},
	}
}

func newCacheInfoCmd() kit.Command {
	return kit.Command{
		Use:   "info",
		Short: "How many entries are cached, how much space they take, and how many are stale",
		Args:  kit.NoArgs,
		Run: func(ctx context.Context, _ []string) error {
			app := appFromCtx(ctx)
			info, err := app.Client.Cache().Info()
			if err != nil {
				return err
			}
			if info.Dir == "" {
				return errNoCache()
			}
			rows := [][2]string{
				{"dir", info.Dir},
				{"ttl", info.TTL.String()},
				{"entries", itoa(info.Entries)},
				{"fresh", itoa(info.Fresh)},
				{"stale", itoa(info.Stale)},
				{"size", humanBytes(info.Bytes)},
			}
			if !info.Oldest.IsZero() {
				rows = append(rows,
					[2]string{"oldest", info.Oldest.Format(time.RFC3339)},
					[2]string{"newest", info.Newest.Format(time.RFC3339)})
			}
			return EmitAll(app, rows, func(r [2]string) Row {
				return Row{
					Cols:  []string{"key", "value"},
					Vals:  []string{r[0], r[1]},
					Value: map[string]any{"key": r[0], "value": r[1]},
				}
			})
		},
	}
}

func newCacheClearCmd() kit.Command {
	return kit.Command{
		Use:   "clear",
		Short: "Delete every cached response",
		Long: `Delete every entry, fresh or stale.

Nothing here is a record: the store keeps what was parsed and this keeps the
bytes a request answered with, so clearing it costs time on the next run and
loses nothing.`,
		Args: kit.NoArgs,
		Run: func(ctx context.Context, _ []string) error {
			app := appFromCtx(ctx)
			cache := app.Client.Cache()
			if cache.Dir() == "" {
				return errNoCache()
			}
			if app.dryRun {
				info, err := cache.Info()
				if err != nil {
					return err
				}
				app.logf("would delete %d entries (%s) from %s", info.Entries, humanBytes(info.Bytes), info.Dir)
				return nil
			}
			n, err := cache.Purge()
			if err != nil {
				return err
			}
			return app.Line(fmt.Sprintf("deleted %d cached responses", n))
		},
	}
}

// errNoCache is what all three say when --no-cache is in effect, since the
// alternative is printing an empty path or a count of zero and letting the user
// conclude the cache is empty when it is only switched off for this run.
func errNoCache() error {
	return usageErr("the cache is off for this run, so there is nothing to look at; drop --no-cache")
}
