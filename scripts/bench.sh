#!/usr/bin/env bash
# bench.sh — exercise every ytb command and record exit code, duration, and a
# coarse status (OK / EMPTY / GATED / NOTOOL / FAIL). Writes one TSV row per
# command to stdout. Designed to run identically on macOS and Linux.
#
#   usage: bench.sh [path-to-ytb-binary]
#
# Status meaning:
#   OK     exit 0 with at least one output row
#   EMPTY  exit 0 with no rows, or exit 3 (no results)
#   GATED  exit 4 (partial / Restricted Mode / anti-bot)
#   NOTOOL exit 6 (external tool such as yt-dlp absent)
#   FAIL   any other non-zero exit

set -u

BIN="${1:-./bin/ytb}"
DB="$(mktemp -t ytbbench.XXXXXX.db)"
TMPDIR_OUT="$(mktemp -d -t ytbbench.XXXXXX)"
trap 'rm -f "$DB"; rm -rf "$TMPDIR_OUT"' EXIT

# now_ms prints the current time in integer milliseconds (perl is on both OSes).
now_ms() { perl -MTime::HiRes=time -e 'printf "%.0f", time()*1000'; }

printf 'command\texit\tms\trows\tstatus\n'

# run <label> <args...> : time the command, count stdout rows, classify.
run() {
	local label="$1"; shift
	local out rc start end ms rows status
	start="$(now_ms)"
	out="$("$BIN" "$@" 2>/dev/null)"; rc=$?
	end="$(now_ms)"
	ms=$((end - start))
	if [ -z "$out" ]; then rows=0; else rows="$(printf '%s\n' "$out" | grep -c .)"; fi
	case "$rc" in
		0) if [ "$rows" -gt 0 ]; then status=OK; else status=EMPTY; fi ;;
		3) status=EMPTY ;;
		4) status=GATED ;;
		6) status=NOTOOL ;;
		*) status=FAIL ;;
	esac
	printf '%s\t%s\t%s\t%s\t%s\n' "$label" "$rc" "$ms" "$rows" "$status"
}

# first_id <args...> : run a command, return its first stdout line (for discovery).
first_id() { "$BIN" "$@" 2>/dev/null | grep . | head -1; }

VID="dQw4w9WgXcQ"
CH="@MrBeast"
PLCH="@mkbhd"   # a channel with a conventional playlists tab
Q="lofi hip hop"

# ---- discovery (search/channel are not gated even from a flagged IP) ----
PLAYLIST="$(first_id channel "$PLCH" --playlists -n 1 -o id)"
[ -z "$PLAYLIST" ] && PLAYLIST="$(first_id search "$Q" --type playlist -n 1 -o id)"
ARTIST="$(first_id music search --type artist "daft punk" -n 1 -o id)"
ALBUM="$(first_id music search --type album "discovery daft punk" -n 1 -o id)"
MPLIST="$(first_id music search --type playlist "lofi" -n 1 -o id)"
MSONG="$(first_id music search --type song "blinding lights" -n 1 -o id)"
[ -z "$MSONG" ] && MSONG="$VID"

# ---- core read commands ----
run "video"              video "$VID"
run "channel"            channel "$CH"
run "channel-videos"     channel "$CH" --videos -n 5
run "channel-playlists"  channel "$PLCH" --playlists -n 5
run "search"             search "$Q" -n 10
run "search-channel"     search "$CH" --type channel -n 5
run "trending"           trending -n 10
run "trending-music"     trending --category music -n 10
run "related"            related "$VID" -n 10
run "suggest"            suggest "$Q"
run "hashtag"            hashtag lofi -n 10
run "formats"            formats "$VID"
run "transcript-list"    transcript "$VID" --list

# ---- gated-or-works (depends on host IP) ----
run "transcript-text"    transcript "$VID"
run "comments"           comments "$VID" -n 10
run "community"          community "$CH" -n 5

# ---- playlist (needs discovered id) ----
if [ -n "$PLAYLIST" ]; then
	run "playlist"       playlist "$PLAYLIST" -n 10
else
	printf 'playlist\t-\t0\t0\tSKIP\n'
fi

# ---- music ----
run "music-search"       music search "$Q" -n 10
run "music-song"         music song "$MSONG"
[ -n "$ARTIST" ]  && run "music-artist"   music artist "$ARTIST"   || printf 'music-artist\t-\t0\t0\tSKIP\n'
[ -n "$ALBUM" ]   && run "music-album"    music album "$ALBUM"     || printf 'music-album\t-\t0\t0\tSKIP\n'
[ -n "$MPLIST" ]  && run "music-playlist" music playlist "$MPLIST" || printf 'music-playlist\t-\t0\t0\tSKIP\n'

# ---- store (sqlite, --db) ----
run "db-reset"           db reset --db "$DB" --yes
run "seed-search"        seed --entity search "$Q" --db "$DB" -n 10
run "crawl"              crawl --entity search --db "$DB" --max-pages 1
run "db-stats"           db stats --db "$DB"
run "db-query"           db query "select count(*) from videos" --db "$DB"
run "db-search"          db search "$Q" --db "$DB"
run "db-path"            db path --db "$DB"
run "queue"              queue --db "$DB"
run "jobs"               jobs --db "$DB"
run "export"             export --db "$DB" --out "$TMPDIR_OUT"
run "db-vacuum"          db vacuum --db "$DB"

# ---- yt-dlp backed (NOTOOL if absent) ----
run "extract-transcript" extract transcript "$VID" --out "$TMPDIR_OUT"
run "download-audio"     download "$VID" --audio --out "$TMPDIR_OUT"

# ---- meta ----
run "version"            version
run "config-path"        config path
