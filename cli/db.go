package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

func newDBCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "The local SQLite store",
		Long:  `Inspect and query the optional SQLite store (--db). Pure-Go, no cgo.`,
	}
	cmd.AddCommand(
		newDBStatsCmd(app),
		newDBQueryCmd(app),
		newDBSearchCmd(app),
		newDBPathCmd(app),
		newDBVacuumCmd(app),
		newDBResetCmd(app),
	)
	return cmd
}

func newDBStatsCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Row counts per table",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
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

func newDBQueryCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "query <sql>",
		Short: "Run a read-only SQL query",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
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

func newDBSearchCmd(app *App) *cobra.Command {
	var channels bool
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Full-text search over stored data",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
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
	cmd.Flags().BoolVar(&channels, "channels", false, "search channels instead of videos")
	return cmd
}

func newDBPathCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the db file location",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			store, err := app.RequireStore()
			if err != nil {
				return err
			}
			return app.Out.Line(store.Path())
		},
	}
}

func newDBVacuumCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "vacuum",
		Short: "Compact the database file",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			store, err := app.RequireStore()
			if err != nil {
				return err
			}
			return store.Vacuum()
		},
	}
}

func newDBResetCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "reset",
		Short: "Drop and recreate all tables",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
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
