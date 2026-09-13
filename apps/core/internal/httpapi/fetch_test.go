package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lalternative/packages/go/search/fetch"

	"github.com/lalternativefabrique/vvaves/core/internal/challenge"
	"github.com/lalternativefabrique/vvaves/core/internal/httpapi"
)

const cloudflareChallengeHTML = `<!DOCTYPE html>
<html lang="en-US"><head><title>Just a moment...</title></head>
<body class="no-js">
<div class="main-wrapper"><h1>www.example.com</h1>
<p>Verifying you are human. This may take a few seconds.</p>
<p>www.example.com needs to review the security of your connection before proceeding.</p>
<p>Ray ID: 8d1f2c3a4b5e6f70</p>
<p>Performance &amp; security by Cloudflare</p>
</div>
</body></html>`

const articleHTML = `<!doctype html>
<html><head><title>A real article</title></head>
<body>
<article>
<h1>A real article</h1>
<p>This is the first paragraph of a normal article with enough text for
readability to consider it the main content of the page, rather than
boilerplate navigation or a footer.</p>
<p>A second paragraph adds more substance so the extracted text comfortably
clears any minimum-length heuristic a caller might apply downstream.</p>
</article>
</body></html>`

func serveHTML(t *testing.T, html string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(html))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchRefusesAChallengeServedAsThePage(t *testing.T) {
	origin := serveHTML(t, cloudflareChallengeHTML)

	d := baseDeps()
	d.Unguarded = true
	// The fixture is served from 127.0.0.1, which the fetch guard refuses.
	d.AllowPrivateFetch = true
	d.Renderer = &stubRenderer{html: cloudflareChallengeHTML}
	d.Cache = challenge.GuardCache(fetch.NewMemoryCache(time.Minute))
	h := httpapi.New(d)

	rec := post(t, h, "/fetch", `{"url":"`+origin.URL+`"}`)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body %s", rec.Code, rec.Body)
	}
	var body map[string]string
	json.Unmarshal(rec.Body.Bytes(), &body)
	if !strings.Contains(body["error"], "cloudflare challenge") {
		t.Errorf("error = %q, want it to name the cloudflare challenge", body["error"])
	}

	if _, ok := d.Cache.Get(origin.URL); ok {
		t.Error("the challenge page was cached")
	}
	if _, ok := d.Cache.Get(origin.URL + "#rendered"); ok {
		t.Error("the rendered challenge page was cached")
	}
}

func TestFetchServesAnOrdinaryArticle(t *testing.T) {
	origin := serveHTML(t, articleHTML)

	d := baseDeps()
	d.Unguarded = true
	// The fixture is served from 127.0.0.1, which the fetch guard refuses.
	d.AllowPrivateFetch = true
	rec := post(t, httpapi.New(d), "/fetch", `{"url":"`+origin.URL+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body)
	}
}

// /fetch and /render take a URL from their caller and report what came back,
// so without an address check they read the internal network one request at
// a time. The reason is never named to the caller: answering "is this
// address reachable from inside" for any address asked about is the scan the
// check exists to prevent.
func TestFetchRefusesAnInternalAddress(t *testing.T) {
	d := baseDeps()
	d.Unguarded = true
	h := httpapi.New(d)

	for name, target := range map[string]string{
		"loopback":       "http://127.0.0.1:8080/",
		"private range":  "http://10.0.0.1/",
		"cloud metadata": "http://169.254.169.254/latest/meta-data/",
	} {
		rec := post(t, h, "/fetch", `{"url":"`+target+`"}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", name, rec.Code)
			continue
		}
		var body map[string]string
		json.Unmarshal(rec.Body.Bytes(), &body)
		if strings.Contains(body["error"], "resolve") || strings.Contains(body["error"], "address") {
			t.Errorf("%s: error %q tells the caller why, which is the scan itself", name, body["error"])
		}
	}
}

func TestRenderRefusesAnInternalAddress(t *testing.T) {
	d := baseDeps()
	d.Unguarded = true
	d.Renderer = &stubRenderer{html: "<html></html>", finalURL: "https://example.com/"}
	rec := post(t, httpapi.New(d), "/render", `{"url":"http://169.254.169.254/latest/meta-data/"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// Allowing a private address says nothing about allowing file://, which
// reads the local disk rather than the network.
func TestAllowPrivateFetchStillRefusesANonHTTPScheme(t *testing.T) {
	d := baseDeps()
	d.Unguarded = true
	d.AllowPrivateFetch = true
	d.Renderer = &stubRenderer{}
	if rec := post(t, httpapi.New(d), "/render", `{"url":"file:///etc/passwd"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
