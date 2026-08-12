---
title: "Configuration"
description: "The config file, environment, and the global flags that tune every command."
weight: 20
---

ytb needs no configuration to run. Every default lives in the binary, so a
fresh install works the moment it lands on your PATH. A config file is purely
optional and exists only to change those defaults once instead of typing the
same flags every time.

## Managing the config file

Four subcommands cover the whole lifecycle:

```sh
ytb config show     # print the resolved configuration ytb is using
ytb config path     # print the config file path
ytb config init     # write a commented template to that path
ytb config edit     # open the file in $EDITOR (vi if unset)
```

`config init` writes the template only after asking before it overwrites an
existing file. `config edit` creates the file from the template first if it does
not exist yet.

## Where the file lives

The path follows the XDG config convention (Go's `os.UserConfigDir`, then
`ytb/config.toml`):

| OS | Path |
| --- | --- |
| Linux | `~/.config/ytb/config.toml` |
| macOS | `~/Library/Application Support/ytb/config.toml` |

There is one path and no flag that moves it, so `ytb config path` always names
the file that is actually being read. `--data-dir` moves the store and the
cache, not the config.

## The file format

The file is TOML. Its keys mirror the global flags, so anything you can pass on
the command line you can set once here. This is the template `config init`
writes:

```toml
# ytb CLI configuration
# Keys mirror the global flags. Uncomment to override the built-in defaults.

# output    = "auto"      # table|json|jsonl|csv|tsv|url|id|raw
# workers   = 4
# rate      = "1.5s"
# retries   = 3
# timeout   = "30s"
# hl        = "en"
# gl        = "US"
# db        = ""           # tee every record into a generic store
# user_agent = ""
# yt_dlp_bin = "yt-dlp"
```

## ffmpeg

`download` uses a built-in pure-Go engine, so basic downloads need no external
tool. ffmpeg is used only to merge a separate video track with its audio, convert
audio with `--audio-format`, and embed cover art with `--embed-thumbnail`. ytb
looks for `ffmpeg` on your PATH by default. Point at a specific binary two ways,
in order of precedence:

1. the `--ffmpeg-bin` flag
2. the `YTB_FFMPEG_BIN` environment variable

```sh
ytb download dQw4w9WgXcQ -f bv*+ba --ffmpeg-bin /opt/homebrew/bin/ffmpeg
export YTB_FFMPEG_BIN=/opt/homebrew/bin/ffmpeg
```

## yt-dlp

yt-dlp is optional. The `download --use-yt-dlp` and `extract` commands delegate to
it, and `transcript` falls back to it when YouTube gates the caption endpoints.
ytb looks for `yt-dlp` on your PATH by default. Point at a specific binary three
ways, in order of precedence:

1. the `--yt-dlp-bin` flag
2. the `YTB_YT_DLP_BIN` environment variable
3. the `yt_dlp_bin` key in the config file

```sh
ytb download dQw4w9WgXcQ --use-yt-dlp --yt-dlp-bin /opt/bin/yt-dlp
export YTB_YT_DLP_BIN=/opt/bin/yt-dlp
```

## Global flags worth tuning

These apply to every command. Defaults are in parentheses.

| Flag | Tunes |
| --- | --- |
| `--hl` (`en`) | InnerTube interface language, for titles and labels |
| `--gl` (`US`) | InnerTube content country, which changes what YouTube serves |
| `--rate` (`1.5s`) | Minimum delay between requests, to stay polite |
| `--retries` (`3`) | Retry attempts on HTTP 429 and 5xx, for resilience |
| `--timeout` (`30s`) | Per-request timeout |
| `-j, --workers` (`4`) | Concurrency for detail fetches and the crawler |
| `--db` | Tee every record into a generic store, separate from the crawl store |
| `-o, --output` (`auto`) | Output format |
| `-n, --limit` (`0`) | Max rows emitted, `0` is unlimited |
| `--max-pages` (`0`) | Max continuation pages fetched, `0` is unlimited |
| `--cache-ttl` (`15m`) | How long a cached response is served before it is refetched |
| `--no-cache` | Bypass the on-disk caches for this run |
| `-v, --verbose` | Print every request that went out, repeatable |

`--help` prints `0s` for `--rate` and `-1` for `--retries`, which are the
flag's own zero values rather than the effective defaults. Zero there means
"you did not ask", and ytb then uses `1.5s` and `3`. Pass `--rate 0.1s` if you
actually want it fast.

## Seeing what went out

`-v` prints one line per request to stderr, so records on stdout still pipe
cleanly:

```sh
ytb video dQw4w9WgXcQ -v --no-cache -o json | jq -r '.[0].title'
```

```
GET  200  297ms https://www.youtube.com/watch?v=dQw4w9WgXcQ
```

One line, because a whole video record costs one request.

A cache hit never appears, because the trace sits in the HTTP transport and a
cache hit never gets that far. That makes the line count an honest answer to
"how many requests did this actually cost", which is the same rule the crawl
budget counts by. It also means a second run of the command above traces
nothing at all, which is why the example passes `--no-cache`.

`-vv` adds the full query string and the headers that decide what a request
means: the `Range` on a media fetch, the InnerTube client name on a player POST.
That is the level to use when you want to replay a request with curl. `Cookie`
and `Authorization` are never printed at either level, so a trace is safe to
paste into a bug report even with a session attached.

## Precedence

Settings resolve in one direction: a command-line flag overrides the config
file, and the config file overrides the built-in default. So you can set
`rate = "3s"` in the file for everyday politeness and still pass `--rate 1s` on a
single run that you want faster.
