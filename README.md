# ytb

[![CI](https://github.com/tamnd/ytb-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/tamnd/ytb-cli/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/tamnd/ytb-cli)](https://github.com/tamnd/ytb-cli/releases/latest)
[![Go Reference](https://pkg.go.dev/badge/github.com/tamnd/ytb-cli.svg)](https://pkg.go.dev/github.com/tamnd/ytb-cli)
[![Go Report Card](https://goreportcard.com/badge/github.com/tamnd/ytb-cli)](https://goreportcard.com/report/github.com/tamnd/ytb-cli)
[![License](https://img.shields.io/github/license/tamnd/ytb-cli)](./LICENSE)

A command line for [YouTube](https://www.youtube.com). `ytb` resolves any video,
channel, playlist, comment thread, transcript, or YouTube Music record into clean
structured data. One pure-Go binary, no API key, no quota.

[Install](#install) • [Commands](#commands) • [Usage](#usage) • [The local store](#the-local-store)

![ytb searching YouTube and reading a video record from the command line](docs/static/demo.gif)

It talks to the same public InnerTube endpoints the YouTube site uses, so there
is no key to register and no quota to budget. Responses are cached on disk, so a
repeat call is instant. `ytb crawl` walks the graph into a local SQLite store you
can query with SQL.

`ytb` is an independent tool. It is not affiliated with YouTube or Google.

## Install

```bash
go install github.com/tamnd/ytb-cli/cmd/ytb@latest
```

Or grab a prebuilt binary, a Linux package (`deb`/`rpm`/`apk`), or a container
image from the [releases](https://github.com/tamnd/ytb-cli/releases):

```bash
brew install tamnd/tap/ytb
docker run --rm ghcr.io/tamnd/ytb:latest search 'lofi hip hop' -n 10
```

Shell completion is built in: `ytb completion bash|zsh|fish|powershell`.

`ytb download` uses a native pure-Go engine. `yt-dlp` is optional and only needed
for `extract`, for `download --use-yt-dlp`, and as a transcript fallback when
YouTube gates the caption endpoints.

## Commands

| Command | Reads |
| --- | --- |
| `ytb video <id\|url>...` | one or more videos; full metadata |
| `ytb channel <handle\|url>` | a channel's record; `--counts`, `--no-about` |
| `ytb about <handle\|url>` | a channel's about panel: links, country, join date |
| `ytb uploads <handle\|url>` | a channel's uploads; `--kind`, `--via`, `--exact` |
| `ytb feed <handle\|url>` | the newest fifteen, with exact timestamps |
| `ytb playlists <handle\|url>` | a channel's playlists |
| `ytb playlist <id\|url>` | a playlist's header |
| `ytb items <id\|url>` | a playlist's videos |
| `ytb search <query>` | search with type, duration, features, and sort filters |
| `ytb trending` | what is hot right now; `--category` |
| `ytb comments <id\|url>` | a video's comments and replies; `--sort` |
| `ytb community <handle\|url>` | a channel's community / posts tab |
| `ytb hashtag <tag>` | a hashtag feed |
| `ytb related <id\|url>` | related videos for a video |
| `ytb suggest <term>` | search autocomplete terms |
| `ytb transcript <id\|url>` | caption tracks and transcript text; `--timestamps`, `--lang` |
| `ytb formats <id\|url>` | streaming format metadata; `--audio`, `--video`, `--muxed` |
| `ytb captions <id\|url>` | the caption tracks a video has, and which of them fetch |
| `ytb chapters <id\|url>` | a video's chapters, and where each one came from |
| `ytb thumbnail <id\|url>` | thumbnail renditions, confirmed with a HEAD; `--fetch` |
| `ytb sponsorblock <id\|url>` | community SponsorBlock segments; `--categories` |
| `ytb music search <query>` | YouTube Music search |
| `ytb music artist <id\|url>` | a Music artist's profile and releases |
| `ytb music album <id\|url>` | a Music album |
| `ytb music playlist <id\|url>` | a Music playlist |
| `ytb music track <id\|url>` | a Music track; `--lyrics` |
| `ytb download <id\|url>` | download media via yt-dlp |
| `ytb extract <id\|url>` | extract a specific stream via yt-dlp; `--audio`, `--video` |
| `ytb crawl <seed>...` | walk the graph from seeds into the store; `--depth`, `--budget`, `--resume` |
| `ytb archive <id\|url>` | write one read down in full: page, payloads, headers, and what ytb parsed |
| `ytb edges <id\|url>...` | the claims one read makes: subject, predicate, object, and who said so |
| `ytb rdf <id\|url>...` | the same claims as n-triples, turtle, or json-ld |
| `ytb graph <seed>...` | follow the frontier those claims name; `--depth`, `--budget` |
| `ytb discover <seed>...` | breadth-first walk from a video, channel, or playlist |
| `ytb predicates` | the closed vocabulary: every predicate, its domain and range |
| `ytb id <ref>` | classify any id, handle, or URL, with no request at all |
| `ytb query <sql>` | run SQL over the store, read-only |
| `ytb export <handle\|id>` | render the store as interlinked Markdown |
| `ytb db stats\|search\|path\|vacuum\|reset` | work with the local SQLite store |
| `ytb config show\|init\|path` | show or reset configuration |
| `ytb cache path\|info\|clear` | inspect or clear the on-disk cache |
| `ytb serve` | one HTTP route per read, NDJSON, plus a generated OpenAPI spec |
| `ytb mcp` | the same reads as MCP tools over stdio |
| `ytb version` | print version, commit, and build date |

Full reference and guides live at [ytb-cli.tamnd.com](https://ytb-cli.tamnd.com).

## Usage

```bash
ytb video dQw4w9WgXcQ                        # full video metadata
ytb channel @MrBeast                         # a channel's record
ytb uploads @MrBeast -n 20                   # a channel's uploads
ytb search 'lofi hip hop' -n 50              # search
ytb comments dQw4w9WgXcQ --sort new -n 100   # newest 100 comments
ytb transcript dQw4w9WgXcQ                   # transcript as text
ytb trending --category music                # what is hot right now
ytb music search 'rick astley'               # YouTube Music search
```

Records come out as a table (the default on a terminal), list, markdown, JSON,
JSONL, CSV, TSV, url, or raw. The table uses rounded borders and a colored header
on a true-color terminal; JSON and JSONL are syntax-highlighted too:

```bash
ytb search 'lofi hip hop' --fields id,title,channel,views -o table
ytb video dQw4w9WgXcQ -o json
ytb search 'go' -n 50 -o jsonl | jq 'select(.views > 100000)'
ytb search 'go' -o url
ytb uploads @MrBeast -o jsonl > mrbeast.jsonl
ytb items PLFgquLnL59alCl_2TQvOiD5Vgm1hCaGSI -o url | ytb video -
```

Chain commands through stdin with `-` for batch lookups:

```bash
ytb search 'go programming' -o url | ytb video -
```

### Global flags

```
-o, --output       list|table|markdown|json|jsonl|csv|tsv|url|raw      (auto: table on a TTY, jsonl when piped)
    --fields       comma-separated columns to keep, in order
    --no-header    omit the header row
    --template     Go text/template applied per record
-n, --limit        max records (0 = unlimited)
    --max-pages    max continuation pages (0 = unlimited)
-j, --workers      concurrency for detail fetches (default 4)
    --rate         min delay between requests (default 500ms)
    --timeout      per-request timeout (default 30s)
    --retries      retry attempts on 429/5xx (default 4)
    --hl           InnerTube interface language (default en)
    --gl           InnerTube content country (default US)
-q, --quiet        suppress progress output
    --color        auto|always|never
    --db           tee every record into a store (e.g. out.db, postgres://...)
    --data-dir     override the data directory, which is where the store lives
    --no-cache     bypass the on-disk cache
    --dry-run      print the requests that would be made
```

## The local store

`ytb crawl` walks the graph from a seed and writes what it saw into a SQLite file
at `<data-dir>/ytb.db`, which `ytb db path` will print. There are three tables.
`nodes` is everything with an identity, one row per URI, with the record as JSON
and a null record for a node somebody named that nobody has fetched yet. `claims`
is the edges, one row per observation, so the same edge seen on the watch page
and in a browse response is two rows and each says where it came from. `reads` is
the log: every request, what answered it, and how big it was.

```bash
ytb crawl @MrBeast --depth 2 --budget 200     # walk the graph into the store
ytb crawl --resume --budget 50                # keep going where it stopped
ytb db stats                                  # nodes by kind, claims by predicate
ytb db search "lofi"                          # full-text search over stored videos
ytb export @MrBeast --out site/               # render the store as Markdown
```

The unread nodes are the frontier, which is a query rather than a queue, so a
crawl that stops is just a crawl with rows left to read:

```bash
ytb query "select uri from nodes where record is null and kind='video' limit 20"
ytb query "select predicate, count(*) c from claims group by 1 order by c desc"
```

`ytb query` opens the file read-only, so a statement that would write is refused
by SQLite itself. To keep the raw bytes as well, `ytb archive <id>` writes one
read into a directory: the page, every InnerTube payload, the request headers
with the session ones removed, and the records and claims ytb parsed out of them.

## Serve and MCP

Every read is one registration, and that registration is three surfaces: the command, an HTTP route, and an MCP tool.
Neither server has a read the command line lacks, and neither can be missing one, because there is no second implementation to keep in step.

```bash
ytb serve --addr 127.0.0.1:8080                              # 31 routes, NDJSON
curl -s localhost:8080/v1/video?ref=dQw4w9WgXcQ
curl -s "localhost:8080/v1/uploads?ref=@RickAstleyYT&kind=shorts&limit=5"
curl -s localhost:8080/v1/openapi.json                       # the generated spec
ytb mcp                                                      # the same set on stdio
```

Arguments go in the query as well as on the path, which is the form to use, because a path splits on slashes and half of what you pass ytb is a URL.
Flags keep the names the command gives them: `--max-pages 3` is `&max-pages=3`.
Nothing that writes is served, so `download`, `crawl`, `export`, `db` and `config` stay on the command line where they belong.
[Reference](https://ytb-cli.tamnd.com/reference/serve-and-mcp/).

## Exit codes

```
0  success
1  error
2  usage error
3  no results
4  auth required
5  rate limited
6  not found
7  unsupported (missing optional tool such as yt-dlp)
```

## Development

```
cmd/ytb/     thin main entry point
cli/         commands and output rendering
youtube/     HTTP client, InnerTube transport, parsers, models, crawl, store
pkg/ytid/    id and URL classification
pkg/graph/   URIs, predicates, and the claim vocabulary
pkg/rdf/     n-triples, turtle, and json-ld writers
pkg/srv3/    caption track parsing
docs/        documentation site (Hugo, tago-doks theme)
```

```bash
make build   # ./bin/ytb
make test    # go test ./...
make vet     # go vet ./...
make fmt     # gofmt -s -w .
```

Requires Go 1.26+. yt-dlp is optional; install it from
[its releases](https://github.com/yt-dlp/yt-dlp) if you want `extract`,
`download --use-yt-dlp`, and transcript recovery.

## Releasing

Push a version tag and GitHub Actions runs GoReleaser:

```bash
git tag -a v0.3.2 -m "v0.3.2"
git push --tags
```

The image tag carries no `v` prefix (`ghcr.io/tamnd/ytb:0.3.2`).

## License

Apache-2.0. See [LICENSE](LICENSE).

`ytb` is an independent client. Use it to access public data responsibly and
within YouTube's Terms of Service. YouTube is a trademark of Google LLC.
