# tornade

Renders a URL's JavaScript and returns the resulting HTML, for pages a plain
HTTP fetch sees as an empty shell.

```
POST /render
{"url": "https://example.com", "timeout_ms": 8000}

200 {"html": "<html>...</html>", "final_url": "https://example.com/"}
502 {"error": "..."}
```

`timeout_ms` is optional (defaults to 8000ms) and clamped to at most 20000ms
regardless of what is asked — an unbounded render is a request that never
finishes, and on a shared browser process that starves every other in-flight
render behind it.

`GET /healthz` returns `{"ok": true}` once the process is up; it does not
wait on the browser (Chromium launches lazily, on the first `/render` call).

## Why this is a separate service

It exists to back `fetch.Renderer` in
[`github.com/lalternative/packages/go/search`](https://github.com/lalternativefabrique/packages/tree/main/go/search) —
that library deliberately keeps this out of itself: rendering JavaScript
means driving a long-lived Chromium process (300-500MB+ per active render,
its own crash/restart lifecycle), which is a deployment concern, not
something every consumer of a Go library should have to bundle into its own
image. One shared browser pool, many callers.

```go
import "github.com/lalternative/packages/go/search/rendersvc"

renderer := rendersvc.NewClient("http://tornade:8080", nil)
page, err := fetch.FetchWithFallback(ctx, url, renderer, 6000, cache)
```

## Running it

```bash
npm install
npx playwright install chromium   # or --with-deps on a fresh Debian/Ubuntu host
npm start
```

Or via Docker, which needs no separate browser install — the base image
ships Chromium and every system library it needs:

```bash
docker build -t tornade .
docker run -p 8080:8080 tornade
```

## Design notes

One Chromium process is shared across every request; a browser *context*
(Playwright's lightweight incognito-style session) is opened and closed per
render instead. Launching a fresh browser per request would pay Chromium's
~1-2s startup cost on every call — the context is the unit of isolation
that's actually cheap.

Rendering waits for `networkidle`, not `load`: a JS-rendered page's content
typically arrives via a fetch/XHR that fires after the `load` event, so
waiting for `load` alone would return the same empty shell a plain HTTP fetch
already sees — the entire reason this service exists.

Runs as a non-root user in its container: this service exists to execute
arbitrary third-party JavaScript, and a compromised render should not run as
root.
