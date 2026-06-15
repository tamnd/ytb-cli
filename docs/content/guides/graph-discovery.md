---
title: "Graph discovery"
description: "Walk the graph linked from a video, channel, or playlist breadth-first with ytb discover: follow uploaders, related videos, uploads, playlists, items, owners, community posts, comments, and the channels behind them."
weight: 55
---

Every other read answers one question about one object: a video's metadata, a
channel's uploads, a playlist's items. `ytb discover` chains them. It starts at a
video, channel, or playlist and follows that object's links outward, hop by hop,
streaming every node it reaches. It is a breadth-first walk of the YouTube graph.

```sh
ytb discover dQw4w9WgXcQ      # what is this video linked to?
ytb discover @MrBeast         # what is this channel linked to?
```

A *seed* is any reference ytb already resolves: a video id or watch URL, a
channel id, `@handle`, or URL, or a playlist id or URL. Pass more than one to
start the walk from several places at once.

## What gets followed

The default follows an object's **content**: a video yields its uploader and
related videos, a channel yields its uploads, a playlist yields its items and
owner. It spans every seed kind, so `ytb discover <anything>` does the obvious
thing with no flags, and it stays on the open surface, so it needs nothing
special.

Choose what to follow with `--follow`. It takes a preset:

```sh
ytb discover <ref> --follow content    # uploader, related, uploads, items, owner (default)
ytb discover <channel> --follow feed   # uploads, playlists, community posts
ytb discover <video> --follow comments # comments and the channels that wrote them
ytb discover <ref> --follow all        # every edge
```

or a comma-separated list of individual edges:

```sh
ytb discover <video> --follow channel,related
ytb discover <channel> --follow uploads,playlists
```

The full edge vocabulary:

| Edge | From → to | Gated | What it follows |
|---|---|---|---|
| `channel` | video → channel | no | the channel that uploaded the video |
| `related` | video → video | no | a related / recommended video |
| `comments` | video → comment | yes | a comment on the video |
| `uploads` | channel → video | no | a video the channel uploaded |
| `playlists` | channel → playlist | no | a playlist the channel owns |
| `community` | channel → post | no | a community-tab post |
| `items` | playlist → video | no | a video in the playlist |
| `owner` | playlist → channel | no | the channel that owns the playlist |
| `commenter` | comment → channel | yes | the channel that wrote the comment |

Name an edge directly to chase just that link: `--follow related` walks the
recommendation graph, `--follow uploads` sweeps a channel.

YouTube serves the open surface to an anonymous request, so there are no tiers to
add. The only gated edges are the two that touch comments: YouTube applies a
per-IP Restricted Mode to some networks, and when it refuses the comments,
`ytb discover` notes it on stderr and keeps going on the rest of the graph rather
than failing the walk. So `--follow all` from a flagged network still returns the
channel, related videos, playlists, and everything else, with one advisory about
the comments it could not read.

## How far and how wide

```sh
ytb discover <ref> --depth 2          # follow two hops from the seed (default 1)
ytb discover <ref> --fanout 50        # up to 50 neighbors per edge (default 25)
ytb discover <ref> --fanout 0         # no per-edge cap
ytb discover <ref> -n 1000            # stop after 1000 nodes total (default 500)
```

`--depth` is how many hops to follow; `0` emits only the seeds. `--fanout` caps
how many neighbors each edge contributes per node, so one hop never pages a whole
upload history unless you raise it. `-n/--limit` is the total node budget, the
hard stop on a deep or wide walk; even `--fanout 0` stays bounded by it, so a
walk always terminates.

## Reading the output

`ytb discover` streams one row per node, tagged with how it was reached:

```text
depth  via      kind     id           who        summary                url
0               video    dQw4w9WgXcQ  RickAstley  Never Gonna Give You U https://youtu.be/dQw4w9WgXcQ
1      channel  channel  UCuAXFkgsw1L  RickAstley  Rick Astley           https://youtube.com/channel/UCuAXFkgsw1L
1      related  video    J---aiyznGQ  ...         Keyboard Cat           https://youtu.be/J---aiyznGQ
```

Because it streams through the same formatter as every read, it shapes and pipes
the same way. The JSON forms carry the full typed node, with the nested video,
channel, playlist, comment, or post:

```sh
ytb discover <ref> -o json | jq -r '.via + " -> " + (.video.id // .channel.id)'
ytb discover @MrBeast --follow feed -o jsonl | jq -r '.video.id' | sort -u
ytb discover <ref> --fields depth,via,who,url -o table
```

## Persisting a walk

Add `--store` to write every node and edge into the local store as the walk
streams, so you keep the graph as well as see it:

```sh
ytb discover @MrBeast --follow all --depth 2 --store
ytb db query "select kind, count(*) from edges group by kind order by 2 desc"
```

Each node lands in its own typed table (`videos`, `channels`, `playlists`,
`comments`, `community_posts`) and each traversed link lands in an `edges` table
(`src`, `dst`, `kind`), so the graph is queryable afterwards. Re-walking is
idempotent.

When you want a dataset built from an explicit worklist rather than a live walk,
reach for [the crawl queue](/guides/the-store/), which drains a queue of URLs you
load yourself; `discover` is the complement that finds the worklist by walking.
See [the local store](/guides/the-store/) for inspecting and exporting what you
collect.
