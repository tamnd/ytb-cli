---
title: "Output formats"
description: "Pick a format with -o, project columns with --fields, or template each row."
weight: 30
---

Every list command renders through the same formatter, so what you learn for
`search` applies to `video`, `channel`, `comments`, and the rest. Pick a format
with `-o, --output`, narrow the columns with `--fields`, or reshape each row with
`--template`.

## Choosing a format

By default the format is `auto`: a human-readable table when ytb is writing
to a terminal, and JSONL when the output is piped or redirected. That way reading
in a shell and piping into another program both do the right thing with no flag.

Override it with `-o`:

```sh
ytb search example -o table   # aligned columns for reading
ytb search example -o json    # a single JSON array
ytb search example -o jsonl   # one JSON object per line, for piping
ytb search example -o csv     # comma-separated, spreadsheet friendly
ytb search example -o tsv     # tab-separated
ytb search example -o url     # just the canonical URL, one per line
ytb search example -o id      # just the id, one per line
ytb search example -o raw     # the raw underlying value
```

`table`, `csv`, and `tsv` carry a header row; pass `--no-header` to drop it.
`json` emits one array for the whole result, while `jsonl` emits one object per
line so it streams and composes with tools like `jq -c`.

## Selecting columns with --fields

`--fields` takes a comma-separated list of table column names. These are the
short, lowercase names you see in the table header, not the JSON keys:

```sh
ytb search "rust tutorial" -n 100 --fields title,channel,views
ytb video dQw4w9WgXcQ --fields title,channel,views -o table
```

The column names differ from the snake_case keys in JSON output. For example the
table column is `views`, while the same value in `-o json` is `view_count`. Use
the column names with `--fields` and the JSON keys when you parse JSON downstream.

## Templating with --template

`--template` is a Go `text/template` applied once per row. Templated fields use
the Go struct field names, which are capitalized, so `views` on the table becomes
`{{.Views}}` here:

```sh
ytb search example --template '{{.Title}} ({{.Views}})'
ytb video dQw4w9WgXcQ --template '{{.ID}} {{.Title}} {{.Channel}}'
```

A template wins over `--fields` and `-o`, since you are describing the exact line
to print.

## Piping ids and urls

`-o id` and `-o url` exist for chaining. Any command that takes an id or URL also
accepts `-` to read a list from stdin, so the output of one command feeds the
next:

```sh
ytb search "go programming" -o id | ytb video -
ytb playlist PLFgquLnL59alCl_2TQvOiD5Vgm1hCaGSI -o id | ytb video -
```

The first command emits one id per line, and `ytb video -` resolves each to
full metadata.
