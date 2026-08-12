---
title: "YouTube Music"
description: "Search and open artists, albums, tracks and playlists through the WEB_REMIX client, with no key and no quota."
weight: 50
---

The `music` command group talks to music.youtube.com, which is a second app over the same catalogue on its own InnerTube key as `WEB_REMIX`.
It answers differently from the main site: it counts plays where www counts views, it credits every artist on a release rather than the uploader, and it knows about albums, which www does not.
As with the rest of the tool there is no API key and no quota.

```sh
ytb music search "daft punk"
```

## A track id is a video id

One song can have two ids, an art track and an official video, and the two carry different numbers.
The `music_video_type` field says which is which: `ATV` is an art track, and `OMV`, `UGC` or `OFFICIAL_SOURCE_MUSIC` is a video of the song.
Nothing decides this from the word next to the row, because that word is whatever language you asked for.
The `kind` column in a table shows `track` for the first and `video` for the second, and both are the same record with the same fields.

Rick Astley is the clearest example: the album lists `dQw4w9WgXcQ`, the official video, and the album's playlist lists `lYBUbBu4W08`, the art track of the same song.

## Searching

`music search` queries everything at once and classifies each row by its own endpoint, so tracks, albums, artists and playlists come back interleaved the way the site returns them.
Narrow it with `--type`, which takes `song`, `video`, `album`, `artist`, `playlist`, `podcast` or `episode`.

```sh
ytb music search "daft punk"                              # everything that matches
ytb music search "daft punk" --type artist
ytb music search "random access memories" --type album
ytb music search "get lucky" --type song -n 20
ytb music search "deep focus" --type playlist
```

The top result, the card the site puts above the list, is a record too rather than a heading.
Searching for an artist returns that artist as the first row with their channel id in hand.

Rows carry the browse id or video id the other subcommands take, so the two compose:

```sh
ytb music search "daft punk" --type artist -o url
```

### Counts that are not labelled

A search row renders a count with a unit word and nothing typed to say which count it is.
One artist row says `38.2M monthly audience` and the next says `1.29K subscribers`; one playlist says `116 songs` and the next says `121 views`.
Only the noun separates them, so `ytb` keeps the rendered line in `metadata_parts` and claims neither field.
The artist page and the playlist page state them properly, and those reads fill them in.

## Opening an artist

`music artist` takes an artist browse id, a channel id or a music.youtube.com URL, and returns the artist with their discography, top songs, videos, playlists and related artists.

```sh
ytb music artist UCwZEU0wAwIyZb4x5G_KJp2w
ytb music artist "https://music.youtube.com/channel/UCwZEU0wAwIyZb4x5G_KJp2w"
```

Albums and singles are split the way the page splits them: a singles lockup carries its own type where an album lockup carries only a year.
Every video keeps the shelf it came off, so a live performance stays distinguishable from a music video without reading either heading.

A user channel resolves to the artist behind it, so `UCuAXFkgsw1L7xaCfnd5JJOw` lands on `UCwZEU0wAwIyZb4x5G_KJp2w`.

## Opening an album

`music album` takes an album browse id or URL and returns the album header and its tracks.

```sh
ytb music album MPREb_dcYZhAh5urI
ytb music album MPREb_dcYZhAh5urI -o json
```

The album also carries `playlist_id`, the `OLAK5uy_` playlist that holds the same songs and is what www knows the release by.
Browsing that playlist gives the art tracks where the album page gives the official videos.

## Opening a music playlist

`music playlist` takes a playlist id or URL and returns the playlist with its tracks, following every continuation.

```sh
ytb music playlist RDCLAK5uy_lMzHW51iFg1Kx0d_2EHpzbOgCrwtu8cgI
ytb music playlist OLAK5uy_nmDUsWOMoEcz0SsVqUwir0oxu-k1oUyXE -n 100
```

An album's `OLAK` playlist browses with an empty header, which is the site's own answer and not a failed read.
The record says so in `missed` and points at the album page for the title and the credits.

## Track detail and lyrics

`music track` takes a video id and returns the track.
Pass `--lyrics` to fetch the lyrics tab as well.

```sh
ytb music track lYBUbBu4W08
ytb music track lYBUbBu4W08 --lyrics
```

Lyrics arrive as lines and are kept as lines, because a verse is not a paragraph.
The credit comes with them in `lyrics_source`, usually `Source: Musixmatch`.

Not every id has them.
The official video of a song often does not even when its art track does, and the tab answers in words when it refuses.
Whatever it said is recorded in `missed` rather than reported as nothing:

```sh
ytb music track dQw4w9WgXcQ --lyrics -o json
# missed: ["the lyrics tab says: Lyrics not available"]
```

## Music as an edge

`ytb edges <id> --music` reads a video through YouTube Music and writes what music states about it as claims: who the artist is and which album it is on.

```sh
ytb edges dQw4w9WgXcQ --music
```

A crawl can do the same for every video it walks, which is where the album, the artist and the release year come from:

```sh
ytb crawl @RickAstleyYT --music --depth 1
```

## Output and persistence

Every `music` subcommand renders through the same formatter as the rest of the tool, so `-o`, `--fields` and `--template` all work.
Each command emits one row shape for the whole stream, so a table stays lined up even when a search returns four kinds at once.

```sh
ytb music search "lo-fi" --type song --fields title,artist,detail
ytb music artist UCwZEU0wAwIyZb4x5G_KJp2w -o jsonl
```

See [The local store](../the-store/) for what gets written and how to query it.
