---
title: "Search"
description: "Search with the full filter grid: type, duration, sort order, and feature flags."
weight: 30
---

`search` runs a query against YouTube's public search endpoint and walks the
result renderers the same way the site does. It exposes the whole filter grid
you would otherwise set through the search UI: result type, duration buckets,
sort order, and the feature flags (HD, 4K, captions, live, and so on).

```sh
ytb search "lofi hip hop"
```

Output is automatic: an aligned table on a terminal, JSONL when piped. Override
it with `-o` (`table`, `json`, `jsonl`, `csv`, `tsv`, `url`, `id`, `raw`).

## Filters

Combine any of these flags. They map onto YouTube's own search filter chips.

| Flag | Values | What it filters |
| --- | --- | --- |
| `--type` | `video`, `channel`, `playlist` | Restrict results to one kind |
| `--duration` | `short`, `medium`, `long` | Length bucket (videos only) |
| `--sort` | `relevance`, `date`, `views`, `rating` | Result ordering |
| `--upload-date` | `hour`, `today`, `week`, `month`, `year` | How recently uploaded |

Feature flags are boolean and stack freely:

| Flag | Meaning |
| --- | --- |
| `--hd` | HD only |
| `--4k` | 4K only |
| `--cc` | Has closed captions / subtitles |
| `--live` | Live only |
| `--360` | 360-degree video |
| `--hdr` | HDR only |
| `--creative-commons` | Creative Commons license |

Find long, 4K, Creative Commons videos sorted by date:

```sh
ytb search drone --duration long --4k --creative-commons --sort date
```

Channels uploaded in the last week, captioned and HD:

```sh
ytb search "rust async" --type video --upload-date week --cc --hd
```

## Paging

Search returns one page at a time and YouTube hands back an opaque continuation
token for the next. `search` follows those tokens for you. Two flags bound how
far it goes:

- `-n, --limit` caps the total rows emitted (`0` means unlimited).
- `--max-pages` caps how many continuation pages are fetched (`0` means
  unlimited).

```sh
ytb search "lofi hip hop" -n 50          # stop after 50 rows
ytb search "lofi hip hop" --max-pages 3  # fetch at most 3 pages
```

## Piping ids into other commands

Use `-o id` to emit just the video id per line, then feed it to `video -`,
which reads ids from stdin and resolves each to full metadata:

```sh
ytb search "go programming" -o id | ytb video -
```

The same works for `--fields` to keep the columns you care about:

```sh
ytb search "rust tutorial" -n 100 --fields title,views
```

## Enqueue into the store

With a SQLite store attached (`--db`), `--enqueue` pushes the search results
into the crawl queue instead of relying on a separate seeding step. A pool of
workers can then drain the queue later. See [The store](/guides/the-store/) for
the full crawl workflow.

```sh
ytb search "podcast" -o id --enqueue --db yt.db
```

## Related discovery commands

### trending

`trending` lists what is hot right now. `--category` narrows it to one tab.

```sh
ytb trending
ytb trending --category music   # music|gaming|movies|news
```

It pages through continuations like `search`, so `-n` and `--max-pages` apply.

### suggest

`suggest` returns search autocomplete suggestions for a query, one per line.
Handy for expanding a seed term before a real search.

```sh
ytb suggest "how to"
```

### hashtag

`hashtag` streams the feed for a tag (pass it with or without the leading `#`).

```sh
ytb hashtag minecraft
```

### community

`community` reads a channel's community / posts tab. It accepts a channel id or
`@handle`.

```sh
ytb community @MrBeast
```
