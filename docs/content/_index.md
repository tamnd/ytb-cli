---
title: "ytb"
description: "A fast, friendly command line for YouTube. Resolve videos to full metadata, stream channels and playlists, search with the full filter grid, pull comments and transcripts, download video and audio with a built-in engine, browse YouTube Music, and persist it all locally, from one binary with no API key."
heroTitle: "YouTube, from the command line"
heroLead: "ytb is a single pure-Go binary that puts the whole of YouTube behind a tool that feels like curl. Resolve a video to full metadata, stream a channel's uploads, page a playlist, search with every filter the site has, pull comments and transcripts, download video and audio with a built-in engine, follow hashtags and community posts, and browse YouTube Music, with no API key and nothing to pay for."
heroPrimaryURL: "/getting-started/quick-start/"
heroPrimaryText: "Get started"
---

Working with YouTube programmatically usually means an API key, a daily quota
that runs out fast, and a Data API whose shape barely resembles what you see in
the browser. ytb talks to the same public InnerTube endpoints the site
itself uses, so there is nothing to sign up for and no quota to budget.

```bash
ytb video dQw4w9WgXcQ                 # full metadata for a video
ytb channel @MrBeast --videos          # stream a channel's uploads
ytb search "lofi hip hop" -n 50        # search with continuation paging
ytb transcript dQw4w9WgXcQ             # the video's transcript as text
ytb download dQw4w9WgXcQ               # download it with the built-in engine
```

It speaks to the public endpoints behind `youtube.com/youtubei/v1/*` over plain
HTTPS. The binary is pure Go with no runtime dependencies. Downloads use a
built-in pure-Go engine, so they need no external tool either; ffmpeg and yt-dlp
are optional and only come in for merging, audio conversion, and recovering
gated transcripts.

## What you can do with it

- **Resolve videos.** Full metadata for any video: title, channel, views, likes,
  publish date, duration, chapters, available caption tracks, streaming formats,
  and the related-video graph.
- **Walk channels and playlists.** Stream a channel's videos, shorts, live
  streams, and playlists, or page through any playlist's items, following the
  continuation tokens automatically.
- **Search like the site does.** The full filter grid: type, upload date,
  duration, sort order, and feature flags (HD, 4K, subtitles, live, 360, HDR,
  Creative Commons).
- **Read the social layer.** Comments and replies, community posts, hashtag
  feeds, and search autocomplete suggestions.
- **Download video and audio.** A built-in pure-Go engine fetches streams with
  no API key and no external downloader, with a yt-dlp-style format selector,
  playlist support, subtitles, SponsorBlock, and optional ffmpeg post-processing.
- **Browse YouTube Music.** Search artists, albums, and songs, and open an
  artist, album, playlist, or song through the Music endpoints.
- **Keep what you fetch.** Point any command at a local SQLite store and it
  becomes a crawler, with a work queue, SQL access, and a Markdown exporter.

## Where to go next

- New here? Start with the [introduction](/getting-started/introduction/) for
  the mental model, then the [quick start](/getting-started/quick-start/).
- Want to install it? See [installation](/getting-started/installation/).
- Looking for a specific task? The [guides](/guides/) cover videos, channels and
  playlists, search, comments and transcripts, downloading media, music, and the
  local store.
- Need every flag? The [CLI reference](/reference/cli/) is the full surface.
