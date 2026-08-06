---
title: "Channels and playlists"
description: "Read a channel's record, stream its uploads, shorts and streams, and page any playlist's items."
weight: 20
---

A channel is spread across a handful of commands rather than one command with tab flags.
`channel` reads the record, `about` reads the about panel, `uploads` streams the videos, `feed` reads the Atom feed, `playlists` lists the playlists, and `items` pages a playlist.
Output is a table on a terminal and JSONL when piped; override with `-o`.
See [Output formats](/reference/output/) and the [CLI reference](/reference/cli/).

## The channel record

Every channel command accepts an `@handle`, a `UC...` channel id, or any channel URL:

```sh
ytb channel @MrBeast
ytb channel UCX6OQ3DkcsbYNE6H8uQQuVA
ytb channel "https://www.youtube.com/@MrBeast"
```

The record is four blocks of the page merged into one thing.
The metadata block has the id, the keywords, the RSS url and the list of countries the channel is available in.
The microformat block has the crawler's view of the page and a schema.org ProfilePage.
The header has the handle, the verified or artist badge, and the banner.
The about panel is a second request, and it is the only place with the join date, the lifetime view count, the country written out, and the titles on the external links.

```sh
ytb channel @RickAstleyYT --no-about   # one request, no join date and no lifetime views
```

The external links come from `mainEntity.sameAs` in the ProfilePage, which lists all of them at once with no continuation, already unwrapped from YouTube's `youtube.com/redirect` wrapper.
The about panel is read anyway when it is not turned off, because that is where the link titles are.

Subscriber counts are always rounded to three significant figures, wherever they are read from, so every record carries `subscriber_count_is_approximate: true`.
There is no signed out surface that states an exact one.

### Counting the uploads

The header and the about panel do not always agree about how many videos a channel has, and neither of them always agrees with the uploads playlist.
`--counts` reads the four playlists derived from the channel id and prints the arithmetic:

```sh
$ ytb channel @RickAstleyYT --counts --fields handle,videos,counts
╭───────────────┬────────┬─────────────────────────────────────╮
│ HANDLE        │ VIDEOS │ COUNTS                              │
├───────────────┼────────┼─────────────────────────────────────┤
│ @RickAstleyYT │ 435    │ 139 + 294 + 2 = 435 uploads, agrees │
╰───────────────┴────────┴─────────────────────────────────────╯
```

Long form plus shorts plus past streams should come to the uploads playlist, because the three partition it.
The page said 433 videos while the uploads playlist holds 435, so the playlist wins and `via` records what it beat.
It costs four requests, which is why it is a flag and not the default.

## Uploads

```sh
ytb uploads @MrBeast                  # everything, newest first
ytb uploads @MrBeast --kind shorts    # just the shorts
ytb uploads @MrBeast --kind streams   # past live streams
ytb uploads @MrBeast --kind popular   # most viewed first
```

There are two routes to these rows and `--via` picks between them.
The default reads the playlist derived from the channel id, and the alternative reads the channel tab you would click on the site.
The playlist is the default because no tab lists a channel's uploads: the Videos tab is long form only, so `--kind all --via tab` has nothing to read and says so instead of returning less.

A listing row is a listing row either way.
It has a title, a duration, a rounded view count and a relative date like "4 days ago", and no description, no keywords and no like count.
`--enrich` fans out one `/player` call per row to fill those in:

```sh
ytb uploads @MrBeast --enrich -j 8
```

### Exact timestamps

"4 days ago" is what the listing gives you and it is all it gives you.
The channel's Atom feed states a publication time to the second and an exact view count, for the fifteen newest uploads and no further.
`--exact` reads it once and merges it in:

```sh
ytb uploads @RickAstleyYT --exact -n 20
```

Rows one to fifteen come back with a real timestamp.
Rows sixteen and on keep the rounded text and say in `missed` that the feed does not reach them.
If the newest fifteen are all you want, read the feed directly:

```sh
ytb feed @RickAstleyYT
```

The feed is also the only place outside the player that says whether a video is a short, which it does by linking to `/shorts/` instead of `/watch`.

### Paging and limits

`-n`/`--limit` caps the rows emitted and `--max-pages` caps the continuation pages fetched.
Both default to unlimited, so a bare `ytb uploads` on a big channel walks the whole history:

```sh
ytb uploads @MrBeast -n 200
ytb uploads @MrBeast -o jsonl > mrbeast.jsonl
```

Narrow the columns with `--fields`, using the lowercase column names from the table header:

```sh
ytb uploads @MrBeast --fields title,views,published
```

## Playlists

`playlists` lists what a channel has published:

```sh
ytb playlists @MrBeast
```

`playlist` takes a playlist id or URL and prints its header, and `items` streams the videos in it, each with its position:

```sh
ytb playlist PLFgquLnL59alCl_2TQvOiD5Vgm1hCaGSI
ytb items PLFgquLnL59alCl_2TQvOiD5Vgm1hCaGSI -n 100
```

### Resolving items to full metadata

Emit just the URLs with `-o url` and pipe them into `video -` to resolve each item to its full record:

```sh
ytb items PLFgquLnL59alCl_2TQvOiD5Vgm1hCaGSI -o url | ytb video -
```

## Persisting

These commands stream and keep nothing. `ytb crawl @handle` writes the channel, its playlists, the videos they name and the claims that connect them into the local store.
See [The store](/guides/the-store/).
