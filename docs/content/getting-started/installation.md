---
title: "Installation"
description: "Install ytb from a release, with go install, or from source. yt-dlp is optional."
weight: 20
---

## Prebuilt binaries

Every [release](https://github.com/tamnd/ytb-cli/releases) carries archives
for Linux, macOS, and Windows on amd64 and arm64, plus deb, rpm, and apk packages
for Linux. Download, unpack, put `ytb` on your `PATH`, done. The
`checksums.txt` on each release is signed with keyless
[cosign](https://docs.sigstore.dev/) if you want to verify before running.

## With Go

```bash
go install github.com/tamnd/ytb-cli/cmd/ytb@latest
```

That puts `ytb` in `$(go env GOPATH)/bin`, which is `~/go/bin` unless you
moved it. Make sure that directory is on your `PATH`.

## With Docker

```bash
docker run --rm ghcr.io/tamnd/ytb video dQw4w9WgXcQ -o json
```

Mount a volume and point `--db` at it to keep a SQLite store across runs:

```bash
docker run --rm -v "$PWD/data:/data" ghcr.io/tamnd/ytb \
  channel @MrBeast --videos --db /data/yt.db
```

## From source

```bash
git clone https://github.com/tamnd/ytb-cli
cd ytb-cli
make build        # produces ./bin/ytb
./bin/ytb version
```

## Optional: yt-dlp

Two features shell out to [yt-dlp](https://github.com/yt-dlp/yt-dlp) when it is
on your `PATH`:

- `ytb download` and `ytb extract` for media files, and
- `ytb transcript` text, when YouTube gates the raw caption endpoint behind a
  proof-of-origin token (it usually does now). Listing tracks with
  `transcript --list` never needs it.

If yt-dlp is absent, those two paths report a clear, actionable error and exit;
everything else in ytb works without it. The ytb binary never links
yt-dlp, so the install stays small and pure Go.

## Requirements

- **Go 1.26 or later** to build. The released binary has no Go requirement.
- **A `yt-dlp` binary** only for media download and gated-transcript text.

That is the whole list. No API key, no config file, no database to provision, no
daemon.

## Checking the install

```bash
ytb version
```

prints the version and exits. Then confirm it can reach YouTube:

```bash
ytb suggest "lofi"
```

should print a handful of autocomplete suggestions. If you see those, you are
ready for the [quick start](/getting-started/quick-start/).
