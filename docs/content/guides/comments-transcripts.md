---
title: "Comments and transcripts"
description: "Read a video's comments and replies, and pull its captions as text or timed segments."
weight: 40
---

Two commands cover the text around a video: `comments` reads the comment thread, `transcript` reads the captions.
Both take a video id or URL, and both honor `-n`/`--limit` and `--max-pages` for paging, plus `-o` for the output format.

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

Comment bodies come from YouTube's current entity-payload model, so each row is parsed out of that representation rather than a flat list.
With `--replies`, reply rows are tagged with the `parent_id` of the comment they answer, which lets you reassemble the thread downstream.

### Restricted Mode

YouTube applies Restricted Mode to some server and datacenter IP ranges, and that mode hides comments entirely.
When the response comes back gated, `comments` reports `comments are hidden by Restricted Mode` and exits with an error.
It does not pretend the video has zero comments.
On a normal residential connection comments stream normally.
See [Troubleshooting](/reference/troubleshooting/) for the gating details.

## Transcripts

Two commands, and the split is what a video has against what one track says.
`captions <video-id|url>` lists the tracks; `transcript <video-id|url>` fetches one of them and writes it out.

```sh
ytb captions dQw4w9WgXcQ              # what tracks exist
ytb transcript dQw4w9WgXcQ            # joined text
```

| Flag | Meaning |
| --- | --- |
| `--format` | `text` (default), `srt`, `vtt`, `json` |
| `--lang` | Caption language code |
| `--auto` | Take the auto-generated track |
| `--translate` | Ask YouTube to machine-translate the chosen track into this language code |
| `--out` | Write to this file instead of stdout |

All four serializations come off one parse, so the timings in the `srt` and the `json` are the same timings.

```sh
ytb transcript dQw4w9WgXcQ --format srt --out never.srt
ytb transcript dQw4w9WgXcQ --format json | jq '.cues[0]'
ytb transcript dQw4w9WgXcQ | wc -w
```

### Picking a track

The caption list is where the choice is made, and `vss_id` is the column that matters.
`.en` is the track a person wrote and `a.en` is the one a machine wrote, which is what tells two tracks with the same language code apart.

With no `--lang` the human track wins, because a person's punctuation is worth more than a machine's word list.
`--auto` overrides that and asks for the machine track even when a human one exists; it is the only track that carries per-word timings, which `--format json` keeps.

`--lang` matches loosely in the one direction that is safe: `--lang es` finds `es-419` when there is no plain `es`.

```sh
ytb transcript dQw4w9WgXcQ --lang es
```

`--translate` asks YouTube for a machine translation of the chosen track into a language nobody has captioned it in.
The translation is the site's own, which is worth knowing before quoting it.

```sh
ytb transcript dQw4w9WgXcQ --translate vi
```

### No yt-dlp, no JavaScript

Transcripts used to need yt-dlp on your `PATH`, because YouTube gates the raw `timedtext` endpoint and a direct fetch came back empty.
That is no longer true: the track is fetched and parsed in-process, with no yt-dlp, no Deno, and no JavaScript interpreter anywhere in the process tree.

```sh
env PATH=/usr/bin:/bin ytb transcript dQw4w9WgXcQ
```

A yt-dlp binary is still used as a fallback if the endpoint refuses and one happens to be installed, and it says so when it does.
See [Troubleshooting](/reference/troubleshooting/) for the gating details.
