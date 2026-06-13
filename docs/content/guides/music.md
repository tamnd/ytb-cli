---
title: "YouTube Music"
description: "Search artists, albums, and songs, and open an artist, album, playlist, or song via the Music endpoints."
weight: 50
---

The `music` command group talks to YouTube Music. It uses the `WEB_REMIX` client
context (the same one music.youtube.com sends), so results are music entities
(artists, albums, songs, and music playlists) rather than the plain video grid
you get from `ytb search`. As with the rest of the tool, there is no API key
and no quota.

```sh
ytb music search "daft punk"
```

## Searching

`music search` queries across artists, albums, songs, and playlists at once.
Narrow it to a single kind with `--type`, which takes `song`, `album`, `artist`,
or `playlist`.

```sh
ytb music search "daft punk"                       # everything that matches
ytb music search "daft punk" --type artist          # artists only
ytb music search "random access memories" --type album
ytb music search "get lucky" --type song -n 20      # cap at 20 rows
ytb music search "deep focus" --type playlist
```

Results carry the browse id or video id you need to open the entity with the
other subcommands, so the two compose:

```sh
ytb music search "daft punk" --type artist -o id    # just the browse ids
```

## Opening an artist

`music artist` takes an artist browse id (or a music.youtube.com URL) and returns
the artist profile, including their albums and top songs.

```sh
ytb music artist UC8j_C5jZheV_2g_AymBhI-A
ytb music artist "https://music.youtube.com/channel/UC8j_C5jZheV_2g_AymBhI-A"
```

## Opening an album

`music album` takes an album browse id or URL and returns the album header plus
its track list.

```sh
ytb music album MPREb_abc123def
ytb music album MPREb_abc123def -o json
```

## Opening a music playlist

`music playlist` takes a playlist id or URL and returns the playlist with its
tracks. These are YouTube Music playlists, so the track rows carry music
metadata rather than plain video fields.

```sh
ytb music playlist RDCLAK5uy_l7
ytb music playlist RDCLAK5uy_l7 -n 100
```

## Song detail and lyrics

`music song` takes a video id and returns the song detail. Pass `--lyrics` to
also fetch the lyrics when they are available (not every track has them).

```sh
ytb music song 5NV6Rdv1a3I
ytb music song 5NV6Rdv1a3I --lyrics
```

## Output and persistence

Every `music` subcommand renders through the same formatter as the rest of the
tool, so `-o`, `--fields`, and `--template` all work:

```sh
ytb music search "lo-fi" --type song --fields title,artist,duration
ytb music artist UCabc123 -o jsonl
```

Pass the global `--db` flag and ytb also persists what it fetches into the
local SQLite store, just as it does for the video and channel commands:

```sh
ytb music album MPREb_abc123def --db yt.db
```

See [The local store](../the-store/) for what gets persisted and how to query it.
