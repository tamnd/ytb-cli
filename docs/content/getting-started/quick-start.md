---
title: "Quick start"
description: "A guided first run: metadata, a channel, a search, and a transcript, in a few commands."
weight: 30
---

You have [installed](/getting-started/installation/) `ytb` and it is on your
`PATH`. This page is a five-minute tour that ends with you pulling real data out
of YouTube. Nothing here needs an API key.

## 1. Resolve a video

Give ytb a video id or any watch URL:

```bash
ytb video dQw4w9WgXcQ
```

Writing to a terminal, list commands render an aligned table; piped, they switch
to JSONL automatically. Ask for the whole record as JSON:

```bash
ytb video dQw4w9WgXcQ -o json
```

You get the title, channel, view and like counts, publish date, duration,
category, tags, hashtags, the thumbnail, and more. Narrow it to the columns you
care about:

```bash
ytb video dQw4w9WgXcQ --fields title,channel,views -o table
```

## 2. Walk a channel

Point at a channel by handle, id, or URL and stream its uploads. ytb follows
the continuation tokens for you, so this keeps going until your limit is hit:

```bash
ytb uploads @RickAstleyYT -n 20
```

Add `--kind shorts` or `--kind streams` for those, `ytb playlists` for the channel's playlists, and `ytb channel` on its own for the channel record.

## 3. Search with filters

Search takes the same filter grid the site has:

```bash
ytb search "lofi hip hop" -n 25
ytb search "drone footage" --4k --duration long --sort date
ytb search "podcast" --type channel
```

Render just the URLs and pipe them straight into another command:

```bash
ytb search "go programming" -o url -n 10 | ytb video -
```

The trailing `-` tells `video` to read its arguments from stdin, one per line.

## 4. Read a transcript

List the caption tracks a video has:

```bash
ytb transcript dQw4w9WgXcQ --list
```

Fetch the text, or the timed segments:

```bash
ytb transcript dQw4w9WgXcQ
ytb transcript dQw4w9WgXcQ --timestamps
```

YouTube now gates the raw caption endpoint behind a proof-of-origin token, so a
direct text fetch often comes back empty. When that happens ytb falls back to
[yt-dlp](https://github.com/yt-dlp/yt-dlp) if it is on your `PATH` and parses the
result for you. Listing tracks never needs it. See
[comments and transcripts](/guides/comments-transcripts/) for the details.

## 5. Download a video

`ytb download` has a built-in pure-Go engine, so a basic grab needs no API key
and no external downloader:

```bash
ytb download dQw4w9WgXcQ
ytb download dQw4w9WgXcQ -x --audio-format mp3
```

The first saves the best combined stream; the second pulls audio only and
transcodes to mp3 (which uses ffmpeg if it is on your `PATH`). See
[downloading media](/guides/downloading/) for format selection, playlists, and
subtitles. Media download is your responsibility: respect YouTube's Terms of
Service and copyright.

## 6. Keep what you fetch

`ytb crawl` walks the graph from a seed and writes what it saw into a local
SQLite file under your data directory. Everything else just streams to stdout.

```bash
ytb crawl @RickAstleyYT --depth 2 --budget 200
ytb db stats
ytb query "select uri from nodes where kind='video' and record is null limit 5"
```

The nodes with no record are the frontier, so `ytb crawl --resume` picks up
where the budget ran out. See [the local store](/guides/the-store/).

## Where to go next

- The [guides](/guides/) go deep on each area: videos, channels and playlists,
  search, comments and transcripts, music, and the local store.
- The [CLI reference](/reference/cli/) is the complete command and flag surface.
- [Output formats](/reference/output/) covers `-o`, `--fields`, and `--template`
  in full.
