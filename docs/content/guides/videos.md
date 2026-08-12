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
ytb search "go programming" -o url | ytb video -
```

`-j`/`--workers` sets how many detail fetches run at once when you pass many ids.

## What one read already carries

Chapters, the description's links and hashtags, the thumbnail URLs and the
caption track list all come back on a plain read and need no flag. `-o json`
has them nested on the record.

The flags below are the parts that cost another request, so each one is a
decision rather than a default:

```sh
ytb video dQw4w9WgXcQ --captions      # caption tracks whose URLs fetch, one request
ytb video dQw4w9WgXcQ --formats       # the stream list, one request
ytb video dQw4w9WgXcQ --thumbnails    # a HEAD per constructed rendition
```

`--captions` is a narrower thing than it sounds. The plain read already lists
the tracks; what it lists are the watch page's URLs, and those answer HTTP 200
with an empty body. The flag re-reads the list through the mobile player, whose
URLs actually fetch, so the count is the same and the `base_url` is the part
that changed.

`--formats` stays off by default because it costs a mobile player request per
video, and a crawl of a thousand videos should not make a thousand extra calls
nobody asked for.

`--transcript` fetches the transcript text and attaches it to the record, and
`--lang` picks the language:

```sh
ytb video dQw4w9WgXcQ --transcript --lang es
```

`--microdata` adds what the page's own schema.org markup says, beside what ytb
parsed, which is how you check one against the other.

## Faster

`--no-player` never calls the mobile player, whatever else was asked, and
resolves from the HTML bootstrap only. It is faster, at the cost of the formats,
the caption list, and some player-only fields:

```sh
ytb video dQw4w9WgXcQ --no-player
```

## Sibling commands

Several commands print one part of a video on its own, which is handy when that
part is all you want:

The related-videos shelf is the one part of the watch page that does not land on
the video record, because it is twenty other videos rather than a fact about
this one. `related` prints it, `chapters` prints the chapter markers, and
`captions` prints the caption track list:

```sh
ytb related dQw4w9WgXcQ
ytb chapters GlYgs6v2YfU
ytb captions dQw4w9WgXcQ
```

`chapters` names where each chapter came from. `markers` means YouTube served a
chapter list of its own, so the site draws them on the scrubber; `description`
means somebody typed `1:23 Verse 2` and the list is only as good as their
typing.

`formats` lists the muxed and adaptive formats from `/player` streamingData,
deduped by itag. It lists metadata only and does not resolve playable URLs.
Filter by track type with `--audio` or `--video`, or show only progressive
streams with `--muxed`:

```sh
ytb formats dQw4w9WgXcQ --muxed
ytb formats dQw4w9WgXcQ --audio
```

## Persisting

These commands stream and keep nothing. To collect a video and what it links to,
run `ytb crawl <id>`, which writes the records and the claims into the local
store. See [The store](/guides/the-store/).
