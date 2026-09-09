package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lalternative/packages/go/search/fetch"

	"github.com/lalternativefabrique/tornade/internal/challenge"
	"github.com/lalternativefabrique/tornade/internal/httpapi"
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

	rec := post(t, httpapi.New(baseDeps()), "/fetch", `{"url":"`+origin.URL+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body)
	}
}
