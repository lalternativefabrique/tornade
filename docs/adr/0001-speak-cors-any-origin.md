# ADR 0001: /speak answers any origin, without credentials

## Context

A listener's browser fetches its audio from tornade directly, on a URL the
application signed, rather than through that application: a reading is tens
of seconds of bytes, and relaying it would have the application stream media
for the whole of it. The page and tornade live on different origins, so the
browser asks tornade's permission first, and tornade has to say which origins
it serves.

## Decision

`/speak` answers `Access-Control-Allow-Origin: *`, never with credentials.
Only `/speak` answers a preflight; `/speak/prime`, `/speak/pregenerate` and
`/speak/exists` answer none, which keeps a browser off them the way the guard
keeps a signature off them.

## Why not an origin per application

The origin would protect nothing. What authorises a `/speak` call is the
signature in the URL, minted by the application for one text, for a while.
A cookie plays no part, so there is nothing an origin check would keep a
malicious page from doing that a `curl` replaying the URL cannot already do.
An `origins` field on each registered application would be one more thing to
administer, one more way for a deployment to fail, and would close no attack.
The guard is the signature; CORS only lets the browser through to it.

## Consequences

The response exposes `Accept-Ranges`, `Content-Length`, `Content-Range` and
`X-Tts-Cache`, or the player could neither read a duration nor seek on a
cached reading. Applications list `https://vvaves.dev` in their `connect-src`;
`signed.Signer.PublicOrigin` exists to hand them that value.
