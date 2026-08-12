---
title: "Downloading media"
description: "Download video and audio with the built-in pure-Go engine, select formats, and post-process with ffmpeg."
weight: 55
---

`ytb download` has a built-in download engine written in pure Go. It needs no
API key, no proof-of-origin token, no JavaScript interpreter, and no external
downloader for the common cases. It asks YouTube's ANDROID_VR client for the
stream list, which is the one anonymous client that answers with plain signed
URLs, so there is no signature to decipher and no `n` parameter to unscramble.

```sh
ytb download dQw4w9WgXcQ
```

That saves the best progressive (combined video+audio) stream to the current
directory. Media download is your responsibility: respect YouTube's Terms of
Service and copyright.

## Why every request is a byte range

The engine never issues a plain GET for a stream. An un-ranged GET to
googlevideo is throttled to about 32 KiB/s and in practice never finishes, while
the same URL fetched in ranges runs at line speed. So the download goes out in
1 MiB chunks, four at a time, and `--chunk` and `--concurrent-fragments` tune
that:

```sh
ytb download dQw4w9WgXcQ --chunk 4M --concurrent-fragments 8
```

The size is known from `contentLength` before the first byte arrives, which is
what makes the progress bar's total real rather than a guess, and what lets
`--continue` resume from however long the existing part file already is.

## One stream, or two merged

`--audio` and `--video` each write a single stream and need nothing else
installed. `--mux` fetches both and merges them, which is the case that needs
ffmpeg:

```sh
ytb download dQw4w9WgXcQ --audio      # audio only, pure Go
ytb download dQw4w9WgXcQ --video      # video only, silent, pure Go
ytb download dQw4w9WgXcQ --mux        # both, merged, needs ffmpeg
```

`--itag` takes one exact itag from `ytb formats` when you already know which
stream you want and would rather not write a selector for it.

## Selecting a format

`-f` accepts a yt-dlp-style selector. List what is available first:

```sh
ytb formats dQw4w9WgXcQ              # metadata for every itag
ytb formats dQw4w9WgXcQ --urls      # resolve the deciphered, playable URLs
```

Then pick one:

```sh
ytb download dQw4w9WgXcQ -f 22                 # explicit itag (720p progressive)
ytb download dQw4w9WgXcQ -f bv*+ba             # best video + best audio, merged
ytb download dQw4w9WgXcQ -f 'bv*[height<=720]+ba/b'   # cap height, fall back to progressive
```

The grammar supports:

- keywords: `best`/`b`, `worst`/`w`, `bestvideo`/`bv`, `bestaudio`/`ba`, `bv*`, `ba*`
- explicit itags: `137`, `22`
- one `+` to merge a video and an audio track: `bv*+ba`, `137+140`
- `/` fallback groups, tried left to right: `bv*+ba/b`
- `[key OP value]` filters on `height`, `width`, `fps`, `ext`, `vcodec`, `acodec`, `itag`, and bitrate, with `=`, `!=`, `<`, `<=`, `>`, `>=`

A `--quality 1080` shorthand expands to a sensible height-capped selector.

## ffmpeg: optional, only for merging and conversion

The `ytb` binary is pure Go and never links ffmpeg. Downloading any single
stream (progressive, or one adaptive video or audio track) works without it.
Three things do need ffmpeg, and `ytb` shells out to it when it is on your
`PATH` (or set with `--ffmpeg-bin` / `YTB_FFMPEG_BIN`):

- merging a separate high-resolution video track with its audio (`bv*+ba`),
- converting audio with `--audio-format` (mp3, m4a, opus, flac), and
- embedding cover art with `--embed-thumbnail`.

When a requested operation needs ffmpeg and none is found, `ytb` prints a clear
message and exits with code 6.

## Audio only

```sh
ytb download dQw4w9WgXcQ -x                       # best audio, original container
ytb download dQw4w9WgXcQ -x --audio-format mp3    # transcode to mp3 (needs ffmpeg)
```

## Naming, playlists, and archives

The `--output-template` flag takes the familiar yt-dlp field syntax. Fields are
sanitized per path component, so a slash in a title never escapes the target
directory.

```sh
ytb download dQw4w9WgXcQ \
  --out ~/Music \
  --output-template '%(uploader)s/%(title)s [%(id)s].%(ext)s'
```

Pass a playlist URL to download its items, and `--playlist-items` to choose a
subset (`1,3,5-7,10-`, negative indices count from the end). A
`--download-archive` file records what you have already fetched and skips it on
the next run:

```sh
ytb download 'https://www.youtube.com/playlist?list=PL...' \
  --playlist-items 1-10 \
  --download-archive ~/.ytb-archive.txt
```

## Subtitles alongside the media

```sh
ytb download dQw4w9WgXcQ --write-subs --sub-langs en --sub-format srt
```

This writes a sidecar `.en.srt` next to the saved file. The subtitle converter
is pure Go (no ffmpeg needed). You can also fetch subtitles on their own with
`ytb transcript --format srt --out captions.srt`.

## Falling back to yt-dlp

If you would rather use [yt-dlp](https://github.com/yt-dlp/yt-dlp) for a
particular download, `--use-yt-dlp` delegates to it (and to `--yt-dlp-bin` /
`YTB_YT_DLP_BIN` when it is not on `PATH`):

```sh
ytb download dQw4w9WgXcQ --use-yt-dlp -f bestvideo+bestaudio
```

## Related commands

- [`ytb sponsorblock`](/reference/cli/#sponsorblock) lists community segments (sponsors, intros, outros).
- [`ytb thumbnail`](/reference/cli/#thumbnail) lists or downloads the preview images.
- [`ytb chapters`](/reference/cli/#chapters) lists the chapter markers.
