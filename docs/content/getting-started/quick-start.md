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
ytb channel @RickAstleyYT --videos -n 20
```

Swap `--videos` for `--shorts`, `--streams`, or `--playlists` to walk the other
tabs.

## 3. Search with filters

Search takes the same filter grid the site has:

```bash
ytb search "lofi hip hop" -n 25
ytb search "drone footage" --4k --duration long --sort date
ytb search "podcast" --type channel
```

Render just the ids and pipe them straight into another command:

```bash
ytb search "go programming" -o id -n 10 | ytb video -
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

Add `--db` to any command and ytb also writes everything into a local SQLite
database, turning the same commands into an incremental crawler:

```bash
ytb channel @RickAstleyYT --videos --db yt.db
ytb db stats --db yt.db
ytb db query "select title, view_count from videos order by view_count desc limit 5" --db yt.db
```

Without `--db`, no database is created and everything just streams to stdout.

## Where to go next

- The [guides](/guides/) go deep on each area: videos, channels and playlists,
  search, comments and transcripts, music, and the local store.
- The [CLI reference](/reference/cli/) is the complete command and flag surface.
- [Output formats](/reference/output/) covers `-o`, `--fields`, and `--template`
  in full.
