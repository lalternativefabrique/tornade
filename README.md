# tornade

The HTTP facade over this platform's search, page extraction, JavaScript
rendering and speech backends. One service, one image: SearXNG, Brave and
Piper sit behind it, and callers only ever address tornade.

```
POST /search   {"q": "gramsci"}                     -> ranked results
POST /fetch    {"url": "https://…"}                 -> the page's main text
POST /render   {"url": "https://…"}                 -> rendered HTML
POST /speak    {"text": "…"}                        -> audio
GET  /healthz                                       -> {"ok": true}
```

It is the HTTP layer over
[`packages/go/search`](https://github.com/lalternativefabrique/packages/tree/main/go/search)
and [`packages/go/tts`](https://github.com/lalternativefabrique/packages/tree/main/go/tts),
plus the one piece that cannot live in a library: a browser.

## Endpoints

### `POST /search`

```json
{"q": "gramsci", "categories": ["general", "academic"],
 "limit": 10, "language": "fr", "deadline_ms": 4000}
```

One category is a plain query. Several are fused by reciprocal rank rather
than raw score — SearXNG's web category peaks around 4.0 where its academic
engines cap at 1.0, so sorting a concatenation buries whichever list scores
lowest. `deadline_ms` bounds the wait on the slowest category; one that misses
it is dropped and `partial` is set, rather than holding the response hostage.

With `BRAVE_API_KEY` set, Brave backs up the general category — it runs its
own index rather than reselling Google or Bing, which covers the gap a
self-hosted SearXNG actually has: an upstream engine rate-limited or blocked.

### `POST /fetch`

```json
{"url": "https://…", "max_runes": 6000, "render": true, "paginate": 4000}
```

Extracts the page's main text with readability. A page whose content is
rendered client-side yields almost nothing statically, so the fetch falls back
to rendering it in-process — no HTTP round trip, the renderer is a function
call. Pass `"render": false` to skip that. `paginate` returns fixed-size pages
instead of a truncation, so a long article can be walked rather than lost.

Pages are cached by URL, holding the full untruncated text, so a later call
with a different `max_runes` still sees the whole page.

### `POST /render`

```json
{"url": "https://…", "timeout_ms": 8000}
```
```json
{"html": "<html>…</html>", "final_url": "https://…/"}
```

`timeout_ms` defaults to 8s and is clamped to 20s regardless of what is asked:
an unbounded render is a request that never finishes, and on a shared browser
it starves everything queued behind it. A page that refuses the connection or
never settles answers `502` — routine on the open web, and callers treat it as
"fall back to the static result" rather than as fatal.

### `POST /speak`

```json
{"text": "…", "stream": false}
```

Reads text through Piper over the OpenAI `/v1/audio/speech` protocol.

`stream: false` returns the finished audio with a `Content-Length`, which a
plain `<audio src>` needs — without one, browsers infer the duration from the
first frame header and stop at the first seam, playing only the opening
seconds of a long text with nothing reported as wrong.

`stream: true` emits each piece as it is ready, so listening starts on the
first one. It needs `MediaSource` on the receiving end. Once a piece has been
sent the `200` is committed, so a later failure cuts the response short
instead of reporting an error — which is why it is not the default.

## Configuration

| | |
|---|---|
| `SEARXNG_URL` | required by `/search`, else `503` |
| `BRAVE_API_KEY` | optional; enables the general-category fallback |
| `PIPER_URL` | required by `/speak`, else `503` |
| `TTS_MODEL`, `TTS_VOICE`, `TTS_FORMAT` | voice selection; format must be frame-based (`mp3`, `opus`, `aac`, `flac`) |
| `TTS_MAX_CHARS` | text per request, default 1000 |
| `TTS_CONCURRENCY` | pieces read at once, default 4 |
| `SEARCH_DEADLINE_MS` | cap on a merged search, default 4000 |
| `FETCH_CACHE_TTL_MS` | default 15 minutes |
| `RENDER_MAX_TIMEOUT_MS` | hard ceiling, default 20000 |
| `CHROMIUM_PATH` | browser binary; set in the image |
| `LISTEN_ADDR` | default `:8080` |

An unconfigured backend disables its endpoint rather than degrading silently.

`TTS_MAX_CHARS` is the latency lever. Piper has no per-request limit and is
slower per character than a hosted endpoint, so the hosted default would read
a whole page as one long utterance while most workers sit idle.

## Running it

```bash
go test ./...
SEARXNG_URL=… PIPER_URL=… go run ./cmd/tornade
```

`/render` and `/fetch`'s fallback need a Chromium on `PATH`, or `CHROMIUM_PATH`
pointing at one. The image carries its own:

```bash
docker build -t tornade .
docker run -p 8080:8080 -e SEARXNG_URL=… -e PIPER_URL=… tornade
```

## Design notes

**One Chromium, many tabs.** The browser process is shared and started
lazily on the first render; a tab is opened and closed per request. Launching
a browser per request would pay Chromium's 1-2s startup every time — the tab
is the unit of isolation that is actually cheap. Each tab inherits its
request's deadline, so an abandoned render tears down its own tab without
touching the browser others are using.

**Waiting for the network, not for `load`.** A JS-rendered page's content
arrives through a fetch/XHR fired after the load event, so waiting on `load`
returns the same empty shell a plain HTTP fetch already sees — the entire
reason rendering exists here. Tornade watches CDP network events and settles
once nothing has been in flight for 500ms, then keeps watching a further two
seconds: a script that fires its request on a timer leaves the network idle in
the meantime, and calling that lull "settled" returns the shell.

**Non-root, without Chromium's inner sandbox.** That sandbox needs
unprivileged user namespaces, which a container's default seccomp profile
blocks; Playwright passed `--no-sandbox` by default, so this is the isolation
the service has always run with. The container is the boundary around the
third-party JavaScript this service exists to execute — hence the non-root
user, which is what actually contains a compromised render.
