---
title: "Troubleshooting"
description: "Gated transcripts, Restricted Mode comments, rate limits, and exit codes."
weight: 40
---

## Transcript text comes back empty

YouTube gates the raw `timedtext` caption endpoint behind a proof-of-origin
token (a poToken). Without it, a direct text fetch returns nothing, so the
transcript prints empty even though the captions clearly exist.

The fix is yt-dlp. Install it from
[github.com/yt-dlp/yt-dlp](https://github.com/yt-dlp/yt-dlp) and put it on your
PATH. When a direct fetch comes back empty, ytb falls back to yt-dlp, parses
the VTT it returns, and gives you the timed segments anyway. Listing the tracks
never needs the token, so this always works:

```sh
ytb transcript dQw4w9WgXcQ --list   # available tracks, no yt-dlp needed
ytb transcript dQw4w9WgXcQ          # text, via yt-dlp fallback if gated
```

If yt-dlp lives somewhere off PATH, point at it with `--yt-dlp-bin` or the
`YOUTUBE_YT_DLP_BIN` environment variable.

## Comments are hidden by Restricted Mode

YouTube applies Restricted Mode to some server and datacenter IP ranges, which
hides comments no matter what cookies you send. This is a property of the network
you are calling from, not of the video.

ytb detects this case and tells you Restricted Mode is in effect rather than
silently reporting zero comments. On a normal residential connection comments
come back fine. There is nothing to configure; if you hit this on a cloud host,
run the command from a residential network instead.

## Rate limiting and HTTP 429

ytb paces its requests and retries on transient failures. It waits at least
`--rate` between requests (default `1.5s`) and retries `429` and `5xx` responses
with backoff up to `--retries` times (default `3`).

If you still see repeated `429`s, raise the delay:

```sh
ytb channel @MrBeast --videos --rate 3s
ytb search "podcast" -n 500 --rate 4s --retries 5
```

## Exit codes

ytb maps outcomes to process exit codes, so scripts can branch on them:

| Code | Meaning |
| --- | --- |
| `0` | Success |
| `2` | Usage error (bad flags or arguments) |
| `3` | No results |
| `4` | Partial result (for example comments suppressed by Restricted Mode) |
| `6` | A required external tool is missing (yt-dlp) |

```sh
ytb search "this returns nothing xyzzy" ; echo "exit $?"
```

## A handle or channel will not resolve

Handles and vanity names are resolved through YouTube and occasionally fail, for
example when a name is ambiguous or recently changed. When that happens, pass the
full URL or the canonical `UC...` channel id instead:

```sh
ytb channel UCX6OQ3DkcsbYNE6H8uQQuVA          # canonical id always resolves
ytb channel https://www.youtube.com/@MrBeast  # or the full URL
```
