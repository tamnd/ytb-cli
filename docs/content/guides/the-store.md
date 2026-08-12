---
title: "The local store"
description: "Crawl the graph into SQLite, then query the nodes, claims, and reads with SQL."
weight: 60
---

Most commands are stateless: they stream straight to stdout and keep nothing.
`ytb crawl` is the one that writes.
It reads a seed, writes down every claim that read made, then goes and reads what those claims named, hop by hop, until the depth or the budget runs out.
Everything lands in one SQLite file at `<data-dir>/ytb.db`, which `ytb db path` will print.

```sh
ytb crawl @RickAstleyYT --depth 2 --budget 200
ytb crawl UCuAXFkgsw1L7xaCfnd5JJOw --depth 1 --budget 50
```

The store is pure Go (modernc.org/sqlite), so nothing links libsqlite and the binary stays static.

## Three tables

`nodes` is everything that has an identity, one row per URI, with the record as JSON.
A node whose `record` is null is one that somebody named and nobody has fetched yet, which is most of them after any crawl: a watch page names thirty videos and reads one.

`claims` is the edges, one row per observation rather than one per fact.
The same edge seen on the watch page and in a browse response is two rows, and each carries the surface, the client, and the tier it came from, so a claim that only one client ever made is a query away.
A lockup also carries a title and a position, and those ride along on the claim as a note rather than as a record, because a title on a shelf is a label and not an assertion about the object.

`reads` is the log: every request, which surface and client answered it, the status, the byte count, and the time.
A cache hit makes no request, so it is not in here and it did not cost budget.

```sh
ytb db stats                                  # nodes by kind, claims by predicate, reads by surface
ytb db path                                   # where the file lives
```

## The frontier is a query

There is no queue table.
The work left to do is just the nodes nobody has read:

```sh
ytb query "select uri from nodes where record is null and kind='video' limit 20"
```

That is why a crawl can be picked up by a later run with nothing passed between them.
`--resume` with no seeds at all starts from the frontier:

```sh
ytb crawl --resume --budget 50
```

The frontier is ordered cheapest first, so listings are read before watch pages: one playlist browse comes back with ninety-eight items while a watch page is 1.3 MB for one video.
Four things are off it.
A mix (`RD...`) is generated per viewer and browses to nothing.
A channel's popular playlist (`UULP`) is its uploads in another order.
Comments are refused at tier 0 on most addresses, and `--comments` turns them back on.
Formats need a player call per video and produce no claim beyond the caption tracks.
None of them is refused as a seed: asking for one by name is a different question.

The budget is counted in requests, not estimated.
It is checked before each reference rather than before each request, because stopping halfway through reading one object stores half of it, so a crawl can finish a few requests over.

Every crawl writes a manifest under `<data-dir>/crawls/` with the seeds, the budget, what it spent, which surfaces and clients answered, and every refusal with its reason quoted.
A crawl that hit a wall and a crawl that ran out of budget leave the same store and different manifests.

## Persist a walk

`ytb discover --store` tees a breadth-first walk into the store as a side effect.
A node the walk fetched is stored as a record; a node it only saw in somebody else's shelf is stored as a sighting with no record, so a later crawl still knows to go and read it.
It writes nodes and not claims: `--follow related` is a walk instruction rather than one of the predicates a claim is made of.

```sh
ytb discover @MrBeast --follow all --depth 2 --store
```

See [graph discovery](/guides/graph-discovery/) for the full edge and preset vocabulary.

## Querying the store

`ytb query` runs one SQL statement and prints the rows.
The file is opened read-only, so a statement that would write is refused by SQLite itself rather than by a check in ytb.

```sh
ytb query "select predicate, count(*) c from claims group by 1 order by c desc"
ytb query "select json_extract(record,'$.title') from nodes where kind='channel'"
ytb query "select surface, client, count(*), sum(bytes) from reads group by 1,2"
```

`db search` runs a full-text search over stored data.
It searches videos by default; pass `--channels` to search channels instead.

```sh
ytb db search "mukbang"
ytb db search "tech" --channels
```

`db vacuum` compacts the file.
`db reset` drops and recreates the tables, so it prompts unless you pass `-y`.

```sh
ytb db vacuum
ytb db reset -y
```

## Keeping the bytes

`ytb archive <ref>` is the other half: it writes one read down in full into a directory.
The page, every InnerTube payload with the endpoint in its name, a `meta.json` naming the requests and the file each answer went into (with the session headers removed), and a `record.json` with the records and claims ytb parsed out of them.
It needs the cache on, since it works by replaying what the cache stored.

```sh
ytb archive dQw4w9WgXcQ
ytb archive PLFgquLnL59alCl_2TQvOiD5Vgm1hCaGSI --items 60 --dir ./capture
```

## Exporting to Markdown

`export` renders the stored data as an interlinked Markdown site under `--out`: per-video pages with YAML frontmatter, chapter lists, transcripts, related sidebars, and channel and playlist index pages.
With no argument it exports every channel in the store; pass a channel id or `@handle` to scope it.

```sh
ytb export --out site/           # the whole store
ytb export @MrBeast --out site/  # one channel
```

## A note on the old store

The schema changed.
A `ytb.db` written by an older version had a `videos` table and an `edges` table, and this one will not open it: ytb recognises the old file by name and refuses it rather than migrating it.
Move it aside and crawl again.
