---
title: "CLI"
description: "Every command and subcommand, with the flags that matter."
weight: 10
---

```
ytb <command> [subcommand] [flags]
```

Run `ytb <command> --help` for the full flag list on any command. This page is the map.

## Global flags

Persistent flags accepted by every command.

| Flag | Meaning |
| --- | --- |
| `-o, --output` | Output format: `list`, `table`, `markdown`, `json`, `jsonl`, `csv`, `tsv`, `url`, `raw` (auto) |
| `--fields` | Comma-separated columns to show |
| `-n, --limit` | Max rows emitted (`0` = unlimited) |
| `--max-pages` | Max continuation pages fetched (`0` = unlimited) |
| `-j, --workers` | Concurrency for detail fetches (4) |
| `--rate` | Minimum delay between requests (1.5s) |
| `--retries` | Retry attempts on 429/5xx (3) |
| `--timeout` | Per-request timeout (30s) |
| `--hl` | InnerTube interface language (en) |
| `--gl` | InnerTube content country (US) |
| `--db` | Tee every record into a generic store (e.g. `out.db`, `postgres://...`) |
| `-q, --quiet` | Suppress progress output |
| `-v, --verbose` | Increase verbosity (repeatable) |
| `--color` | Color output: `auto`, `always`, `never` (auto) |
| `--template` | Go text/template applied per row |
| `--no-header` | Omit the header row in table/csv/tsv |
| `--config` | Config file (default: XDG config) |
| `-y, --yes` | Assume yes to prompts |
| `--dry-run` | Print actions without performing them |
| `--yt-dlp-bin` | Path to the yt-dlp binary (`download --use-yt-dlp`) |
| `--ffmpeg-bin` | Path to ffmpeg (used to merge/convert when present) |

## Commands

| Command | What it does |
| --- | --- |
| `video` | Resolve one or more videos to full metadata |
| `channel` | A channel's record: header, about panel and external links |
| `about` | A channel's about panel on its own |
| `uploads` | Stream a channel's uploads |
| `feed` | A channel's Atom feed: the newest fifteen, with exact times |
| `playlists` | List a channel's playlists |
| `playlist` | Playlist header |
| `items` | Stream a playlist's videos |
| `search` | Search with the full filter grid |
| `trending` | What's hot right now |
| `comments` | Comments and replies |
| `community` | Community / posts tab |
| `hashtag` | A hashtag feed |
| `related` | The related-videos graph |
| `discover` | Breadth-first walk of the graph linked from a video, channel, or playlist |
| `suggest` | Search autocomplete suggestions |
| `transcript` | Captions as text |
| `formats` | Streaming formats (metadata only) |
| `music` | YouTube Music (artists, albums, songs) |
| `download` | Download media with the native engine (or yt-dlp) |
| `extract` | Extract a specific stream via yt-dlp |
| `sponsorblock` | List community SponsorBlock segments |
| `thumbnail` | List a video's thumbnails, or fetch the best one |
| `chapters` | List a video's chapter markers |
| `crawl` | Walk the graph from seeds into the store |
| `archive` | Write one read down in full: page, payloads, headers, and what ytb parsed |
| `query` | Run SQL over the store, read-only |
| `export` | Render the store as interlinked Markdown |
| `db` | The local SQLite store |
| `config` | View and manage configuration |
| `version` | Print version information |

## video

`ytb video <id|url>... [--flags]` resolves a video to full metadata (HTML bootstrap plus `/player` plus `/next`). Pass `-` to read ids/urls from stdin.

| Flag | Meaning |
| --- | --- |
| `--captions` | List available caption tracks |
| `--chapters` | List chapters |
| `--formats` | Attach the stream list, one extra request |
| `--related` | List related videos |
| `--transcript` | Fetch and attach the transcript text |
| `--lang` | Preferred caption language (default auto/English) |
| `--no-player` | Skip `/player` (HTML-only, faster) |
| `--raw` | Emit the full VideoResult as the value |

`formats` is off by default and stays off, because it costs a mobile player request per video and a crawl of a thousand videos should not make a thousand extra calls nobody asked for.
The watch page carries a stream list for free and a plain read still leaves it off: 29 formats nobody asked for bury the dozen fields the caller wanted.

## channel

`ytb channel <id|@handle|url> [--flags]`. Prints the channel record.

| Flag | Meaning |
| --- | --- |
| `--no-about` | Skip the about panel: one request fewer, and no join date, lifetime views or link titles |
| `--counts` | Read the four derived playlists and check that videos + shorts + streams = uploads (four extra requests) |

A channel page publishes its facts in four blocks that barely overlap, and the record is all four merged.
`channelMetadataRenderer` has the id, the keywords, the RSS url and the 249 country codes it will serve in.
`microformatDataRenderer` has the crawler's view plus a schema.org ProfilePage, and `mainEntity.sameAs` in that ProfilePage is the whole external link list with no continuation behind it, which is the cheapest place on the site to get a channel's links.
`pageHeaderViewModel` has the handle, the badge and the banner.
The about panel is a continuation and is the only source of the join date, the lifetime view count, the country in words and the link titles, which is what `--no-about` trades away.

Subscriber counts always carry `subscriber_count_is_approximate: true`.
The site rounds every one of them to three significant figures, including the number in the ProfilePage's `interactionStatistic`, so 1490000 means somewhere in the 1.49 millions and never means exactly that.

The header and the about panel can disagree with each other about the video count, and both can disagree with the uploads playlist.
The panel beats the header, `--counts` beats both, and `via` names the number that lost:

```
$ ytb channel @RickAstleyYT --counts --fields handle,videos,counts
╭───────────────┬────────┬─────────────────────────────────────╮
│ HANDLE        │ VIDEOS │ COUNTS                              │
├───────────────┼────────┼─────────────────────────────────────┤
│ @RickAstleyYT │ 435    │ 139 + 294 + 2 = 435 uploads, agrees │
╰───────────────┴────────┴─────────────────────────────────────╯
```

Those are the four playlists derived from the channel id: `UU` is every upload, `UULF` long form, `UUSH` shorts, `UULV` past live streams.
The three partition the first, so a mismatch means the site is mid-write or a video is in a state none of the three tabs claims.
The table shows the whole check as one `counts` cell; `-o json` carries the four ids, the four numbers and `agrees` separately.

## about

`ytb about <id|@handle|url>`. The about panel on its own: description, join date, lifetime views, country, and the external links with their titles.
No flags beyond the globals.

## uploads

`ytb uploads <id|@handle|url> [--flags]`. Streams a channel's uploads.

| Flag | Meaning |
| --- | --- |
| `--kind` | Which uploads: `all`, `videos`, `shorts`, `streams`, `popular` (default `all`) |
| `--via` | Read them from the derived `playlist` or from the channel `tab` (default `playlist`) |
| `--exact` | Cross-read the Atom feed so the newest fifteen get exact timestamps (one extra request) |
| `--enrich` | Call `/player` per video for full metadata |
| `--max-pages` | Max continuation pages (0 = unlimited) |

The playlist route is the default because it is the only one that answers `--kind all`.
No tab lists a channel's uploads: the Videos tab is long form only and returns exactly what `UULF` returns, so `--kind all --via tab` and `--kind popular --via tab` are usage errors rather than something that quietly returns less.

A shorts row comes back from the two routes with the same id and the same view count and a different `via`, because the two pages render the same view model differently.
The Shorts tab writes a readable `shorts-shelf-item-<id>` and a "22K views" overlay; the `UUSH` playlist writes an opaque hash and no overlay at all, and the only view count on the row is the one spelled out in its accessibility label, "22 thousand views".
Both are read, and `via` says which.

`--exact` fetches the Atom feed once and merges it into the newest fifteen rows.
Those get a timestamp to the second and an exact view count; everything after row fifteen keeps the rounded text and says so in `missed`, because the feed serves fifteen entries and has no continuation.

## feed

`ytb feed <id|@handle|url>`. The channel's Atom feed: fifteen entries, newest first, each with a publication time to the second and an exact view count.
No flags beyond the globals.

This is the only surface outside the player that says whether a video is a short, which it does by linking to `/shorts/<id>` instead of `/watch?v=`.
`media:starRating count` on an entry is a rating count and not a like count, so it is never written into `like_count`.

## playlists

`ytb playlists <id|@handle|url> [--flags]`. Lists the playlists a channel has published, from its Playlists tab.

## playlist

`ytb playlist <id|url>`. Prints the playlist header.

## items

`ytb items <playlist-id|url> [--flags]`. Streams a playlist's videos, each with its position.

## search

`ytb search <query> [--flags]`. Search with the full filter grid.

| Flag | Meaning |
| --- | --- |
| `--type` | `Video`, `Channel`, `Playlist` |
| `--duration` | `Short`, `Medium`, `Long` |
| `--upload-date` | `Hour`, `Today`, `Week`, `Month`, `Year` |
| `--sort` | `Relevance`, `Date`, `Views`, `Rating` |
| `--cc` | Closed captions / subtitles |
| `--creative-commons` | Creative Commons license |
| `--4k` | 4K only |
| `--hd` | HD only |
| `--hdr` | HDR only |
| `--live` | Live only |
| `--360` | 360-degree video |
| `--vr180` | VR180 only |

## trending

`ytb trending [--flags]`. Trending videos.

| Flag | Meaning |
| --- | --- |
| `--category` | `Music`, `Gaming`, `Movies`, `News` |

## comments

`ytb comments <video-id|url> [--flags]`. Comments and replies.

| Flag | Meaning |
| --- | --- |
| `--sort` | `Top`, `New` |
| `--replies` | Also fetch replies (parent_id set) |
| `--all` | Remove the default cap |

## community

`ytb community <channel-id|@handle> [--flags]`. Community / posts tab. No notable flags beyond the globals.

## hashtag

`ytb hashtag <tag> [--flags]`. A hashtag feed. No notable flags beyond the globals.

## related

`ytb related <video-id|url> [--flags]`. The related-videos graph. No notable flags beyond the globals.

## discover

`ytb discover <seed>... [--flags]` (aliases `walk`, `graph`). Walk the graph of linked objects breadth-first from one or more seeds (a video, channel, or playlist reference), streaming one row per node reached. See [graph discovery](/guides/graph-discovery/).

| Flag | Meaning |
| --- | --- |
| `--follow` | Edges to follow: a preset (`content`, `feed`, `comments`, `all`) or a comma-separated edge list (`channel`, `related`, `comments`, `uploads`, `playlists`, `community`, `items`, `owner`, `commenter`). Default `content` |
| `--depth` | Hops to follow from each seed (default `1`; `0` = seeds only) |
| `--fanout` | Max neighbors to follow per edge (default `25`; `0` = unlimited) |
| `--store` | Write every node reached into the local store: a record for what it fetched, a sighting for what it only saw |

The comment edges (`comments`, `commenter`) are served only when YouTube is not applying its per-IP Restricted Mode to this network; when it is, they are noted on stderr and skipped and the rest of the walk continues. `-n/--limit` is the total node budget (default `500`).

## suggest

`ytb suggest <query> [--flags]`. Search autocomplete suggestions. No notable flags beyond the globals.

## transcript

`ytb transcript <video-id|url> [--flags]`. Lists tracks (`--list`) or fetches the chosen track's text. When the raw caption endpoint is gated and yt-dlp is on PATH, the transcript is recovered through it automatically.

| Flag | Meaning |
| --- | --- |
| `--list` | List available caption tracks |
| `--lang` | Preferred caption language |
| `--timestamps` | Emit timed segments instead of joined text |
| `--format` | Render as subtitles: `srt`, `vtt`, `txt` |
| `--out` | Write the transcript/subtitles to this file |

## formats

`ytb formats <video-id|url> [--flags]`. Lists formats from `/player` streamingData, deduped by itag, audio first then video then muxed. Metadata only by default; pass `--urls` to resolve the deciphered, directly-fetchable stream URLs through the native engine.

The list is read from the ANDROID player, which is the only one that answers with plain URLs.
On the watch page every adaptive format arrives with a `contentLength` and neither a `url` nor a `signatureCipher`, so there is nothing to fetch, and the read says so when it has to fall back to it.

Under the table is a note naming the client that answered and when its URLs expire, and saying that fetching any of them without a `Range` header runs at 32 KiB/s where the same URL fetched in ranges runs at 4 MiB/s.
It goes to stderr, so `-o json` stays a clean stream.
A muxed format comes back from the mobile player with no `contentLength`, so its size column is empty and its note says `size unknown` rather than showing a 0 that reads as an empty file.

The table shows eight columns and `-o json` carries the whole record: `kind`, `container` and `codec` parsed off the mime type, `average_bitrate`, `audio_channels`, `audio_sample_rate`, `approx_duration_ms`, `init_range` and `index_range` for a DASH reader, `last_modified` for when the rendition was encoded, the `url` when a mobile player answered, `expires_at` off the URL's own `expire`, and `is_throttled_unranged`, which is true on every format because it is a fact about googlevideo rather than about the format.
A field that does not apply is absent rather than zero, so an audio format has no `width` and no `fps` instead of having them set to 0.

| Flag | Meaning |
| --- | --- |
| `--audio` | Audio-only adaptive formats |
| `--video` | Video-only adaptive formats |
| `--muxed` | Progressive (muxed) formats only |
| `--urls` | Resolve playable stream URLs (deciphered) via the native engine |

## music

`ytb music [command] [--flags]`. Search and browse YouTube Music via the WEB_REMIX client context.

| Subcommand | What it does |
| --- | --- |
| `search <query>` | Search artists, albums and songs |
| `artist <browseId|url>` | Artist profile with albums and top songs |
| `album <browseId|url>` | Album header and track list |
| `playlist <id|url>` | Music playlist and tracks |
| `song <video-id>` | Song detail (with `--lyrics` if available) |

Notable subcommand flags:

| Flag | Subcommand | Meaning |
| --- | --- | --- |
| `--type` | `music search` | `Song`, `Album`, `Artist`, `Playlist` |
| `--lyrics` | `music song` | Fetch lyrics if available |

## download

`ytb download <id|url>... [--flags]`. Downloads media with the built-in pure-Go engine.

The native engine fetches streams through the ANDROID_VR client (no API key, no token), deciphers the signature and the throttling `n` parameter with an embedded JavaScript interpreter, and downloads in parallel byte ranges. The `ytb` binary itself stays pure-Go and CGO-free; merging separate video+audio tracks, audio conversion, and thumbnail embedding shell out to ffmpeg when it is on PATH (or `--ffmpeg-bin` / `YTB_FFMPEG_BIN`). Without ffmpeg the engine still downloads any single progressive or adaptive stream, and commands that need merging exit with code 6. Pass `--use-yt-dlp` to delegate to a yt-dlp binary instead.

The `--format` selector accepts a yt-dlp-style grammar: keywords (`best`, `worst`, `bestvideo`/`bv`, `bestaudio`/`ba`, `bv*`), explicit itags (`22`), a single `+` to merge a video and audio track (`bv*+ba`, `137+140`), `/` fallback groups (`bv*+ba/b`), and `[key OP value]` filters on `height`, `width`, `fps`, `ext`, `vcodec`, `acodec`, `itag`, and bitrate (`bv*[height<=720]+ba`).

| Flag | Meaning |
| --- | --- |
| `-x, --audio` | Download audio only |
| `--audio-format` | Convert audio to this codec (`mp3`, `m4a`, `opus`, `flac`); needs ffmpeg |
| `-f, --format` | Format selector (e.g. `best`, `22`, `bv*+ba`, `bv[height<=720]+ba`) |
| `--quality` | Max video height shorthand (e.g. 1080) |
| `--out` | Output directory (.) |
| `--output-template` | yt-dlp-style output filename template (`%(title)s [%(id)s].%(ext)s`) |
| `--concurrent-fragments` | Parallel byte-range workers (4) |
| `--embed-thumbnail` | Embed the thumbnail as cover art (mp4/m4a); needs ffmpeg |
| `--playlist-items` | Playlist item selection (e.g. `1,3,5-7,10-`) |
| `--sub-langs` | Subtitle language to write (e.g. `en`) |
| `--sub-format` | Subtitle format to write (`srt`, `vtt`, `txt`) |
| `--write-subs` | Write the subtitle sidecar file |
| `--download-archive` | Record downloaded ids here and skip ones already present |
| `--use-yt-dlp` | Delegate to a yt-dlp binary instead of the native engine |

## extract

`ytb extract <audio|video|transcript|all> <id|url> [--flags]`. Extract a specific stream via yt-dlp.

| Flag | Meaning |
| --- | --- |
| `--format` | Audio format for `extract audio` (e.g. mp3) |
| `--quality` | Max video height for `extract video` (e.g. 1080) |
| `--out` | Output directory (.) |

## sponsorblock

`ytb sponsorblock <id|url> [--flags]`. Lists community-submitted segments from the public SponsorBlock API (sponsor, intros, outros, self-promo, and more). This is an independent community service; no key is required.

| Flag | Meaning |
| --- | --- |
| `--categories` | Segment categories to fetch (default all): `sponsor`, `selfpromo`, `intro`, `outro`, ... |

## thumbnail

`ytb thumbnail <id|url> [--flags]`. Lists the standard thumbnail renditions, or writes the best available one to disk.

Each of the five standard names can be constructed for any video id and only some of them exist, so the list is HEADed before it is printed and a rendition that answers 404 is left out.
A video with no `maxresdefault` answers that URL with a 1097 byte body that is still `Content-Type: image/jpeg`, so nothing but the status code separates a rendition from a placeholder.

| Flag | Meaning |
| --- | --- |
| `--fetch` | Write the best available rendition to disk |
| `--unconfirmed` | List the constructed URLs without HEADing them |
| `--out` | Output path or directory for `--fetch` |

## chapters

`ytb chapters <id|url>`. Lists a video's chapters: position, start time, title and origin. No notable flags beyond the globals.

`origin` is a column because a macro marker and a timestamped description are two different things that produce the same list.
`markers` means YouTube served a `macroMarkersListRenderer`, so the site itself treats these as chapters, draws them on the scrubber and has a preview frame for each.
`description` means somebody typed `1:23 Verse 2` and the list is only as good as their typing.
Both line layouts are read: the timestamp can come first, as in `0:00 Introduction`, or last, as in `Eyes To The Sky - 0:00:00`.

## crawl

`ytb crawl <seed>... [--flags]`. Read each seed, write its claims into the store, then read what those claims named, hop by hop, until the depth or the budget runs out.

The budget is counted in requests rather than estimated, and a cache hit makes no request so it costs nothing.
The frontier is every node the store has heard of and not read, which is a query and not a queue, so `--resume` with no seeds picks up where an earlier run stopped.
Mixes, a channel's popular playlist, comments and formats are off the frontier by default, though none of them is refused as a seed.

| Flag | Meaning |
| --- | --- |
| `--depth` | Hops to follow from each seed (default 1; 0 = seeds only) |
| `--budget` | Request budget for the whole crawl (default 200; 0 = no limit) |
| `--items` | Playlist items to read per playlist (default 100; 0 = header only) |
| `--comments` | Comments to read per video (default 0) |
| `--posts` | Community posts to read per channel (default 0) |
| `--captions` | Read each video's caption list (one ANDROID player call per video) |
| `--featured` | Read each channel's featured channels shelf |
| `--music` | Read each video through YouTube Music too |
| `--uploads` | Put each channel's derived playlists on the frontier (default true) |
| `--resume` | Start from the nodes the store has heard of and not read |
| `--manifest` | Where to write the manifest (default `<data-dir>/crawls/crawl-<unix>.json`) |

## archive

`ytb archive <ref> [--flags]`. Write one read down in full: the page, every InnerTube payload, a `meta.json` naming each request and the file its answer went into, and a `record.json` with the records and claims ytb parsed.

It needs the cache on, since it works by replaying what the cache stored, so `--no-cache` is refused.
Session headers are removed from `meta.json` before it is written.

| Flag | Meaning |
| --- | --- |
| `--dir` | Where to write (default `<data-dir>/archive/<ref>-<unix>`) |
| `--items` | Playlist items to read, for a playlist ref |
| `--comments` | Comments to read, for a video ref |
| `--captions` | Read the caption list too |

## query

`ytb query <sql>`. Run one SQL statement over the store and print the rows. The file is opened read-only, so a statement that would write is refused by SQLite itself rather than by a check in ytb. No notable flags beyond the globals.

```sh
ytb query "select predicate, count(*) c from claims group by 1 order by c desc"
ytb query "select uri from nodes where kind='video' and record is null limit 20"
```

## export

`ytb export [channel-id|@handle] [--flags]`. Render the stored data as an interlinked Markdown site. With no argument, every channel in the store is exported.

| Flag | Meaning |
| --- | --- |
| `--out` | Output directory for the Markdown site |

## db

`ytb db [command]`. Inspect the store at `<data-dir>/ytb.db`. Pure-Go, no cgo.

Three tables. `nodes` is everything with an identity, one row per URI, with the record as JSON and a null record for a node a claim named that nobody has fetched.
`claims` is the edges, one row per observation, so the same edge seen on the watch page and in a browse response is two rows.
`reads` is the log: every request, what answered it, and how big it was.

| Subcommand | What it does |
| --- | --- |
| `stats` | Nodes by kind, claims by predicate, reads by surface and client |
| `search <query>` | Full-text search over stored data |
| `path` | Print the db file location |
| `vacuum` | Compact the database file |
| `reset` | Drop and recreate all tables |

Notable subcommand flags:

| Flag | Subcommand | Meaning |
| --- | --- | --- |
| `--channels` | `db search` | Search channels instead of videos |

## config

`ytb config [command] [--flags]`. View and manage configuration.

| Subcommand | What it does |
| --- | --- |
| `show` | Print the resolved configuration |
| `path` | Print the config file path |
| `init` | Write a commented config template |
| `edit` | Open the config file in `$EDITOR` |

## version

`ytb version [--flags]`. Print version information.

| Flag | Meaning |
| --- | --- |
| `--short` | Print just the version number |
