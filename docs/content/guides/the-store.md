---
title: "The local store"
description: "Persist everything you fetch into SQLite, then crawl it with a work queue and query it with SQL."
weight: 60
---

Most commands are stateless: they stream straight to stdout and keep nothing.
Pass the global `--db <path>` flag and ytb also writes everything it fetches
into a SQLite database (videos, channels, playlists, comments, caption tracks,
formats, and the relationships between them). The same commands you already run
become a crawler, and you get SQL over what you have collected.

```sh
ytb channel @MrBeast --videos --db yt.db   # stream and persist in one pass
ytb video dQw4w9WgXcQ --db yt.db            # one video, with its relations
```

The store is pure Go (modernc.org/sqlite), so nothing links libsqlite and the
binary stays static. Without `--db`, no database is ever created: the flag is the
only thing that turns persistence on.

## Building a crawl queue

For larger collection runs, `seed`, `crawl`, `queue`, and `jobs` turn the store
into a work queue that a pool of workers drains. All four need `--db`.

`seed` loads a worklist. Pass a single item, or `--file` to load a
newline-delimited list, and `--entity` to tag the kind
(`video`, `channel`, `playlist`, `search`, `hashtag`, or `community`). Use
`--priority` to push entries to the front.

```sh
ytb seed @MrBeast --entity channel --db yt.db
ytb seed --file ids.txt --entity video --db yt.db
ytb seed "lofi" --entity search --priority 10 --db yt.db
```

`search` can enqueue its own results directly with `--enqueue`:

```sh
ytb search "podcast" -o id --enqueue --db yt.db   # seed the queue from a search
```

`crawl` drains the queue with `-j` workers, persisting each result as it goes.
`--max-per-item` caps how many items a single queue entry yields, and `--entity`
restricts the run to one kind.

```sh
ytb crawl --db yt.db -j 8                  # drain with 8 workers
ytb crawl --db yt.db --entity video --max-per-item 200
```

`queue` shows what is pending, and `--status` filters by `pending`, `done`, or
`failed`. `jobs` prints the recent crawl job history.

```sh
ytb queue --db yt.db                        # pending work
ytb queue --status failed --db yt.db        # what went wrong
ytb jobs --db yt.db                          # recent runs
```

## Querying the store

The `db` command group inspects and queries the SQLite store. It is read-only by
nature (queries cannot mutate), pure Go, with no cgo.

`db stats` reports row counts per table. `db path` prints where the file lives.

```sh
ytb db stats --db yt.db
ytb db path --db yt.db
```

`db query` runs a read-only SQL statement against the store. Tables follow the
data models, so a `videos` table carries columns like `title`, `view_count`, and
`channel_name`:

```sh
ytb db query "select title, view_count from videos order by view_count desc limit 10" --db yt.db
ytb db query "select channel_name, count(*) from videos group by channel_name" --db yt.db
```

`db search` runs a full-text search over stored data. It searches videos by
default; pass `--channels` to search channels instead.

```sh
ytb db search "mukbang" --db yt.db          # over stored video titles
ytb db search "tech" --channels --db yt.db
```

`db vacuum` compacts the database file. `db reset` drops and recreates all tables
(it clears everything, so it prompts unless you pass `-y`).

```sh
ytb db vacuum --db yt.db
ytb db reset --db yt.db -y
```

## Exporting to Markdown

`export` renders the stored data as an interlinked Markdown site under `--out`:
per-video pages with YAML frontmatter, chapter lists, transcripts, related
sidebars, and channel and playlist index pages. With no argument it exports every
channel in the store; pass a channel id or `@handle` to scope it. It needs `--db`.

```sh
ytb export --db yt.db --out site/           # the whole store
ytb export @MrBeast --db yt.db --out site/  # one channel
```

## These commands need a store

`crawl`, `queue`, `jobs`, `export`, the `db` subcommands, and `search --enqueue`
all operate on the store, so they require `--db`. Run them without it and ytb
errors clearly rather than silently doing nothing, since without `--db` there is
no database to act on.
