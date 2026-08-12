---
title: "Claims"
description: "What one read asserts, who asserted it, and how to walk or export the graph those assertions make."
weight: 57
---

Every other command in ytb answers "what is this thing".
Four commands answer a different question: what did this read claim, and on whose word.

```sh
ytb edges dQw4w9WgXcQ         # what one read claims
ytb graph dQw4w9WgXcQ         # walk the nodes those claims name
ytb predicates                # the vocabulary, no request
ytb rdf dQw4w9WgXcQ           # the same claims as n-triples, turtle or json-ld
```

## A claim is an edge with its provenance attached

A row from `edges` is a subject, a predicate, an object, and the URL that said so:

```sh
ytb edges dQw4w9WgXcQ -o table --fields predicate,to,surface -n 5
```

The source is part of the claim's identity, not a note on the side.
Two surfaces asserting the same edge stay two rows, so a disagreement between the watch page and the mobile player is something you can query rather than something the later read overwrote.
That is the whole reason this plane exists separately from the records: a record is what ytb thinks is true, and a claim is what somebody said.

Alongside the source, each claim carries the surface it came from (`s1` to `s11`), the client ytb told YouTube it was (`WEB`, `ANDROID`, `WEB_REMIX`, or empty for a plain HTML read), and the tier, which is 0 without cookies and 1 with them.

## What one watch page already knows

Read one video and count what it claimed:

```sh
ytb edges dQw4w9WgXcQ -o jsonl | jq -r .predicate | sort | uniq -c | sort -rn
```

```
  30 related_to
  21 published
  11 links_to
   6 has_captions
   5 tagged
   1 uploads_of
```

Seventy four claims from one request.
Thirty of them name other videos, and twenty one name the channels that published those videos, none of which ytb has fetched.
The description's eleven external links, the six caption tracks and the five hashtags are all in there too.

This is the point of the plane.
A single watch page names dozens of nodes, and the cheapest crawl is the one that harvests what it already has in hand before asking for anything else.

Each of the flags below is another read, and each says what it costs:

| Flag | What it costs |
| --- | --- |
| `--captions` | Read the ANDROID player for the caption list (1 request) |
| `--comments` | Read up to n comments (1 request per page of about 20) |
| `--featured` | Read a channel's home tab for its featured channels (1 request) |
| `--items` | Read up to n playlist items (1 request per page) |
| `--music` | Read the same id through YouTube Music (1 request) |
| `--posts` | Read up to n community posts (the tab refuses tier 0 today) |

A handle costs one more request than an id, because a handle is not an identity and can never be a node.
Something has to resolve it first.

## The vocabulary is closed

```sh
ytb predicates
```

Twenty one predicates, each with the kinds of node it runs between, the RDF term it maps to, and where in a page it comes from.
`ytb predicates` makes no request at all.

The table being closed is the point.
A predicate not in it cannot be written, which is what stops a typo becoming a claim that looks fine, is never queried because nobody knows to ask for it, and turns up a year later in somebody's count.

Most of the RDF column is schema.org, and most of that comes from the markup YouTube publishes on its own pages rather than from anything invented here.
Exactly five terms had no schema.org equivalent and live under a `yt:` namespace: `yt:features`, `yt:inPlaylistAfter`, `yt:seenAs`, `yt:tagged` and `yt:uploadsOf`.

Where the arrow turns round on the way out, the table says so.
ytb writes `channel published video`, because that is the direction a page reads in, and `schema:author` runs from the work to its author.

## Walking the frontier

`edges` is depth 0: one read and what it claimed.
`graph` follows the nodes those claims named:

```sh
ytb graph dQw4w9WgXcQ --depth 1 --budget 25
```

`--budget` is in requests, and it is counted rather than estimated.
Every request that goes out passes a hook the walk counts on.

A cache hit never reaches that hook, because the counting happens in the HTTP transport and a cache hit never gets that far.
So a cache hit is not a request, does not count, and a second walk over the same seeds gets further on the same budget.
That is a feature worth using deliberately: walk with a small budget, then walk again.

The budget is checked between references, and a reference is read whole.
A channel costs three requests, so a walk with one request left can finish three past its budget.
The note says both numbers rather than leaving them to disagree:

```
note: budget of 2 requests reached at hop 1, 4 spent
79 claims over 78 nodes, 4 requests spent
```

Only nodes ytb can actually read are followed.
An external URL and a hashtag get named and never fetched, which is why the frontier is mostly videos.

`--edges` prints the claims themselves instead of the per-predicate counts.

### graph against discover

The two look similar and answer different questions.
`discover` streams records, one row per node it reached, and tells you what is out there.
`graph` streams claims, and tells you who said so.
Reach for `discover` when you want the things, and `graph` when you want the assertions about them.

## Exporting as RDF

Same claims, standard serializations:

```sh
ytb rdf dQw4w9WgXcQ --format nt
ytb rdf dQw4w9WgXcQ --format turtle
ytb rdf dQw4w9WgXcQ --format jsonld
```

Provenance survives the trip.
In n-triples and turtle each claim becomes a quoted triple with a `prov:wasDerivedFrom`:

```
<< <yt://video/14zr3yAbm_c> <https://schema.org/author> <yt://channel/UCN_ov9pbPW9RCuOzp--l_yw> >> <http://www.w3.org/ns/prov#wasDerivedFrom> <https://www.youtube.com/watch?v=dQw4w9WgXcQ> .
```

JSON-LD has no quoted triples, so there the same information becomes a named graph per source.
`--no-provenance` drops it entirely when you only want the data, and `--types-only` emits the `rdf:type` statements alone.

### Checking ytb against the page

`--check` puts what ytb parsed beside what the page's own schema.org markup says, predicate by predicate:

```sh
ytb rdf dQw4w9WgXcQ --check
```

Most rows agree outright.
Some agree after normalising, like `schema:author`, where ytb writes the canonical `yt://channel/UC...` and the page writes the handle URL.
Some the page simply does not carry: it has no `schema:caption`, no `schema:citation` and no `schema:relatedLink`, because those come from parts of the response that never make it into the markup.

The interesting rows are the ones that disagree.
On this video there is exactly one:

```
schema:thumbnailUrl
  ytb   https://i.ytimg.com/vi_webp/dQw4w9WgXcQ/maxresdefault.webp
  page  https://i.ytimg.com/vi/dQw4w9WgXcQ/maxresdefault.jpg
```

Both URLs work.
The player hands out the webp and the markup advertises the jpg, and neither is wrong, which is the kind of thing `--check` is for: it finds the places where two parts of the same response say different things, and leaves the judgement to you.

## Persisting claims

`edges`, `graph` and `rdf` all stream and keep nothing.
`ytb crawl` walks the same way and writes the nodes, the claims and a log of every read into the local store, where the claims table keeps one row per source.
See [the store](/guides/the-store/).
