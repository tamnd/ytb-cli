---
title: "Channels and playlists"
description: "Stream a channel's videos, shorts, streams, and playlists, and page any playlist's items."
weight: 20
---

The `channel` and `playlist` commands stream the content tabs you browse on the
site, following YouTube's continuation tokens to page through them. Output is a
table on a terminal and JSONL when piped; override with `-o`. See
[Output formats](/reference/output/) and the [CLI reference](/reference/cli/).

## Channels

`channel` accepts an `@handle`, a `UC...` channel id, or any channel URL:

```sh
ytb channel @MrBeast
ytb channel UCX6OQ3DkcsbYNE6H8uQQuVA
ytb channel "https://www.youtube.com/@MrBeast"
```

With no tab flag it prints the channel record: name, id, handle, subscriber
count, description, banner and avatar thumbnails, and the like.

### Content tabs

The tab flags switch it from the record to streaming that tab's items via
`/browse` continuation:

```sh
ytb channel @MrBeast --videos      # the uploads tab
ytb channel @MrBeast --shorts      # the Shorts tab
ytb channel @MrBeast --streams     # past live streams
ytb channel @MrBeast --playlists   # the channel's playlists
```

The grid that `/browse` returns omits some fields (view counts, full
descriptions). Add `--enrich` to fan out one `/player` call per video and fill
them in:

```sh
ytb channel @MrBeast --videos --enrich -j 8
```

### Paging and limits

`-n`/`--limit` caps the rows emitted and `--max-pages` caps continuation pages
fetched (both `0` mean unlimited). `--all` removes the default page cap and
streams the whole tab:

```sh
ytb channel @MrBeast --videos -n 200
ytb channel @MrBeast --videos --all -o jsonl > mrbeast.jsonl
```

Narrow the columns with `--fields` (lowercase column names):

```sh
ytb channel @MrBeast --videos --fields title,views,published
```

## Playlists

`playlist` takes a playlist id or URL. With no flag it prints the playlist
header (title, owner, item count, description):

```sh
ytb playlist PLFgquLnL59alCl_2TQvOiD5Vgm1hCaGSI
```

Add `--videos` to stream the items, each with its position and the basic video
fields, paging by continuation up to `-n`/`--max-pages`:

```sh
ytb playlist PLFgquLnL59alCl_2TQvOiD5Vgm1hCaGSI --videos -n 100
```

### Resolving items to full metadata

Emit just the ids with `-o id` and pipe them into `video -` to resolve each item
to its full record:

```sh
ytb playlist PLFgquLnL59alCl_2TQvOiD5Vgm1hCaGSI --videos -o id | ytb video -
```

## Persisting

Add `--db <path>` to either command and ytb writes the channel, playlists,
videos, and their relationships into a local SQLite store as it streams. See
[The store](/guides/the-store/).
