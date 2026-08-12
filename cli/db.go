package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/ytb-cli/ytb"
)

func newDBCmd() kit.Command {
	return kit.Command{
		Use:   "db",
		Short: "The local store",
		Long: `Inspect the local SQLite store at <data-dir>/ytb.db. Pure-Go, no cgo.

Three tables. nodes is everything that has an identity, one row per URI, with
the record as JSON and a null record for a node a claim named that nobody has
fetched yet. claims is the edges, one row per observation, so the same edge seen
on the watch page and in a browse response is two rows. reads is the log: every
request, what answered, and how big it was.

To run SQL over them use ytb query, which opens the file read-only.`,
		Sub: []kit.Command{
			newDBStatsCmd(),
			newDBSearchCmd(),
			newDBPathCmd(),
			newDBVacuumCmd(),
			newDBResetCmd(),
		},
	}
}

func newDBStatsCmd() kit.Command {
	return kit.Command{
		Use:   "stats",
		Short: "What is in the store: nodes by kind, claims by predicate, reads by surface",
		Args:  kit.NoArgs,
		Run: func(ctx context.Context, _ []string) error {
			app := appFromCtx(ctx)
			store, err := app.RequireStore()
			if err != nil {
				return err
			}
			stats, err := store.Stats()
			if err != nil {
				return err
			}
			if len(stats) == 0 {
				return noResults("the store is empty")
			}
			return EmitAll(app, stats, func(s ytb.StatRow) Row {
				vals := []string{s.Table, s.Key, i64a(s.Rows), ""}
				if s.Bytes > 0 {
					vals[3] = humanBytes(s.Bytes)
				}
				return Row{
					Cols:  []string{"table", "key", "rows", "bytes"},
					Vals:  vals,
					Value: s,
				}
			})
		},
	}
}

// newQueryCmd is doc 04 section 4: SQL over the store with no wrapper.
//
// The file is opened mode=ro, so a finger slip that says delete is refused by
// SQLite rather than by a check in this tool.
func newQueryCmd() kit.Command {
	return kit.Command{
		Use:   "query <sql>",
		Short: "Run SQL over the store, read-only",
		Long: `Run a SQL statement against <data-dir>/ytb.db and print the rows.

The file is opened read-only, so a statement that would write is refused by
SQLite itself rather than by a check here.

  ytb query "select predicate, count(*) c from claims group by 1 order by c desc"
  ytb query "select uri from nodes where kind='video' and record is null limit 20"
  ytb query "select json_extract(record,'$.title') from nodes where kind='channel'"`,
		Args: kit.ExactArgs(1),
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)
			path := app.StorePath()
			if _, err := os.Stat(path); err != nil {
				return fmt.Errorf("no store at %s yet: ytb crawl writes one", path)
			}
			store, err := ytb.OpenStoreReadOnly(path)
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()
			cols, rows, err := store.Query(args[0])
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				return noResults("no rows")
			}
			return EmitAll(app, rows, func(r []any) Row {
				vals := make([]string, len(r))
				obj := make(map[string]any, len(r))
				for i, v := range r {
					vals[i] = anyToString(v)
					if i < len(cols) {
						obj[cols[i]] = v
					}
				}
				return Row{Cols: cols, Vals: vals, Value: obj}
			})
		},
	}
}

func newDBSearchCmd() kit.Command {
	var channels bool
	return kit.Command{
		Use:   "search <query>",
		Short: "Full-text search over stored data",
		Args:  kit.MinimumNArgs(1),
		Flags: func(f *kit.FlagSet) {
			f.BoolVar(&channels, "channels", false, "search channels instead of videos")
		},
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)
			store, err := app.RequireStore()
			if err != nil {
				return err
			}
			q := strings.Join(args, " ")
			limit := app.Limit
			if limit == 0 {
				limit = 50
			}
			if channels {
				rows, err := store.SearchChannels(q, limit)
				if err != nil {
					return err
				}
				if len(rows) == 0 {
					return noResults("no matching channels")
				}
				return EmitAll(app, rows, channelRow)
			}
			rows, err := store.SearchVideos(q, limit)
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				return noResults("no matching videos")
			}
			return EmitAll(app, rows, videoRow)
		},
	}
}

func newDBPathCmd() kit.Command {
	return kit.Command{
		Use:   "path",
		Short: "Print the db file location",
		Args:  kit.NoArgs,
		Run: func(ctx context.Context, _ []string) error {
			app := appFromCtx(ctx)
			store, err := app.RequireStore()
			if err != nil {
				return err
			}
			return app.Line(store.Path())
		},
	}
}

func newDBVacuumCmd() kit.Command {
	return kit.Command{
		Use:   "vacuum",
		Short: "Compact the database file",
		Args:  kit.NoArgs,
		Run: func(ctx context.Context, _ []string) error {
			app := appFromCtx(ctx)
			store, err := app.RequireStore()
			if err != nil {
				return err
			}
			return store.Vacuum()
		},
	}
}

func newDBResetCmd() kit.Command {
	return kit.Command{
		Use:   "reset",
		Short: "Drop and recreate all tables",
		Args:  kit.NoArgs,
		Run: func(ctx context.Context, _ []string) error {
			app := appFromCtx(ctx)
			store, err := app.RequireStore()
			if err != nil {
				return err
			}
			if !confirm(app.yes, "This deletes all stored data. Continue?") {
				return usageErr("aborted")
			}
			return store.Reset()
		},
	}
}

func anyToString(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	default:
		return fmt.Sprintf("%v", x)
	}
}
