---
title: "CLI"
description: "Every command and subcommand, with the flags that matter."
weight: 10
---

```
ytb <command> [subcommand] [flags]
```

Run `ytb <command> --help` for the full flag list on any command. This page is the map.

## Global flags

Persistent flags accepted by every command.

| Flag | Meaning |
| --- | --- |
| `-o, --output` | Output format: `list`, `table`, `markdown`, `json`, `jsonl`, `csv`, `tsv`, `url`, `raw` (auto) |
| `--fields` | Comma-separated columns to show |
| `-n, --limit` | Max rows emitted (`0` = unlimited) |
| `--max-pages` | Max continuation pages fetched (`0` = unlimited) |
| `-j, --workers` | Concurrency for detail fetches (4) |
| `--rate` | Minimum delay between requests (1.5s) |
| `--retries` | Retry attempts on 429/5xx (3) |
| `--timeout` | Per-request timeout (30s) |
| `--hl` | InnerTube interface language (en) |
| `--gl` | InnerTube content country (US) |
| `--db` | Tee every record into a generic store (e.g. `out.db`, `postgres://...`) |
| `-q, --quiet` | Suppress progress output |
| `-v, --verbose` | Print each request that went out; `-vv` adds the query and the headers |
| `--color` | Color output: `auto`, `always`, `never` (auto) |
| `--template` | Go text/template applied per row |
| `--no-header` | Omit the header row in table/csv/tsv |
| `--cache-ttl` | How long a cached response is served before it is refetched (15m) |
| `--no-cache` | Bypass the on-disk caches |
| `--data-dir` | Override the data directory (the store, the cache, the cookies) |
| `-y, --yes` | Assume yes to prompts |
| `--dry-run` | Print actions without performing them |
| `--yt-dlp-bin` | Path to the yt-dlp binary (`download --use-yt-dlp`) |
| `--ffmpeg-bin` | Path to ffmpeg (used to merge/convert when present) |

`-n` means the same thing everywhere: stop after N records.
A read that pages is told the number up front and stops asking for continuations; a read that answers in one request cuts its list on the way out.
So `ytb captions <id> -n 2` prints two tracks and `ytb search <q> -n 2` makes one request rather than paging to the default limit.

There is no `--config` flag.
The config file has one path, `ytb config path` prints it, and `--data-dir` moves the data rather than the config.
`--profile` is accepted by the framework and nothing in ytb reads it yet.

`-v` writes to stderr, one line per request, so records on stdout still pipe.
A cache hit never prints, because the trace sits in the HTTP transport and a cache hit never reaches it, which makes the line count an honest answer to what a command cost.
`-vv` adds the full query and the headers that decide what a request means, which is the `Range` on a media fetch and the client name on a player POST.
`Cookie` and `Authorization` are never printed at either level.

## Commands

| Command | What it does |
| --- | --- |
| `video` | Resolve one or more videos to full metadata |
| `channel` | A channel's record: header, about panel and external links |
| `about` | A channel's about panel on its own |
| `uploads` | Stream a channel's uploads |
| `feed` | A channel's Atom feed: the newest fifteen, with exact times |
| `playlists` | List a channel's playlists |
| `playlist` | Playlist header |
| `items` | Stream a playlist's videos |
| `search` | Search with the full filter grid |
| `trending` | What's hot right now |
| `comments` | Comments and replies |
| `community` | Community / posts tab |
| `hashtag` | A hashtag feed |
| `related` | The related-videos graph |
| `discover` | Breadth-first walk of the graph linked from a video, channel, or playlist |
| `id` | Classify any id, handle or URL, and derive what it implies |
| `edges` | The claims one read makes: subject, predicate, object, and who said so |
| `predicates` | The closed vocabulary: every predicate with its domain and its range |
| `rdf` | The same claims as n-triples, turtle or json-ld, with provenance |
| `graph` | Walk the frontier the claims name, on a budget counted in requests |
| `suggest` | Search autocomplete suggestions |
| `transcript` | Captions as text |
| `captions` | A video's caption tracks, one record each |
| `formats` | Streaming formats (metadata only) |
| `music` | YouTube Music (artists, albums, songs) |
| `download` | Download media with the native engine (or yt-dlp) |
| `extract` | Extract a specific stream via yt-dlp |
| `sponsorblock` | List community SponsorBlock segments |
| `thumbnail` | List a video's thumbnails, or fetch the best one |
| `chapters` | List a video's chapter markers |
| `crawl` | Walk the graph from seeds into the store |
| `archive` | Write one read down in full: page, payloads, headers, and what ytb parsed |
| `query` | Run SQL over the store, read-only |
| `export` | Render the store as interlinked Markdown |
| `db` | The local SQLite store |
| `cache` | The on-disk response cache: where it is, what is in it, and how to empty it |
| `auth` | Manage the optional YouTube session |
| `config` | View and manage configuration |
| `version` | Print version information |
| `serve` | Serve the operations over HTTP as NDJSON, covered in [serve and MCP](/reference/serve-and-mcp/) |
| `mcp` | Run as an MCP server over stdio, covered in [serve and MCP](/reference/serve-and-mcp/) |

## video

`ytb video <id|url>... [--flags]` resolves a video to full metadata (HTML bootstrap plus `/player` plus `/next`). Pass `-` to read ids/urls from stdin.

| Flag | Meaning |
| --- | --- |
| `--captions` | Also list the caption tracks that fetch, one extra request |
| `--formats` | Also read the stream list, one extra request |
| `--transcript` | Fetch and attach the transcript text |
| `--lang` | Preferred caption language for `--transcript` |
| `--thumbnails` | Confirm each constructed thumbnail rendition with a HEAD |
| `--microdata` | Include what the page's own schema.org markup says |
| `--no-player` | Never call the mobile player, whatever else was asked |

Chapters, related videos and the description's links come back on a plain read and need no flag; `ytb chapters` and `ytb related` are the same data on its own.

`formats` is off by default and stays off, because it costs a mobile player request per video and a crawl of a thousand videos should not make a thousand extra calls nobody asked for.
The watch page carries a stream list for free and a plain read still leaves it off: 29 formats nobody asked for bury the dozen fields the caller wanted.

## channel

`ytb channel <id|@handle|url> [--flags]`. Prints the channel record.

| Flag | Meaning |
| --- | --- |
| `--no-about` | Skip the about panel: one request fewer, and no join date, lifetime views or link titles |
| `--counts` | Read the four derived playlists and check that videos + shorts + streams = uploads (four extra requests) |

A channel page publishes its facts in four blocks that barely overlap, and the record is all four merged.
`channelMetadataRenderer` has the id, the keywords, the RSS url and the 249 country codes it will serve in.
`microformatDataRenderer` has the crawler's view plus a schema.org ProfilePage, and `mainEntity.sameAs` in that ProfilePage is the whole external link list with no continuation behind it, which is the cheapest place on the site to get a channel's links.
`pageHeaderViewModel` has the handle, the badge and the banner.
The about panel is a continuation and is the only source of the join date, the lifetime view count, the country in words and the link titles, which is what `--no-about` trades away.

Subscriber counts always carry `subscriber_count_is_approximate: true`.
The site rounds every one of them to three significant figures, including the number in the ProfilePage's `interactionStatistic`, so 1490000 means somewhere in the 1.49 millions and never means exactly that.

The header and the about panel can disagree with each other about the video count, and both can disagree with the uploads playlist.
The panel beats the header, `--counts` beats both, and `via` names the number that lost:

```
$ ytb channel @RickAstleyYT --counts --fields handle,videos,counts
╭───────────────┬────────┬─────────────────────────────────────╮
│ HANDLE        │ VIDEOS │ COUNTS                              │
├───────────────┼────────┼─────────────────────────────────────┤
│ @RickAstleyYT │ 435    │ 139 + 294 + 2 = 435 uploads, agrees │
╰───────────────┴────────┴─────────────────────────────────────╯
```

Those are the four playlists derived from the channel id: `UU` is every upload, `UULF` long form, `UUSH` shorts, `UULV` past live streams.
The three partition the first, so a mismatch means the site is mid-write or a video is in a state none of the three tabs claims.
The table shows the whole check as one `counts` cell; `-o json` carries the four ids, the four numbers and `agrees` separately.

## about

`ytb about <id|@handle|url>`. The about panel on its own: description, join date, lifetime views, country, and the external links with their titles.
No flags beyond the globals.

## uploads

`ytb uploads <id|@handle|url> [--flags]`. Streams a channel's uploads.

| Flag | Meaning |
| --- | --- |
| `--kind` | Which uploads: `all`, `videos`, `shorts`, `streams`, `popular` (default `all`) |
| `--via` | Read them from the derived `playlist` or from the channel `tab` (default `playlist`) |
| `--exact` | Cross-read the Atom feed so the newest fifteen get exact timestamps (one extra request) |
| `--enrich` | Call `/player` per video for full metadata |
| `--max-pages` | Max continuation pages (0 = unlimited) |

The playlist route is the default because it is the only one that answers `--kind all`.
No tab lists a channel's uploads: the Videos tab is long form only and returns exactly what `UULF` returns, so `--kind all --via tab` and `--kind popular --via tab` are usage errors rather than something that quietly returns less.

A shorts row comes back from the two routes with the same id and the same view count and a different `via`, because the two pages render the same view model differently.
The Shorts tab writes a readable `shorts-shelf-item-<id>` and a "22K views" overlay; the `UUSH` playlist writes an opaque hash and no overlay at all, and the only view count on the row is the one spelled out in its accessibility label, "22 thousand views".
Both are read, and `via` says which.

`--exact` fetches the Atom feed once and merges it into the newest fifteen rows.
Those get a timestamp to the second and an exact view count; everything after row fifteen keeps the rounded text and says so in `missed`, because the feed serves fifteen entries and has no continuation.

## feed

`ytb feed <id|@handle|url>`. The channel's Atom feed: fifteen entries, newest first, each with a publication time to the second and an exact view count.
No flags beyond the globals.

This is the only surface outside the player that says whether a video is a short, which it does by linking to `/shorts/<id>` instead of `/watch?v=`.
`media:starRating count` on an entry is a rating count and not a like count, so it is never written into `like_count`.

## playlists

`ytb playlists <id|@handle|url> [--flags]`. Lists the playlists a channel has published, from its Playlists tab.

## playlist

`ytb playlist <id|url>`. Prints the playlist header.

## items

`ytb items <playlist-id|url> [--flags]`. Streams a playlist's videos, each with its position.

## search

`ytb search <query> [--flags]`. Search with the full filter grid.

| Flag | Meaning |
| --- | --- |
| `--type` | `Video`, `Channel`, `Playlist`, `Movie` |
| `--duration` | `Short`, `Medium`, `Long` |
| `--upload-date` | `Hour`, `Today`, `Week`, `Month`, `Year` |
| `--sort` | `Relevance`, `Date`, `Views`, `Rating` |
| `--cc` | Closed captions / subtitles |
| `--creative-commons` | Creative Commons license |
| `--purchased` | Purchased titles only |
| `--4k` | 4K only |
| `--hd` | HD only |
| `--hdr` | HDR only |
| `--live` | Live only |
| `--360` | 360-degree video |
| `--vr180` | VR180 only |

## trending

`ytb trending [--flags]`. Trending videos.

| Flag | Meaning |
| --- | --- |
| `--category` | `Music`, `Gaming`, `Movies`, `News` |

## comments

`ytb comments <video-id|url> [--flags]`. Comments and replies.

| Flag | Meaning |
| --- | --- |
| `--sort` | `Top`, `New` |
| `--replies` | Also fetch replies (`parent_id` set) |

Use `-n` for how many, and `-n 0` for no cap at all.

## community

`ytb community <channel-id|@handle> [--flags]`. Community / posts tab. No notable flags beyond the globals.

## hashtag

`ytb hashtag <tag> [--flags]`. Streams a hashtag's videos.

| Flag | Meaning |
| --- | --- |
| `--info` | The hashtag's own record instead of its videos |

## related

`ytb related <video-id|url> [--flags]`. The related-videos graph. No notable flags beyond the globals.

## discover

`ytb discover <seed>... [--flags]` (alias `walk`). Walk the graph of linked objects breadth-first from one or more seeds (a video, channel, or playlist reference), streaming one row per node reached. See [graph discovery](/guides/graph-discovery/).

`discover` streams records, one per node it reached. `graph`, below, streams claims, and the two answer different questions: what is out there, and who said so.

| Flag | Meaning |
| --- | --- |
| `--follow` | Edges to follow: a preset (`content`, `feed`, `comments`, `all`) or a comma-separated edge list (`channel`, `related`, `comments`, `uploads`, `playlists`, `community`, `items`, `owner`, `commenter`). Default `content` |
| `--depth` | Hops to follow from each seed (default `1`; `0` = seeds only) |
| `--fanout` | Max neighbors to follow per edge (default `25`; `0` = unlimited) |
| `--store` | Write every node reached into the local store: a record for what it fetched, a sighting for what it only saw |

The comment edges (`comments`, `commenter`) are served only when YouTube is not applying its per-IP Restricted Mode to this network; when it is, they are noted on stderr and skipped and the rest of the walk continues. `-n/--limit` is the total node budget (default `500`).

## id

`ytb id <ref>`. Classify any id, handle or URL, say what it names, and derive whatever follows from it without a request. No flags beyond the globals.

Four of YouTube's id shapes decode locally and the command says which one it got, whether reaching it needs a request, and why.

```
$ ytb id UUuAXFkgsw1L7xaCfnd5JJOw -o json
{
  "id": "UUuAXFkgsw1L7xaCfnd5JJOw",
  "kind": "uploads_playlist",
  "url": "https://www.youtube.com/playlist?list=UUuAXFkgsw1L7xaCfnd5JJOw",
  "channel_id": "UCuAXFkgsw1L7xaCfnd5JJOw",
  "needs_request": false
}
```

A channel id is five playlists: swap the `UC` prefix for `UU`, `UULF`, `UUSH`, `UULV` or `UULP` and you have every upload, the long-form videos, the shorts, the past streams and the popular list, none of which costs a request to construct.
The reverse works too, which is why a `UU` id found in a payload names a channel nobody fetched.

A handle reports `needs_request: true` and says why: a handle is an alias a channel can change, so it is a property of a channel and never the key.
A video id reports `needs_request: false` and stops, because eleven characters of base64url decode to nothing else and a tool that guessed the channel from them would be making it up.

## edges

`ytb edges <ref>... [--flags]`. Read anything YouTube has an id for and print what that read claims. See [claims and RDF](/guides/claims/).

A claim is an edge with its provenance attached: which URL asserted it, which surface answered, which client ytb said it was, and at which tier.
The source is part of the claim's identity, so two surfaces asserting the same edge stay two rows and a disagreement between them is something you can query rather than something the last read overwrote.

One request per reference. Each flag below is another read and says what it costs.
A handle costs one more, to resolve it, because a handle is not an identity and can never be a node.

| Flag | Meaning |
| --- | --- |
| `--captions` | Read the ANDROID player for the caption list (1 request) |
| `--comments` | Read up to n comments (1 request per page of about 20) |
| `--items` | Read up to n playlist items (1 request per page) |
| `--featured` | Read a channel's home tab for its featured channels (1 request) |
| `--posts` | Read up to n community posts (the tab refuses tier 0 today) |
| `--music` | Read the same id through YouTube Music (1 request) |

## predicates

`ytb predicates`. The twenty one predicates the graph plane may write, with the domain and range of each, the RDF term it maps to, and where on the site it comes from. This command makes no request. No flags beyond the globals.

The table is closed. A predicate not in it cannot be written, which is what stops a typo becoming a claim that looks fine, is never queried because nobody knows to ask for it, and is found a year later by somebody counting.

Where the arrow turns round on the way to RDF the `rdf` column says `(inverse)`: ytb writes `channel published video` because that is the direction a page reads in, and `schema:author` runs from the work to its author.

## rdf

`ytb rdf <ref>... [--flags]`. The same claims as n-triples, turtle or json-ld, with provenance. See [claims and RDF](/guides/claims/).

The vocabulary was not invented here.
YouTube publishes schema.org about its own pages, so a watch page is a `schema:VideoObject`, its author a `schema:Person` and both counts `schema:InteractionCounter` blocks, and the mapping is read off the site.
Where the site says nothing, the terms are the ones x-cli and facebook-cli already export into, so a store from all three tools joins, and a predicate with no schema.org equivalent goes in the `yt:` namespace declared in the output rather than assumed.

Output is byte stable between two runs over the same input in all three formats, because a dump that reorders itself cannot be diffed and a diff is how somebody notices that YouTube started saying something different.

| Flag | Meaning |
| --- | --- |
| `--format` | `nt` (default, streams), `turtle`, `jsonld` |
| `--check` | Compare our triples with the page's own microdata, predicate by predicate |
| `--types-only` | Write nothing but the `rdf:type` of each node |
| `--no-provenance` | Drop the source and client annotations |

`ytb rdf` takes the same six extra-read flags as `ytb edges`.

## graph

`ytb graph <seed>... [--flags]`. Collect claims from the seeds, then from the nodes those claims named, and so on, and print how many claims of each predicate came back.

`--depth 0` is the seeds and nothing else, which is exactly what `ytb edges` does.

The budget is in requests and it is counted, not estimated.
Every request that goes out passes a hook the walk counts on, and the walk stops when the count reaches the budget, mid-level if that is where it lands.
A cache hit never reaches the hook, so it is not a request and does not count, which is why a second walk over the same seeds gets further on the same budget.

The frontier is every node the claims so far have named and nothing has read.
Only nodes ytb can read are followed, so an external URL and a hashtag are named and never fetched.

| Flag | Meaning |
| --- | --- |
| `--depth` | Hops to follow from each seed (default 1; 0 = the seeds only) |
| `--budget` | How many requests the walk may spend (default 25) |
| `--edges` | Print the claims themselves rather than the per-predicate counts |

`ytb graph` takes the same six extra-read flags as `ytb edges`.

## suggest

`ytb suggest <query> [--flags]`. Search autocomplete suggestions. No notable flags beyond the globals.

## transcript

`ytb transcript <video-id|url> [--flags]`. Fetches one caption track and writes it out. Use `ytb captions` to see what a video has.

| Flag | Meaning |
| --- | --- |
| `--format` | `text` (default), `srt`, `vtt`, `json` |
| `--lang` | Caption language code |
| `--auto` | Take the auto-generated track |
| `--translate` | Ask YouTube to machine-translate the chosen track into this language code |
| `--out` | Write to this file instead of stdout |

All four serializations come off one parse, so the timings in the `srt` and the `json` are the same timings.

`--lang en` matches `en-GB` when there is no plain `en`.
With no `--lang` the human track wins over the auto-generated one, because a person's punctuation is worth more than a machine's word list.
`--auto` asks for the auto track even when a human one exists, and it is the only one that carries per-word timings, which `json` keeps.

No yt-dlp, no Deno and no JavaScript interpreter is involved: the track is fetched and parsed in-process. When the caption endpoint refuses and a yt-dlp binary is on PATH, it is used as a fallback and says so.

## captions

`ytb captions <video-id|url>`. The caption tracks a video has, one record each: language code, `vss_id`, name, whether it was machine generated and whether it can be auto-translated. No flags beyond the globals.

The tracks come off the ANDROID player. The watch page lists the same tracks with URLs that answer HTTP 200 and an empty body, so they are not listed here.

`vss_id` is the track's own name for itself, and it is the column that matters: `.en` is the human English track and `a.en` is the machine one, which is what tells two tracks with the same language code apart.

```
$ ytb captions dQw4w9WgXcQ
╭───────────────┬─────────┬──────────────────────────┬───────┬──────────────╮
│ LANGUAGE_CODE │ VSS_ID  │ NAME                     │ AUTO  │ TRANSLATABLE │
├───────────────┼─────────┼──────────────────────────┼───────┼──────────────┤
│ en            │ .en     │ English                  │ false │ true         │
│ en            │ a.en    │ English (auto-generated) │ true  │ true         │
│ de-DE         │ .de-DE  │ German (Germany)         │ false │ true         │
│ ja            │ .ja     │ Japanese                 │ false │ true         │
│ pt-BR         │ .pt-BR  │ Portuguese (Brazil)      │ false │ true         │
│ es-419        │ .es-419 │ Spanish (Latin America)  │ false │ true         │
╰───────────────┴─────────┴──────────────────────────┴───────┴──────────────╯
```

`ytb transcript` fetches one of these tracks; this command is the one that says which tracks exist.

## formats

`ytb formats <video-id|url> [--flags]`. Lists formats from `/player` streamingData, deduped by itag, audio first then video then muxed. Metadata only by default; pass `--urls` to resolve the deciphered, directly-fetchable stream URLs through the native engine.

The list is read from the ANDROID player, which is the only one that answers with plain URLs.
On the watch page every adaptive format arrives with a `contentLength` and neither a `url` nor a `signatureCipher`, so there is nothing to fetch, and the read says so when it has to fall back to it.

Under the table is a note naming the client that answered and when its URLs expire, and saying that fetching any of them without a `Range` header runs at 32 KiB/s where the same URL fetched in ranges runs at 4 MiB/s.
It goes to stderr, so `-o json` stays a clean stream.
A muxed format comes back from the mobile player with no `contentLength`, so its size column is empty and its note says `size unknown` rather than showing a 0 that reads as an empty file.

The table shows eight columns and `-o json` carries the whole record: `kind`, `container` and `codec` parsed off the mime type, `average_bitrate`, `audio_channels`, `audio_sample_rate`, `approx_duration_ms`, `init_range` and `index_range` for a DASH reader, `last_modified` for when the rendition was encoded, the `url` when a mobile player answered, `expires_at` off the URL's own `expire`, and `is_throttled_unranged`, which is true on every format because it is a fact about googlevideo rather than about the format.
A field that does not apply is absent rather than zero, so an audio format has no `width` and no `fps` instead of having them set to 0.

| Flag | Meaning |
| --- | --- |
| `--audio` | Audio-only adaptive formats |
| `--video` | Video-only adaptive formats |
| `--muxed` | Progressive (muxed) formats only |
| `--urls` | Resolve playable stream URLs (deciphered) via the native engine |

## music

`ytb music [command] [--flags]`. Search and browse YouTube Music via the WEB_REMIX client context.

| Subcommand | What it does |
| --- | --- |
| `search <query>` | Search everything, or one kind with `--type` |
| `artist <browseId\|url>` | Artist profile with discography, top songs, videos and related artists |
| `album <browseId\|url>` | Album header and track list |
| `playlist <id\|url>` | Music playlist and tracks |
| `track <video-id>` | Track detail (with `--lyrics` if available) |

Notable subcommand flags:

| Flag | Subcommand | Meaning |
| --- | --- | --- |
| `--type` | `music search` | `song`, `video`, `album`, `artist`, `playlist`, `podcast`, `episode` |
| `--lyrics` | `music track` | Fetch the lyrics tab if it has any |

Every row is classified by the type on its own endpoint and never by the word the page rendered next to it, so a search reads the same in any `--hl`. `music_video_type` is `ATV` for an art track and `OMV`, `UGC` or `OFFICIAL_SOURCE_MUSIC` for a video of the same song, which is what keeps the two ids of one song apart. A count with no type on it, like `116 songs` against `121 views`, is kept verbatim in `metadata_parts` rather than filed as an item count.

## download

`ytb download <id|url>... [--flags]`. Downloads media with the built-in pure-Go engine.

The native engine fetches streams through the ANDROID_VR client (no API key, no token), which answers with plain signed URLs, so there is nothing to decipher and no JavaScript to run.

Every request goes out as a byte range, in 1 MiB chunks by default. That is not a tuning choice: an un-ranged GET to googlevideo is throttled to about 32 KiB/s and never finishes, while the same URL fetched in ranges runs at line speed. Because `contentLength` is known before the first byte, the progress total is real and `--continue` resumes from the size of the part file.

`--audio` and `--video` each write one stream and need nothing else installed. `--mux` fetches both and merges them, which needs ffmpeg, as do `--audio-format` and `--embed-thumbnail`. When a requested operation needs ffmpeg and none is found on PATH (or at `--ffmpeg-bin` / `YTB_FFMPEG_BIN`), the command exits with code 6. Pass `--use-yt-dlp` to delegate to a yt-dlp binary instead.

The `--format` selector accepts a yt-dlp-style grammar: keywords (`best`, `worst`, `bestvideo`/`bv`, `bestaudio`/`ba`, `bv*`), explicit itags (`22`), a single `+` to merge a video and audio track (`bv*+ba`, `137+140`), `/` fallback groups (`bv*+ba/b`), and `[key OP value]` filters on `height`, `width`, `fps`, `ext`, `vcodec`, `acodec`, `itag`, and bitrate (`bv*[height<=720]+ba`).

| Flag | Meaning |
| --- | --- |
| `-x, --audio` | Download the audio stream only, no ffmpeg needed |
| `--video` | Download the video stream only, no ffmpeg needed |
| `--mux` | Download video and audio and merge them, needs ffmpeg |
| `--audio-format` | Convert audio to this codec (`mp3`, `m4a`, `opus`, `flac`); needs ffmpeg |
| `-f, --format` | Format selector (e.g. `best`, `22`, `bv*+ba`, `bv[height<=720]+ba`) |
| `--itag` | Download this exact itag, as listed by `ytb formats` |
| `--quality` | Max video height (e.g. `best`, `1080`, `1080p`) |
| `--out` | Output directory (.) |
| `--output-template` | yt-dlp-style filename template (default `%(title)s [%(id)s].%(ext)s`) |
| `--chunk` | Byte range requested per GET (e.g. `512K`, `1M`, `4M`; default `1M`) |
| `--concurrent-fragments` | Parallel byte-range workers (4) |
| `--continue` | Resume into an existing part file instead of starting over |
| `--embed-thumbnail` | Embed the thumbnail as cover art (mp4/m4a); needs ffmpeg |
| `--playlist-items` | Playlist item selection (e.g. `1,3,5-7,10-`) |
| `--sub-langs` | Subtitle language to write (e.g. `en`) |
| `--sub-format` | Subtitle format to write (`srt`, `vtt`, `txt`) |
| `--write-subs` | Write the subtitle sidecar file |
| `--download-archive` | Record downloaded ids here and skip ones already present |
| `--use-yt-dlp` | Delegate to a yt-dlp binary instead of the native engine |

## extract

`ytb extract <audio|video|transcript|all> <id|url> [--flags]`. Extract a specific stream via yt-dlp.

| Flag | Meaning |
| --- | --- |
| `--format` | Audio format for `extract audio` (e.g. mp3) |
| `--quality` | Max video height for `extract video` (e.g. 1080) |
| `--out` | Output directory (.) |

## sponsorblock

`ytb sponsorblock <id|url> [--flags]`. Lists community-submitted segments from the public SponsorBlock API (sponsor, intros, outros, self-promo, and more). This is an independent community service; no key is required.

| Flag | Meaning |
| --- | --- |
| `--categories` | Segment categories to fetch (default all): `sponsor`, `selfpromo`, `intro`, `outro`, ... |

## thumbnail

`ytb thumbnail <id|url> [--flags]`. Lists the standard thumbnail renditions, or writes the best available one to disk.

Each of the five standard names can be constructed for any video id and only some of them exist, so the list is HEADed before it is printed and a rendition that answers 404 is left out.
A video with no `maxresdefault` answers that URL with a 1097 byte body that is still `Content-Type: image/jpeg`, so nothing but the status code separates a rendition from a placeholder.

| Flag | Meaning |
| --- | --- |
| `--fetch` | Write the best available rendition to disk |
| `--unconfirmed` | List the constructed URLs without HEADing them |
| `--out` | Output path or directory for `--fetch` |

## chapters

`ytb chapters <id|url>`. Lists a video's chapters: position, start time, title and origin. No notable flags beyond the globals.

`origin` is a column because a macro marker and a timestamped description are two different things that produce the same list.
`markers` means YouTube served a `macroMarkersListRenderer`, so the site itself treats these as chapters, draws them on the scrubber and has a preview frame for each.
`description` means somebody typed `1:23 Verse 2` and the list is only as good as their typing.
Both line layouts are read: the timestamp can come first, as in `0:00 Introduction`, or last, as in `Eyes To The Sky - 0:00:00`.

## crawl

`ytb crawl <seed>... [--flags]`. Read each seed, write its claims into the store, then read what those claims named, hop by hop, until the depth or the budget runs out.

The budget is counted in requests rather than estimated, and a cache hit makes no request so it costs nothing.
The frontier is every node the store has heard of and not read, which is a query and not a queue, so `--resume` with no seeds picks up where an earlier run stopped.
Mixes, a channel's popular playlist, comments and formats are off the frontier by default, though none of them is refused as a seed.

| Flag | Meaning |
| --- | --- |
| `--depth` | Hops to follow from each seed (default 1; 0 = seeds only) |
| `--budget` | Request budget for the whole crawl (default 200; 0 = no limit) |
| `--items` | Playlist items to read per playlist (default 100; 0 = header only) |
| `--comments` | Comments to read per video (default 0) |
| `--posts` | Community posts to read per channel (default 0) |
| `--captions` | Read each video's caption list (one ANDROID player call per video) |
| `--featured` | Read each channel's featured channels shelf |
| `--music` | Read each video through YouTube Music too |
| `--uploads` | Put each channel's derived playlists on the frontier (default true) |
| `--resume` | Start from the nodes the store has heard of and not read |
| `--manifest` | Where to write the manifest (default `<data-dir>/crawls/crawl-<unix>.json`) |

## archive

`ytb archive <ref> [--flags]`. Write one read down in full: the page, every InnerTube payload, a `meta.json` naming each request and the file its answer went into, and a `record.json` with the records and claims ytb parsed.

It needs the cache on, since it works by replaying what the cache stored, so `--no-cache` is refused.
Session headers are removed from `meta.json` before it is written.

| Flag | Meaning |
| --- | --- |
| `--dir` | Where to write (default `<data-dir>/archive/<ref>-<unix>`) |
| `--items` | Playlist items to read, for a playlist ref |
| `--comments` | Comments to read, for a video ref |
| `--captions` | Read the caption list too |

## query

`ytb query <sql>`. Run one SQL statement over the store and print the rows. The file is opened read-only, so a statement that would write is refused by SQLite itself rather than by a check in ytb. No notable flags beyond the globals.

```sh
ytb query "select predicate, count(*) c from claims group by 1 order by c desc"
ytb query "select uri from nodes where kind='video' and record is null limit 20"
```

## export

`ytb export [channel-id|@handle] [--flags]`. Render the stored data as an interlinked Markdown site. With no argument, every channel in the store is exported.

| Flag | Meaning |
| --- | --- |
| `--out` | Output directory for the Markdown site |

## db

`ytb db [command]`. Inspect the store at `<data-dir>/ytb.db`. Pure-Go, no cgo.

Three tables. `nodes` is everything with an identity, one row per URI, with the record as JSON and a null record for a node a claim named that nobody has fetched.
`claims` is the edges, one row per observation, so the same edge seen on the watch page and in a browse response is two rows.
`reads` is the log: every request, what answered it, and how big it was.

| Subcommand | What it does |
| --- | --- |
| `stats` | Nodes by kind, claims by predicate, reads by surface and client |
| `search <query>` | Full-text search over stored data |
| `path` | Print the db file location |
| `vacuum` | Compact the database file |
| `reset` | Drop and recreate all tables |

Notable subcommand flags:

| Flag | Subcommand | Meaning |
| --- | --- | --- |
| `--channels` | `db search` | Search channels instead of videos |

## cache

`ytb cache [command]`. Inspect or empty the response cache at `<data-dir>/cache/innertube`.

Every read goes through it, keyed by the URL plus the client that claimed it, so a WEB player response is never served to a caption read that asked as ANDROID.
An entry older than `--cache-ttl` is not served and is not deleted either, so a cache can be almost entirely stale and still take up the whole of its space.

| Subcommand | What it does |
| --- | --- |
| `path` | Print the cache directory |
| `info` | Entries, how much space they take, how many are still fresh, and the age range |
| `clear` | Delete every entry, fresh or stale |

```console
$ ytb cache info
╭─────────┬───────────────────────────────────────────────╮
│ KEY     │ VALUE                                         │
├─────────┼───────────────────────────────────────────────┤
│ dir     │ /Users/apple/.local/share/ytb/cache/innertube │
│ ttl     │ 15m0s                                         │
│ entries │ 350                                           │
│ fresh   │ 4                                             │
│ stale   │ 346                                           │
│ size    │ 263.7 MiB                                     │
│ oldest  │ 2026-07-30T09:23:06+07:00                     │
│ newest  │ 2026-08-12T10:40:09+07:00                     │
╰─────────┴───────────────────────────────────────────────╯
```

Fresh and stale are counted against the `--cache-ttl` in effect for this run rather than stored, so the same 350 files come back 109 fresh under `ytb cache info --cache-ttl 24h`.

`clear` takes `--dry-run`, which reports the count and the size it would free and deletes nothing.
Nothing in here is a record: the store keeps what was parsed and this keeps the bytes a request answered with, so clearing it costs time on the next run and loses nothing.
`ytb archive` writes outside this cache and is never touched by `clear`.

All three exit 2 with a usage error under `--no-cache`, rather than printing an empty path or a count of zero and letting you read that as an empty cache.

## auth

`ytb auth [command] [--flags]`. Manage your YouTube session, which is optional and which the [signing in](/guides/signing-in/) guide covers in full.

| Subcommand | What it does |
| --- | --- |
| `import` | Store the session cookies out of a browser you are already signed into |
| `status` | Name the stored cookies, without their values, and say what they unlock |
| `clear` | Delete the local cookie file (alias `logout`) |

| Flag | Subcommand | Meaning |
| --- | --- | --- |
| `--cookies` | `auth import` | A cookies.txt path, a pasted Cookie header, or `-` for stdin |

## config

`ytb config [command] [--flags]`. View and manage configuration.

| Subcommand | What it does |
| --- | --- |
| `show` | Print the resolved configuration |
| `path` | Print the config file path |
| `init` | Write a commented config template |
| `edit` | Open the config file in `$EDITOR` |

## version

`ytb version [--flags]`. Print version information.

| Flag | Meaning |
| --- | --- |
| `--short` | Print just the version number |
