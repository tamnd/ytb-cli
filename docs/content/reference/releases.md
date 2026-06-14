---
title: "Release notes"
description: "What changed in each ytb release."
weight: 35
---

Every release ships as prebuilt archives, Linux packages (deb, rpm, apk), and a
multi-arch container image, each with checksums, an SBOM, and a cosign
signature. Grab one from the [releases page] or install the latest with
`go install`.

[releases page]: https://github.com/tamnd/ytb-cli/releases

## v0.3.0

Output got a face-lift. Every list command renders through a redrawn formatter
built on [lipgloss](https://github.com/charmbracelet/lipgloss).

- **`-o table`** is now a rounded-border grid with an accented bold header and a
  dim border, colored on a terminal and plain in a pipe.
- **`-o markdown`** (alias `md`) — a GitHub pipe table you can paste straight
  into a README, issue, or PR. Pipes inside titles are escaped, so the table is
  always valid Markdown.
- **`-o json` and `-o jsonl`** are syntax-highlighted on a terminal: keys,
  strings, numbers, and literals each get a color. A pipe still receives plain,
  parseable bytes, so `ytb ... | jq` is unaffected.
- **`--color auto|always|never`** controls all of the above and honors
  `NO_COLOR`. The default colors only an interactive terminal.
- A too-wide table **shrinks to fit the terminal** instead of wrapping at the
  edge.

None of the existing formats or flags changed, so scripts that pipe `ytb` keep
working byte for byte.

## v0.2.0

Rebuilt `ytb` on the shared [any-cli/kit](https://github.com/tamnd/any-cli)
framework. One operation registry now backs the CLI, the `serve` HTTP surface,
and the `mcp` tool set, and every command shares the same output contract
(`-o`, `--fields`, `--template`, `-n`).

## v0.1.1

Maintenance release.

## v0.1.0

First public release: video, channel, playlist, search, comments, transcripts,
downloads, community and hashtag feeds, YouTube Music, and a local SQLite store,
from one pure-Go binary with no API key.
