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
| `-o, --output` | Output format: `table`, `json`, `jsonl`, `csv`, `tsv`, `url`, `id`, `raw` (auto) |
| `--fields` | Comma-separated columns to show |
| `-n, --limit` | Max rows emitted (`0` = unlimited) |
| `--max-pages` | Max continuation pages fetched (`0` = unlimited) |
| `-j, --workers` | Concurrency for detail fetches (4) |
| `--rate` | Minimum delay between requests (1.5s) |
| `--retries` | Retry attempts on 429/5xx (3) |
| `--timeout` | Per-request timeout (30s) |
| `--hl` | InnerTube interface language (en) |
| `--gl` | InnerTube content country (US) |
| `--db` | Optional SQLite store; persist everything fetched |
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
| `channel` | Channel metadata and its content |
| `playlist` | Playlist header and its items |
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
| `seed` | Load a worklist into the crawl queue (needs `--db`) |
| `crawl` | Process the crawl queue with workers (needs `--db`) |
| `queue` | Inspect the crawl queue (needs `--db`) |
| `jobs` | Recent crawl job history (needs `--db`) |
| `export` | Render the store as interlinked Markdown (needs `--db`) |
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

`ytb channel <id|@handle|url> [--flags]`. Without a tab flag, prints the channel record.

| Flag | Meaning |
| --- | --- |
| `--videos` | Stream the uploads tab |
| `--shorts` | Stream the Shorts tab |
| `--streams` | Stream the past live streams tab |
| `--playlists` | List the channel's playlists |
| `--enrich` | Call `/player` per video for full metadata |
| `--all` | Remove the default page cap |

## playlist

`ytb playlist <id|url> [--flags]`. Prints the playlist header.

| Flag | Meaning |
| --- | --- |
| `--videos` | Stream the playlist items |

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
| `--enqueue` | Push results into the crawl queue (needs `--db`) |

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
| `--store` | Persist every node into its typed table and each traversed edge into the `edges` table |

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

## seed

`ytb seed [item] [--flags]`. Enqueue items for the crawler (needs `--db`).

| Flag | Meaning |
| --- | --- |
| `--file` | Newline-delimited worklist file |
| `--entity` | Entity kind: `video`, `channel`, `playlist`, `search`, `hashtag`, `community` |
| `--priority` | Queue priority (higher runs first) |

## crawl

`ytb crawl [--flags]`. Process the crawl queue with workers (needs `--db`).

| Flag | Meaning |
| --- | --- |
| `--entity` | Only crawl one entity kind |
| `--max-per-item` | Cap items fetched per queue entry |

## queue

`ytb queue [--flags]`. Inspect the crawl queue (needs `--db`).

| Flag | Meaning |
| --- | --- |
| `--status` | Filter by status: `pending`, `done`, `failed` |

## jobs

`ytb jobs [--flags]`. Recent crawl job history (needs `--db`). No notable flags beyond the globals.

## export

`ytb export [channel-id|@handle] [--flags]`. Render the stored data as an interlinked Markdown site (needs `--db`). With no argument, every channel in the store is exported.

| Flag | Meaning |
| --- | --- |
| `--out` | Output directory for the Markdown site |

## db

`ytb db [command] [--flags]`. Inspect and query the optional SQLite store (`--db`). Pure-Go, no cgo.

| Subcommand | What it does |
| --- | --- |
| `stats` | Row counts per table |
| `query <sql>` | Run a read-only SQL query |
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
