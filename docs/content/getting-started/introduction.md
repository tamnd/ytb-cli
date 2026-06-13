---
title: "Introduction"
description: "What YouTube's data really looks like, and how ytb turns it into a command line."
weight: 10
---

## The problem with the official API

The obvious way to script YouTube is the Data API. In practice it means
registering a Google Cloud project, minting an API key, and then living inside a
daily quota where a single search costs 100 units out of 10,000. A modest crawl
exhausts it before lunch. Worse, the Data API exposes a curated subset of what
the site shows: no transcripts, no community posts, no Music, and view counts
that lag the page.

## What the site actually uses

Open a YouTube watch page and view source. Almost everything you see is rendered
from one large JSON blob the page ships inline (`ytInitialData`, plus
`ytInitialPlayerResponse` for the player). The same data is served to YouTube's
own JavaScript through a private JSON API called **InnerTube**, at
`https://www.youtube.com/youtubei/v1/*`. The endpoints map to what you do on the
site:

- `/player` — a video's playability, streaming formats, and caption list
- `/next` — the watch page: metadata, related videos, comments entry
- `/browse` — channels, playlists, trending, community, hashtags
- `/search` — search results and continuations
- `/navigation/resolve_url` — turn a handle or vanity URL into an id

These need no API key for public reads. ytb bootstraps a short-lived session
from a watch page (the visitor data and client version the endpoints expect),
then calls them exactly as the website does.

## Renderers and continuations

InnerTube responses are trees of typed nodes called **renderers**:
`videoRenderer`, `playlistVideoRenderer`, `commentRenderer`, and so on. A channel
page is a list of section renderers; a search result is a list of item
renderers. ytb walks these trees and pattern-matches the renderer keys to
pull out clean records, the same approach the site's own UI takes.

Long lists do not arrive all at once. Each response carries an opaque
**continuation token** that asks for the next chunk. ytb follows those tokens
for you, so `ytb channel @someone --videos` streams the entire upload history
without you ever seeing a page boundary. The `-n`/`--limit` and `--max-pages`
flags bound how far it goes.

## Downloading without an external tool

Older command-line YouTube tools lean on a separate downloader for the bytes.
ytb has a built-in one, in pure Go. It asks YouTube's `ANDROID_VR` client for the
stream list (the one anonymous client that still returns directly-fetchable URLs
with no proof-of-origin token), deciphers the URL signature and the `n`
throttling parameter with an embedded JavaScript interpreter, and pulls the media
down in parallel byte ranges. So `ytb download <id>` works on its own, with no
API key and nothing else installed.

ffmpeg only enters the picture for the three things that genuinely need it:
merging a separate high-resolution video track with its audio, transcoding audio,
and embedding cover art. When it is on your `PATH` ytb uses it; when it is not,
single-stream downloads still work and the rest report a clear error. The binary
never links ffmpeg, so it stays small and CGO-free.

## How ytb is built

Two layers. The `ytb` Go package is the library: an HTTP client with polite
rate limiting and retries, the InnerTube transport, the renderer-walking parsers,
the data models, and an optional SQLite store. The `cli` package is the command
tree on top, built on [Cobra](https://github.com/spf13/cobra) and
[fang](https://github.com/charmbracelet/fang). The library never imports the
CLI, so you can embed it directly in your own Go programs.

Everything streams. A command fetches a page, emits rows as it parses them, and
follows continuations until your limit is reached or the data runs out. Pass
`--db` and the same stream is also written to SQLite, turning any command into an
incremental crawler.

With that model in mind, [install the binary](/getting-started/installation/) and
take it for a [first run](/getting-started/quick-start/).
