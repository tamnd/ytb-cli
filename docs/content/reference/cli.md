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
| `--yt-dlp-bin` | Path to the yt-dlp binary (download/extract) |

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
| `suggest` | Search autocomplete suggestions |
| `transcript` | Captions as text |
| `formats` | Streaming formats (metadata only) |
| `music` | YouTube Music (artists, albums, songs) |
| `download` | Download media via yt-dlp |
| `extract` | Extract a specific stream via yt-dlp |
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
| `--formats` | List streaming formats |
| `--related` | List related videos |
| `--transcript` | Fetch and attach the transcript text |
| `--lang` | Preferred caption language (default auto/English) |
| `--no-player` | Skip `/player` (HTML-only, faster) |
| `--raw` | Emit the full VideoResult as the value |

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

## suggest

`ytb suggest <query> [--flags]`. Search autocomplete suggestions. No notable flags beyond the globals.

## transcript

`ytb transcript <video-id|url> [--flags]`. Lists tracks (`--list`) or fetches the chosen track's text. When the raw caption endpoint is gated and yt-dlp is on PATH, the transcript is recovered through it automatically.

| Flag | Meaning |
| --- | --- |
| `--list` | List available caption tracks |
| `--lang` | Preferred caption language |
| `--timestamps` | Emit timed segments instead of joined text |

## formats

`ytb formats <video-id|url> [--flags]`. Lists formats from `/player` streamingData, deduped by itag. Metadata only; does not resolve playable URLs.

| Flag | Meaning |
| --- | --- |
| `--audio` | Audio-only adaptive formats |
| `--video` | Video-only adaptive formats |
| `--muxed` | Progressive (muxed) formats only |

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

`ytb download <id|url>... [--flags]`. Download media via yt-dlp.

| Flag | Meaning |
| --- | --- |
| `--audio` | Download best audio only |
| `--format` | yt-dlp format selector |
| `--quality` | Preferred video quality (e.g. 1080) |
| `--out` | Output directory (.) |
| `--add-metadata` | Embed metadata in the output file |
| `--concurrent-fragments` | Parallel fragment downloads |
| `--playlist-items` | Playlist item selection |
| `--sub-langs` | Subtitle languages to write |

## extract

`ytb extract <audio|video|transcript|all> <id|url> [--flags]`. Extract a specific stream via yt-dlp.

| Flag | Meaning |
| --- | --- |
| `--format` | Audio format for `extract audio` (e.g. mp3) |
| `--quality` | Max video height for `extract video` (e.g. 1080) |
| `--out` | Output directory (.) |

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
