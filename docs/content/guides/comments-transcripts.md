---
title: "Comments and transcripts"
description: "Read a video's comments and replies, and pull its captions as text or timed segments."
weight: 40
---

Two commands cover the text around a video: `comments` reads the comment thread,
`transcript` reads the captions. Both take a video id or URL, and both honor
`-n`/`--limit` and `--max-pages` for paging, plus `-o` for the output format.

## Comments

`comments <video-id|url>` streams the top-level comments for a video.

```sh
ytb comments dQw4w9WgXcQ
```

| Flag | Meaning |
| --- | --- |
| `--replies` | Also fetch replies; each reply carries its `parent_id` |
| `--all` | Remove the default cap and page the whole thread |
| `--sort` | `top` or `new` |

Newest 50 top-level comments:

```sh
ytb comments dQw4w9WgXcQ --sort new -n 50
```

Top comments with their replies threaded in:

```sh
ytb comments dQw4w9WgXcQ --replies
```

Comment bodies come from YouTube's current entity-payload model, so each row is
parsed out of that representation rather than a flat list. With `--replies`,
reply rows are tagged with the `parent_id` of the comment they answer, which
lets you reassemble the thread downstream.

### Restricted Mode

YouTube applies Restricted Mode to some server and datacenter IP ranges, and
that mode hides comments entirely. When the response comes back gated,
`comments` reports `comments are hidden by Restricted Mode` and exits with an
error. It does not pretend the video has zero comments. On a normal residential
connection comments stream normally. See
[Troubleshooting](/reference/troubleshooting/) for the gating details.

## Transcripts

`transcript <video-id|url>` lists caption tracks or fetches the chosen track as
text.

```sh
ytb transcript dQw4w9WgXcQ            # joined text
ytb transcript dQw4w9WgXcQ --list     # available caption tracks
```

| Flag | Meaning |
| --- | --- |
| `--list` | List the available caption tracks and exit |
| `--timestamps` | Emit timed `{start, dur, text}` segments instead of joined text |
| `--lang` | Preferred caption language (default auto / English) |

Timed segments, in Spanish:

```sh
ytb transcript dQw4w9WgXcQ --timestamps --lang es
```

Count the words in a transcript:

```sh
ytb transcript dQw4w9WgXcQ | wc -w
```

### Why text fetches need yt-dlp

`--list` always works: it reads the caption track list straight out of the
player response, no extra fetch required.

Fetching the actual text is harder. YouTube gates the raw `timedtext` caption
endpoint behind a proof-of-origin (poToken), so a direct text fetch frequently
comes back empty. When that happens, `transcript` falls back to `yt-dlp` if it
is on your `PATH`: yt-dlp extracts the subtitles and `ytb` parses the VTT
back into segments.

If yt-dlp is not installed, the text path cannot recover and `transcript`
reports a clear error and exits. Install yt-dlp from
[its repository](https://github.com/yt-dlp/yt-dlp) if you want transcript text.
`--list` keeps working either way. See
[Troubleshooting](/reference/troubleshooting/) for more on the caption gating.
