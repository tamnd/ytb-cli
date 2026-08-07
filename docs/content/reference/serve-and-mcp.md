---
title: "Serve and MCP"
description: "The same reads over HTTP and as MCP tools, generated from the operation table."
weight: 32
---

Every read in ytb is one registration, and that registration is three surfaces: a command, an HTTP route under `ytb serve`, and an MCP tool under `ytb mcp`.
Neither server adds a read the command line does not have, and neither can be missing one, because there is nothing to keep in sync.
A test walks the commands and fails when a read has no operation behind it.

## ytb serve

```sh
ytb serve --addr :8080
```

One route per read, at `/v1/<verb>`, answering NDJSON: one JSON object per line, flushed as it is read.
A long `uploads` starts arriving before the last page is fetched, which is the point of the format.

```sh
curl -s localhost:8080/v1/video?ref=dQw4w9WgXcQ
curl -s "localhost:8080/v1/uploads?ref=@RickAstleyYT&kind=shorts&limit=5"
curl -s "localhost:8080/v1/music/search?query=rick+astley&type=song"
```

Arguments go on the path or in the query, and the query form is the one to reach for.
A path splits on slashes, so an argument that contains one cannot survive the trip, and half of what you pass ytb is a URL.
`/v1/video?ref=https://youtu.be/dQw4w9WgXcQ` works; the path form of the same thing does not.

Repeat a parameter to pass more than one value to a read that takes a list:

```sh
curl -s "localhost:8080/v1/video?ref=dQw4w9WgXcQ&ref=GlYgs6v2YfU"
```

Flags are query parameters under their own names, spelled the way the command spells them.
`--max-pages 3` is `&max-pages=3`, `--no-player` is `&no-player=true`, and `-n 5` is `&limit=5`.

The routes are these, which is every read the binary has:

| Group | Routes |
| --- | --- |
| Video | `video`, `formats`, `captions`, `transcript`, `chapters`, `thumbnail`, `sponsorblock`, `comments`, `related` |
| Channel | `channel`, `about`, `uploads`, `feed`, `playlists`, `community` |
| Playlist | `playlist`, `items` |
| Search | `search`, `suggest`, `hashtag`, `trending` |
| Music | `music/search`, `music/artist`, `music/album`, `music/playlist`, `music/track` |
| Graph | `edges`, `graph`, `discover`, `predicates` |
| Offline | `id` |

`GET /v1/openapi.json` is the generated spec, with every argument and flag listed as a query parameter.
It is the answer to what a route takes, and it cannot go stale, because it is read off the same table the routes are.

What is not there is anything that writes.
`download` and `extract` write a file, `crawl` and `archive` and `export` write into the local store or a directory, `db` and `query` are about that store, and `config` is about this machine.
A GET that streams a video to the server's disk for ten minutes is not a read, so it is not a route.
`--allow-writes` is a kit flag that every binary in the series has, and here it exposes nothing, because there is nothing registered for it to expose.

## ytb mcp

```sh
ytb mcp
```

The same operations as MCP tools over stdio, named with an underscore where the route has a slash: `music_search`, not `music/search`.
Arguments and flags are the tool's input schema, generated from the same declaration, so a model gets the help text the command's `--help` prints.

Point a client at the binary:

```json
{
  "mcpServers": {
    "ytb": { "command": "ytb", "args": ["mcp"] }
  }
}
```

Everything is a read, so there is nothing here that can change anything.
The worst a client can do is spend requests.

## Neither needs a credential

`ytb serve` and `ytb mcp` read what the command line reads, which is public YouTube, with no API key and no Google Cloud project.
There is no token to configure and none to leak.

The one thing to keep in mind is the address.
`--addr` defaults to `:8080`, which is every interface, and whatever the process can read, anything that can reach that port can read, at your address and your rate limit.
Pass `--addr 127.0.0.1:8080` unless you meant to share it.
