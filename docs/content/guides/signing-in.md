---
title: "Signing in"
description: "Optional: hand ytb the cookies your browser already has, and what that changes."
weight: 70
---

Every other page of these docs works signed out, with no key, no account and no quota, and that is the point of the tool.
This page is about the small number of things it cannot reach that way.

## What a session buys

Five things, and nothing else.

| Unlocked | Why it is gated |
| --- | --- |
| Comments on a network with Restricted Mode on | Restricted Mode is set on the network or the account, not the video, and it hides the comment thread from a signed-out reader |
| The community tab of a channel that gates it | The tab answers a signed-out client with "Posts aren't currently available on this device" |
| Age-restricted videos | The watch page returns a refusal instead of a player response |
| Members-only videos and posts | They are only listed to an account that is a member |
| Your own subscriptions, playlists and history | They are yours, so there is nobody else to ask |

If none of those are in your way, do not sign in.
A signed-out dataset is one anybody else can reproduce, and that is worth more than five extra shelves.

## Importing

`ytb` never asks for a password, never drives a login form and never touches a consent screen.
It takes the cookies your browser already has, in either of the two formats people actually have them in.

```sh
ytb auth import --cookies ~/Downloads/cookies.txt   # a Netscape cookies.txt from a browser extension
pbpaste | ytb auth import --cookies -               # a Cookie header pasted out of the network panel
ytb auth import --cookies 'SID=...; SAPISID=...'    # or the same header as an argument
```

Only the session cookies are kept and the rest of the export is dropped, which for a real cookies.txt is most of it.
`PREF` is dropped on purpose: `ytb` sets it per request from `--hl` and `--gl`, and a stored copy would quietly pin every read to the language your browser happened to be in.
An export missing `SAPISID` or `SID` is refused rather than stored, because a partial session is a session that fails on the first request with no clue why.

## Checking

```sh
ytb auth status
```

```
PRESENT  TIER  COOKIES                                MISSING  SOURCE          IMPORTED
true     1     HSID, SAPISID, SID, __Secure-3PAPISID           ~/cookies.txt   2026-08-12 01:34
```

The command names the cookies and never prints one, in any output format, so it is safe to run in front of other people or in a recording.

## Where the cookies go

Into the data directory, mode 0600, and into a request header to youtube.com.
Nowhere else, and the list of nowhere else is the part worth reading.

- Not to the CDN.
  A media request goes to `googlevideo.com`, which is a different host, and the session is not attached to it.
- Not into the cache.
  A cache key becomes a filename, so the key carries a short hash of the account rather than anything belonging to it.
- Not into the store.
  Records are what a read returned, and a cookie is not that.
- Not into an archive.
  `ytb archive` writes the request headers down beside the payload and replaces the ones that carried a session with a sentence saying one was sent, because a capture with a blank `Cookie` header describes a different request from the one that was made.

`Authorization: SAPISIDHASH` is computed fresh for each request from the timestamp, the cookie and the origin, the same way the site computes it in the browser.
Nothing is stored precomputed and nothing is reused.

## What it changes about the records

A record fetched with a session says so.

```sh
ytb video dQw4w9WgXcQ -o json | jq -c '.[0] | {tier, surfaces}'
```

```json
{"tier":1,"surfaces":["s1","s11"]}
```

`tier: 1` means this record needed an account, and `s11` is the session surface listed alongside whichever surface actually answered.
Every record kind carries both, so a mixed dataset can always be split back into the half anybody can reproduce and the half only you can.
The cache splits on the same line, which is what stops an age-restricted page fetched signed in from later being served to a signed-out read.

## Forgetting

```sh
ytb auth clear
```

That deletes the local file.
It does not sign your browser out and it does not tell YouTube anything, because it never talks to YouTube at all.
To sign out for real, sign out in the browser, which invalidates the cookies wherever they were copied to.

## What it cannot do

Nothing in this binary writes to YouTube.
There is no code path that posts a comment, likes a video, subscribes, or deletes anything, so a session handed to `ytb` cannot do any of it either.
`auth` is also the one command group `ytb serve` and `ytb mcp` do not expose, because a route that took your cookies as a query parameter is a route that writes them into an access log.
