# tornade

The HTTP facade over this platform's search, page extraction, JavaScript
rendering and speech backends. One service, one image: SearXNG, Brave and
Piper sit behind it, and callers only ever address tornade.

```
POST /search   {"q": "gramsci"}                     -> ranked results
POST /fetch    {"url": "https://…"}                 -> the page's main text
POST /render   {"url": "https://…"}                 -> rendered HTML
POST /speak    {"text": "…"}                        -> audio
POST /speak/prime       {"text": "…", "id": "…"}    -> 202, opening read ahead
POST /speak/pregenerate {"text": "…", "id": "…"}    -> 202, whole reading cached
POST /speak/exists      {"text": "…", "id": "…"}    -> {"ready": true}
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
{"text": "…", "scope": "chat", "id": "msg-42", "stream": false}
```

Reads text through Piper over the OpenAI `/v1/audio/speech` protocol.

`scope` and `id` name where the reading is kept. Both are optional: with an
`id` a caller can have a reading made before anyone asks for it, since reading
ahead means naming ahead of time what will be listened to. Without one the
reading is still cached under the hash of its own text, so a second listen of
the same words finds it with nothing to opt into.

The key holds a hash of the text, so an edited text misses and is read again
rather than being served the audio of words that are no longer there. Nothing
has to be invalidated: a draft that changes on every keystroke simply writes
under a new key, and a bucket lifecycle rule collects the ones nobody came
back to.

`stream: false` returns the finished audio with a `Content-Length`, which a
plain `<audio src>` needs — without one, browsers infer the duration from the
first frame header and stop at the first seam, playing only the opening
seconds of a long text with nothing reported as wrong. A reading already in
the store is always served this way, ranges included, so a second listen
starts at once and can be seeked.

`stream: true` emits each piece as it is ready, so listening starts on the
first one instead of after the last. Pieces arrive length-prefixed — a
big-endian `uint32` byte count then that many bytes — under
`application/x-lalter-audio-frames`, because concatenated mp3 frames carry no
boundary a player could find on its own. Once a piece has been sent the `200`
is committed, so a later failure cuts the response short instead of reporting
an error — which is why it is not the default.

### `POST /speak/prime`

```json
{"text": "…", "scope": "chat", "id": "msg-42"}
```

Reads the **opening** of a text — one piece, `AUDIO_OPENING_CHARS` — and
stores it, so the first listen starts on audio that already exists while the
rest is read behind it. Answers `202` without waiting: nobody is listening
yet, and a failure only means the first listener waits as they used to.

The cut is the one the reading itself would make, so the two halves meet
exactly where a seam would have fallen anyway — no word is read twice, none is
skipped.

This is what to use for a text that will be heard once or not at all — a reply
to a prompt. It buys the seconds that matter for the price of one piece,
instead of paying for a whole reading on the chance that someone might listen.
`id` is required: a reading nobody can name again cannot be found later.

### `POST /speak/pregenerate`

Reads a text **in full** and caches it, so every listen after it is served
from the store. For a text that will be heard more than once — a published
page — where paying the whole synthesis up front is amortised, and where the
first visitor should not be the one who triggers it.

Piper synthesizes one utterance at a time, so a reading nobody asked for
occupies the voice while someone who pressed play waits behind it. Reach for
`/speak/prime` unless the reading is genuinely expected to be heard more than
once.

### `POST /speak/exists`

Reports whether a reading is already stored, without reading its bytes — for a
caller deciding whether to offer a play button, which would otherwise download
the whole file to answer yes or no.

## Configuration

| | |
|---|---|
| `SEARXNG_URL` | required by `/search`, else `503` |
| `BRAVE_API_KEY` | optional; enables the general-category fallback |
| `PIPER_URL` | required by `/speak`, else `503` |
| `TTS_MODEL`, `TTS_VOICE`, `TTS_FORMAT` | voice selection; format must be frame-based (`mp3`, `opus`, `aac`, `flac`) |
| `TTS_MAX_CHARS` | text per request, default 120 |
| `AUDIO_OPENING_CHARS` | how much of a text counts as its opening, default 800 |
| `S3_ENDPOINT`, `S3_BUCKET`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_REGION` | where readings are kept; unset disables the cache, half-set is fatal |
| `TTS_CONCURRENCY` | pieces read at once, default 1 |
| `SEARCH_DEADLINE_MS` | cap on a merged search, default 4000 |
| `FETCH_CACHE_TTL_MS` | default 15 minutes |
| `RENDER_MAX_TIMEOUT_MS` | hard ceiling, default 20000 |
| `CHROMIUM_PATH` | browser binary; set in the image |
| `LISTEN_ADDR` | default `:8080` |

An unconfigured backend disables its endpoint rather than degrading silently.

`AUDIO_OPENING_CHARS` is not `TTS_MAX_CHARS`, though both count characters.
The latter is how small a reading is cut for Piper to work on; the former is
how much of a text counts as its opening. They must not be conflated: the
primer and the reader each split the text themselves, and two different sizes
have the halves meet somewhere other than the same cut.

With no `S3_*` set, tornade still reads text aloud — every reading is simply
paid for again, which is what it did before there was a bucket. A half-set
configuration is fatal instead: someone meant to have a cache, and starting
without one would hide that behind a bill nobody notices until it arrives.

`TTS_MAX_CHARS` and `TTS_CONCURRENCY` are the latency levers, and a
self-hosted Piper wants the opposite of a hosted endpoint. It serializes
synthesis — one utterance at a time per process — so reading pieces
concurrently only queues them, and every piece in the queue delays the one the
listener is actually waiting for. Measured against it on a 1200-character
text, concurrency 1 returns first audio in 2.2s against 3.9s at 4, for 6% more
total time; 120 characters a piece beats both 80 (more per-request overhead,
5.7s) and 200 (a longer first piece, 3.4s). Raise the concurrency only for a
backend that synthesizes in parallel.

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

## Deployment

It runs on the OVH cluster in its own `tornade-prod` namespace, reaching
searxng and piper across the `ai` namespace by their cluster DNS names.
Manifests live in `infra/k8s/base`; ArgoCD syncs them from `main` through the
Application in `infra/k8s/argocd`, which the cluster's `tornade-root`
app-of-apps discovers (declared in kube-infra's `app-v1` stack).

The `ai` namespace is declared here too, in `infra/k8s/base-ai` behind the
`production-ai` overlay. It holds searxng, piper and the redis searxng caches
into — the backends this service exists to put one HTTP contract in front of.
They used to be declared in synthiz, which reaches them the same way tornade
does; the namespace kept its name through the move, so every caller's DNS
still resolves.

It stays a namespace of its own rather than folding into `tornade-prod`: its
ResourceQuota covers workloads shared by more than one product, and that
budget is easier to reason about next to them than mixed into a single
service's.

Rolling a version means publishing an image: argocd-image-updater watches the
registry for immutable date-sha tags and writes the new one back to `main`
itself. The tag committed in `infra/k8s/base/kustomization.yaml` is only the
version a fresh cluster starts from. Nothing reads `latest`.

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

**The opening, not the whole reading.** A text of a few thousand characters
takes Piper tens of seconds to read, so a listener who presses play waits out
a spinner. Streaming cuts that to the first piece — two seconds instead of
thirty — and reading that first piece before anyone asks removes even those:
the start comes out of the store at once, and the rest is read while it plays.
The listener hears one recording.

Only the opening, because most readings never happen. Reading a whole text
ahead of time pays for all of them on the chance that someone listens to one,
and it occupies a voice that synthesizes one utterance at a time — a burst of
texts nobody opened would put someone who did press play at the back of a
queue. The opening is a single request, and it buys the seconds that actually
show.

Reading everything ahead is still the right answer where a text will be heard
more than once, or where the first listener must not be the one who pays:
that is `/speak/pregenerate`, and the choice between the two is a question
about how many listens are expected, not about which caller is asking.

**Non-root, without Chromium's inner sandbox.** That sandbox needs
unprivileged user namespaces, which a container's default seccomp profile
blocks; Playwright passed `--no-sandbox` by default, so this is the isolation
the service has always run with. The container is the boundary around the
third-party JavaScript this service exists to execute — hence the non-root
user, which is what actually contains a compromised render.
