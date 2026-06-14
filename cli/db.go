package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/tamnd/any-cli/kit"
)

func newDBCmd() kit.Command {
	return kit.Command{
		Use:   "db",
		Short: "The local SQLite store",
		Long:  `Inspect and query the local SQLite store at <data-dir>/ytb.db. Pure-Go, no cgo.`,
		Sub: []kit.Command{
			newDBStatsCmd(),
			newDBQueryCmd(),
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
		Short: "Row counts per table",
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
			tables := make([]string, 0, len(stats))
			for t := range stats {
				tables = append(tables, t)
			}
			sort.Strings(tables)
			for _, t := range tables {
				if err := app.Out.Emit(Row{
					Cols:  []string{"table", "rows"},
					Vals:  []string{t, i64a(stats[t])},
					Value: map[string]any{"table": t, "rows": stats[t]},
				}); err != nil {
					return err
				}
			}
			return app.Out.Flush()
		},
	}
}

func newDBQueryCmd() kit.Command {
	return kit.Command{
		Use:   "query <sql>",
		Short: "Run a read-only SQL query",
		Args:  kit.ExactArgs(1),
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)
			store, err := app.RequireStore()
			if err != nil {
				return err
			}
			cols, rows, err := store.Query(args[0])
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				return noResults("no rows")
			}
			for _, r := range rows {
				vals := make([]string, len(r))
				obj := make(map[string]any, len(r))
				for i, v := range r {
					vals[i] = anyToString(v)
					if i < len(cols) {
						obj[cols[i]] = v
					}
				}
				if err := app.Out.Emit(Row{Cols: cols, Vals: vals, Value: obj}); err != nil {
					return err
				}
			}
			return app.Out.Flush()
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
				for _, c := range rows {
					if err := app.Out.Emit(channelRow(c)); err != nil {
						return err
					}
				}
				return app.Out.Flush()
			}
			rows, err := store.SearchVideos(q, limit)
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				return noResults("no matching videos")
			}
			for _, v := range rows {
				if err := app.Out.Emit(videoRow(v)); err != nil {
					return err
				}
			}
			return app.Out.Flush()
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
