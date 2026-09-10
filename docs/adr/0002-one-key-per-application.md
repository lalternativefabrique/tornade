# ADR 0002: one key per application

## Context

An application speaking through tornade used to hold two secrets: a signing
key its server signed browser-bound `/speak` URLs with, and an app key it
presented on `X-Tornade-Key` for its own calls. The admin panel minted both,
and every application had to be configured with both.

The two are held by the same party, side by side in the same configuration.
The app key is strictly the more powerful of the two: whoever holds it can
have any text read, which is everything a signature can buy and more. So the
split reduced no blast radius — a compromise of the application's server
exposes both, and a leak of the app key alone already grants everything the
signing key could. It was a habit inherited from the `SPEAK_SIGNING_KEYS` /
`SPEAK_APP_KEYS` era, not a security boundary.

## Decision

An application has one key. The panel mints one value, the application
configures one variable, and it is used for both purposes:

- presented as-is on `X-Tornade-Key`;
- the root of the MAC key its signatures use, derived inside the `signed`
  package as `HMAC-SHA256(key, "tornade/sign/v1")`.

The derivation is internal to `signed`: `Sign`, `NewSigner` and both
verifiers take the application's key, and nothing outside the package sees
the derived value. The registry stores one sealed secret per application
(plus the previous one during a rotation's grace), and `SPEAK_KEYS` replaces
the two environment variables with the same `issuer:key` shape.

## Why derive rather than reuse the bytes

The bytes presented on the wire are never themselves the MAC key. A signature
therefore reveals nothing about the credential it came from, and the two uses
stay cryptographically separated without asking an operator to manage that
separation. The cost is one HMAC per signature and per verifier construction,
which is nothing.

## Consequences

Registered applications keep their app key as their single key: the
migration carries it over, previous key included. Their signatures stop
verifying until they redeploy with the `signed` package that derives, and
that is what the grace period exists for. The `tornade-speak-keys` secret,
where mounted, must expose a `SPEAK_KEYS` entry instead of
`SPEAK_SIGNING_KEYS`. The `client.Config` field is `Key`, the header
constant `client.HeaderKey`, and the admin API answers `key` and `last4`
where it answered two of each.
