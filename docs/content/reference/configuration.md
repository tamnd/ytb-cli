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

Point at a different file for one run with the global `--config` flag.

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
# db        = ""           # path to the optional SQLite store
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
| `--db` | Path to the optional SQLite store |
| `-o, --output` (`auto`) | Output format |
| `-n, --limit` (`0`) | Max rows emitted, `0` is unlimited |
| `--max-pages` (`0`) | Max continuation pages fetched, `0` is unlimited |

## Precedence

Settings resolve in one direction: a command-line flag overrides the config
file, and the config file overrides the built-in default. So you can set
`rate = "3s"` in the file for everyday politeness and still pass `--rate 1s` on a
single run that you want faster.
