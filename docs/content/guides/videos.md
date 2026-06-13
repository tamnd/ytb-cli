---
title: "Videos"
description: "Resolve a video to full metadata, chapters, formats, captions, and the related graph."
weight: 10
---

The `video` command resolves one or more videos to their full metadata in one
shot. It bootstraps a session from the watch page, then combines the HTML, the
`/player` response (description, dates, category, tags, formats, captions), and
the `/next` response (chapters, related videos, comment token) into a single
record.

```sh
ytb video dQw4w9WgXcQ
```

On a terminal this prints a table. Piped, it emits JSONL. Override with `-o`
(table, json, jsonl, csv, tsv, url, id, raw). See [Output formats](/reference/output/)
for the full list, and the [CLI reference](/reference/cli/) for every flag.

## What it returns

The resolved record carries the fields you see in the browser: title, channel
name and id, view count, like count, publish and upload dates, duration,
category, tags, the full description, thumbnails, and the live/short markers.
The `--player`-derived sections (formats and caption tracks) and the
`/next`-derived sections (chapters, related videos) are attached too, and the
sub-flags below pull each of those out as its own row stream.

Narrow the table columns with `--fields` (lowercase column names, not JSON
keys):

```sh
ytb video dQw4w9WgXcQ --fields title,channel,views -o table
```

## Many ids, and stdin

`video` takes more than one id or URL. Mix bare ids and full watch URLs freely:

```sh
ytb video dQw4w9WgXcQ "https://www.youtube.com/watch?v=9bZkp7q19f0"
```

Pass `-` to read ids or URLs from stdin, one per line. This is how the other
commands chain into `video`:

```sh
ytb search "go programming" -o id | ytb video -
```

`-j`/`--workers` sets how many detail fetches run at once when you pass many ids.

## Sub-views of one video

The sub-flags switch `video` from printing one metadata row to streaming the
matching sub-list instead:

```sh
ytb video dQw4w9WgXcQ --chapters    # chapter list (title and start time)
ytb video dQw4w9WgXcQ --captions    # available caption tracks
ytb video dQw4w9WgXcQ --formats     # streaming formats, deduped by itag
ytb video dQw4w9WgXcQ --related     # the related-videos graph
```

`--transcript` fetches the transcript text and attaches it to the record.
`--lang` picks the preferred caption language (it defaults to auto, English):

```sh
ytb video dQw4w9WgXcQ --transcript --lang es
```

## Faster and rawer

`--no-player` skips the `/player` call and resolves from the HTML bootstrap
only. It is faster, at the cost of the formats, captions, and some `/player`
exclusive fields:

```sh
ytb video dQw4w9WgXcQ --no-player
```

`--raw` emits the full `VideoResult` as the value, the whole parsed model rather
than the projected row:

```sh
ytb video dQw4w9WgXcQ --raw -o json
```

## Sibling commands

Two commands operate on a single video and overlap with the `video` sub-flags,
handy when that is all you want:

`related` prints the related-videos graph for one video:

```sh
ytb related dQw4w9WgXcQ
```

`formats` lists the muxed and adaptive formats from `/player` streamingData,
deduped by itag. It lists metadata only and does not resolve playable URLs.
Filter by track type with `--audio` or `--video`, or show only progressive
streams with `--muxed`:

```sh
ytb formats dQw4w9WgXcQ --muxed
ytb formats dQw4w9WgXcQ --audio
```

## Persisting

Add `--db <path>` to any of these and ytb writes everything it fetches into
a local SQLite store as it streams. See [The store](/guides/the-store/).
