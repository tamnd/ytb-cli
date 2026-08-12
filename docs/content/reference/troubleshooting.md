---
title: "Troubleshooting"
description: "Gated transcripts, Restricted Mode comments, rate limits, and exit codes."
weight: 40
---

## Transcript text comes back empty

This used to be the common case and no longer is.
Transcripts are fetched and parsed in-process, with no yt-dlp and no JavaScript interpreter, so the ordinary path works on a machine with nothing else installed:

```sh
ytb captions dQw4w9WgXcQ            # what tracks exist
ytb transcript dQw4w9WgXcQ          # the text of one of them
```

If a track still comes back empty, check the list first.
A video with no captions at all is a different problem from a track that refused, and `ytb captions` tells the two apart in one request.

When the endpoint does refuse and a yt-dlp binary happens to be on PATH, it is used as a fallback and says so.
If yt-dlp lives somewhere off PATH, point at it with `--yt-dlp-bin` or the `YTB_YT_DLP_BIN` environment variable.

## A download exits with code 7

Exit code 7 means the run needed an external tool that was not there.
Single-stream downloads use only the built-in engine, but three things shell out to ffmpeg: merging a separate video track with its audio (`-f bv*+ba`), audio conversion (`--audio-format`), and embedding cover art (`--embed-thumbnail`).
Install [ffmpeg](https://ffmpeg.org/), put it on your PATH (or pass `--ffmpeg-bin` / set `YTB_FFMPEG_BIN`), or pick a single progressive stream that needs no merge:

```sh
ytb download dQw4w9WgXcQ -f 22         # a progressive stream, no ffmpeg needed
ytb download dQw4w9WgXcQ --ffmpeg-bin /opt/homebrew/bin/ffmpeg
```

The same code comes back when `--use-yt-dlp` is set and no yt-dlp binary is found, and when a video is served only over SABR, which the native engine does not speak.

## Comments are hidden by Restricted Mode

YouTube applies Restricted Mode to some server and datacenter IP ranges, which hides comments no matter what cookies you send.
This is a property of the network you are calling from, not of the video.

ytb detects this case and tells you Restricted Mode is in effect rather than silently reporting zero comments.
On a normal residential connection comments come back fine.
There is nothing to configure; if you hit this on a cloud host, run the command from a residential network instead.

## Rate limiting and HTTP 429

ytb paces its requests and retries on transient failures.
It waits at least `--rate` between requests (default `1.5s`) and retries `429` and `5xx` responses with backoff up to `--retries` times (default `3`).

If you still see repeated `429`s, raise the delay:

```sh
ytb uploads @MrBeast --rate 3s
ytb search "podcast" -n 500 --rate 4s --retries 5
```

## Exit codes

ytb maps outcomes to process exit codes, so scripts can branch on them:

| Code | Meaning |
| --- | --- |
| `0` | Success |
| `1` | Something else went wrong, including a 5xx that outlasted the retries |
| `2` | Usage error: bad flags, missing arguments, or nothing named to work on |
| `3` | The query ran and matched nothing |
| `4` | Signing in would be needed to see this |
| `5` | Still rate limited after the retries |
| `6` | No such video, channel, playlist or track |
| `7` | An external tool or a capability was missing (ffmpeg, yt-dlp, a SABR-only stream) |
| `8` | The request never got an answer |

The distinction that matters most in a script is 3 against 6.
A search that matched nothing is a normal outcome and a video id that does not resolve is not:

```sh
ytb search "this returns nothing xyzzy" ; echo "exit $?"   # 3
ytb video aaaaaaaaaaa                   ; echo "exit $?"   # 6
ytb video                               ; echo "exit $?"   # 2, nothing was named
```

A 5xx that survives every retry stays at 1 on purpose.
It is not the transport failing and it is not a rate limit, and giving it 8 or 5 would be a tidier table that says something untrue.

An unknown flag or an unknown command exits 1 rather than 2, because that is decided by the argument parser before ytb sees the run.

## A handle or channel will not resolve

Handles and vanity names are resolved through YouTube and occasionally fail, for example when a name is ambiguous or recently changed.
When that happens, pass the full URL or the canonical `UC...` channel id instead:

```sh
ytb channel UCX6OQ3DkcsbYNE6H8uQQuVA          # canonical id always resolves
ytb channel https://www.youtube.com/@MrBeast  # or the full URL
```
