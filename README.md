# ytb

A fast, friendly command line for [YouTube](https://www.youtube.com). One binary
that resolves a video to its full metadata, streams a channel's uploads, pages a
playlist, searches with the full filter grid, pulls comments and transcripts,
follows hashtags and community posts, browses YouTube Music, and optionally
persists everything into a local SQLite store.

```
ytb video dQw4w9WgXcQ --fields title,channel,views -o table
```

```
TITLE                                                  CHANNEL      VIEWS
Rick Astley - Never Gonna Give You Up (Official Video) Rick Astley  1782393180
```

Full documentation: [ytb-cli.tamnd.com](https://ytb-cli.tamnd.com).

## Why

Working with YouTube programmatically usually means an API key, a quota that runs
out by lunch, and a Data API whose shape barely resembles what you see in the
browser. ytb talks to the same public InnerTube endpoints the site itself
uses, so there is no key to register and no quota to budget. It puts the whole
surface (videos, channels, playlists, search, comments, transcripts, hashtags,
community posts, and YouTube Music) behind one tool with real output formats and
pipelines that compose.

## Install

```sh
go install github.com/tamnd/ytb-cli/cmd/ytb@latest
```

Or grab a prebuilt binary from the [releases page](https://github.com/tamnd/ytb-cli/releases).
The binary is pure Go with no runtime dependencies. yt-dlp is optional and only
needed for the `download`/`extract` commands and for recovering transcripts when
YouTube gates the caption endpoints (see [Transcripts](#transcripts)).

Build from source:

```sh
git clone https://github.com/tamnd/ytb-cli
cd ytb-cli
make build      # produces ./bin/ytb
```

## Quick start

```sh
ytb video dQw4w9WgXcQ                  # full metadata for a video
ytb channel @MrBeast --videos          # stream a channel's uploads
ytb playlist PLFgquLnL59alCl_2TQvOiD5Vgm1hCaGSI  # page a playlist's items
ytb search "lofi hip hop" -n 50        # search with continuation paging
ytb transcript dQw4w9WgXcQ             # the video's transcript as text
ytb trending --category music          # what's hot right now
```

Anywhere a command takes an id or URL it also accepts `-` to read a list from
stdin, so commands chain:

```sh
ytb search "go programming" -o id | ytb video -   # batch lookups
```

## How it works

YouTube renders every page from a JSON document (`ytInitialData`) and serves the
same data to its own JavaScript through the InnerTube API at
`youtubei/v1/*`, the endpoints behind `/player`, `/next`, `/browse`, and
`/search`. ytb bootstraps a session from a watch page (visitor data, client
version), then walks those renderer trees the same way the website does, follows
the opaque continuation tokens for pagination, and maps everything onto clean
data models. No API key, no quota.

## Commands

| Command | What it does |
| --- | --- |
| `video` | Resolve one or more videos to full metadata |
| `channel` | Channel metadata and its content (videos, shorts, streams, playlists) |
| `playlist` | Playlist header and its items |
| `search` | Search with the full filter grid (type, duration, features, sort) |
| `trending` | What is hot right now, by category |
| `comments` | Comments and replies for a video |
| `community` | A channel's community / posts tab |
| `hashtag` | A hashtag feed |
| `related` | The related-videos graph for a video |
| `suggest` | Search autocomplete suggestions |
| `transcript` | Caption tracks and transcript text |
| `formats` | Streaming format metadata for a video |
| `music` | YouTube Music: search, artists, albums, playlists, songs |
| `download` | Download media via yt-dlp |
| `extract` | Extract a specific stream (audio, video, transcript) via yt-dlp |
| `seed` | Load a worklist into the crawl queue (needs `--db`) |
| `crawl` | Process the crawl queue with workers (needs `--db`) |
| `queue` | Inspect the crawl queue (needs `--db`) |
| `jobs` | Recent crawl job history (needs `--db`) |
| `export` | Render the store as interlinked Markdown (needs `--db`) |
| `db` | The local SQLite store: stats, query, search, vacuum |
| `config` | Show and manage configuration |

Run `ytb <command> --help` for the full flag list on any command.

## Recipes

Pull a channel's entire upload history as JSONL:

```sh
ytb channel @MrBeast --videos -o jsonl > mrbeast.jsonl
```

Get the title and view count of every video a search returns:

```sh
ytb search "rust tutorial" -n 100 --fields title,views
```

Read the transcript of a video and count the words:

```sh
ytb transcript dQw4w9WgXcQ | wc -w
```

Find a playlist's videos and resolve each to full metadata:

```sh
ytb playlist PLFgquLnL59alCl_2TQvOiD5Vgm1hCaGSI -o id | ytb video -
```

Search only for long, 4K, creative-commons videos sorted by date:

```sh
ytb search drone --duration long --4k --creative-commons --sort date
```

Pull the top-level comments of a video, newest first:

```sh
ytb comments dQw4w9WgXcQ --sort new -n 50
```

## Output formats

Every list command renders through the same formatter. Pick a format with `-o`,
or let ytb choose: a table when writing to a terminal, JSONL when piped.

```sh
ytb search example -o table   # aligned columns for reading
ytb search example -o jsonl   # one JSON object per line, for piping
ytb search example -o json    # a single JSON array
ytb search example -o csv     # spreadsheet friendly
ytb search example -o url     # just the canonical URL
ytb search example -o id      # just the id, ideal for stdin chaining
```

Narrow the columns with `--fields`, or template each row:

```sh
ytb search example --fields title,channel,views
ytb search example --template '{{.Title}} — {{.Views}}'
```

## The local store

Most commands are stateless and stream straight to stdout. Pass `--db <path>`
and ytb also persists everything it fetches into a SQLite database: videos,
channels, playlists, comments, caption tracks, formats, and the relationships
between them. That turns the same commands into a crawler and gives you SQL over
what you have collected.

```sh
ytb channel @MrBeast --videos --db yt.db    # stream and persist in one pass
ytb db stats --db yt.db                      # row counts per table
ytb db search videos "mukbang" --db yt.db    # full-text over stored titles
ytb db query "select title, views from videos order by views desc limit 10" --db yt.db
ytb export @MrBeast --db yt.db --out site/   # render the store as Markdown
```

For larger collection runs, the `seed`/`crawl`/`queue`/`jobs` commands turn the
store into a work queue that a pool of workers drains:

```sh
ytb search "podcast" -o id --enqueue --db yt.db   # seed the queue from a search
ytb crawl --db yt.db -j 8                          # drain it with 8 workers
ytb queue --db yt.db                               # see what is pending
```

The store is pure Go (modernc.org/sqlite), so nothing links libsqlite and the
binary stays static. Without `--db`, no database is ever created.

## Transcripts

`ytb transcript <video> --list` shows every caption track a video has.
Fetching the text is harder than it used to be: YouTube now gates the raw
`timedtext` endpoint behind a proof-of-origin token, so direct text fetches often
come back empty. When that happens and `yt-dlp` is on your `PATH`, ytb
recovers the transcript through it automatically and parses the result back into
timed segments:

```sh
ytb transcript dQw4w9WgXcQ --list           # available tracks
ytb transcript dQw4w9WgXcQ                   # joined text
ytb transcript dQw4w9WgXcQ --timestamps      # {start, dur, text} segments
ytb transcript dQw4w9WgXcQ --lang es         # a specific language
```

Install yt-dlp from [its releases](https://github.com/yt-dlp/yt-dlp) if you want
transcript text and media downloads. Everything else works without it.

## Configuration

ytb needs no configuration to run. Defaults live in code; a config file is
optional and mirrors the global flags. See the resolved settings any time:

```sh
ytb config show
ytb config init     # write a commented template to the XDG config path
```

Useful global flags (all have sensible defaults):

| Flag | Meaning |
| --- | --- |
| `-o, --output` | Output format (default auto) |
| `-n, --limit` | Maximum rows emitted (`0` means unlimited) |
| `--max-pages` | Maximum continuation pages to fetch (`0` means unlimited) |
| `-j, --workers` | Concurrency for detail fetches and the crawler |
| `--rate` | Minimum delay between requests, to stay polite |
| `--hl, --gl` | InnerTube interface language and content country |
| `--db` | Path to the optional SQLite store |
| `--fields`, `--template` | Narrow or reshape each emitted row |

## Development

```sh
make test    # run the test suite
make vet     # go vet
make build   # build ./bin/ytb
```

The code is two packages: `ytb/` is the library (client, InnerTube transport,
renderer-walking parsers, data models, optional store), and `cli/` is the command
tree built on Cobra and [fang](https://github.com/charmbracelet/fang). The
library has no dependency on the CLI, so it is usable on its own.

## License

[Apache 2.0](LICENSE).

YouTube is a trademark of Google LLC. This project is an independent client that
talks to publicly served endpoints and is not affiliated with or endorsed by
YouTube or Google. Respect [YouTube's Terms of Service](https://www.youtube.com/t/terms)
and the rights of content owners when you use it.
