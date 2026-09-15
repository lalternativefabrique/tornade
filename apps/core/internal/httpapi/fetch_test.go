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

const tableArticleHTML = `<html><head><title>Exemple — offres</title></head><body><article>
<h2>Tarifs</h2>
<p>Nos offres sont pensées pour accompagner chaque équipe, de la première expérimentation
au déploiement en production sur des volumes importants, avec un support adapté.</p>
<table>
<tr><th>Plan</th><th>Prix</th></tr>
<tr><td>Pro</td><td>49 €</td></tr>
</table>
<p>Voir la <a href="/pricing">grille complète</a> pour le détail des options et des
engagements de disponibilité qui accompagnent chacun des plans proposés ici.</p>
</article></body></html>`

func fetchJSON(t *testing.T, body string) (int, map[string]any) {
	t.Helper()
	d := baseDeps()
	d.Unguarded = true
	d.AllowPrivateFetch = true
	rec := post(t, httpapi.New(d), "/fetch", body)
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	return rec.Code, out
}

func TestFetchReturnsBothRenderingsByDefault(t *testing.T) {
	origin := serveHTML(t, tableArticleHTML)

	code, out := fetchJSON(t, `{"url":"`+origin.URL+`"}`)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %v", code, out)
	}
	text, _ := out["text"].(string)
	md, _ := out["markdown"].(string)
	if strings.Contains(text, "|") || !strings.Contains(text, "49 €") {
		t.Errorf("text should be flat, got %q", text)
	}
	if !strings.Contains(md, "| Pro") || !strings.Contains(md, "[grille complète](") {
		t.Errorf("markdown should keep the table and the link, got %q", md)
	}
}

func TestFetchFormatSelectsOneRendering(t *testing.T) {
	origin := serveHTML(t, tableArticleHTML)

	_, out := fetchJSON(t, `{"url":"`+origin.URL+`","format":"markdown"}`)
	if _, has := out["text"]; has {
		t.Error("format markdown should omit text")
	}
	if _, has := out["markdown"]; !has {
		t.Error("format markdown should carry markdown")
	}

	_, out = fetchJSON(t, `{"url":"`+origin.URL+`","format":"text"}`)
	if _, has := out["markdown"]; has {
		t.Error("format text should omit markdown")
	}
	if _, has := out["text"]; !has {
		t.Error("format text should carry text")
	}
}

func TestFetchPaginatesTheMarkdownWhenAsked(t *testing.T) {
	origin := serveHTML(t, tableArticleHTML)

	_, out := fetchJSON(t, `{"url":"`+origin.URL+`","format":"markdown","paginate":80}`)
	pages, _ := out["pages"].([]any)
	if len(pages) < 2 {
		t.Fatalf("expected several pages, got %v", out)
	}
	joined := ""
	for _, p := range pages {
		joined += p.(string)
	}
	if !strings.Contains(joined, "| Pro") {
		t.Errorf("pages should be cut from the markdown, got %q", joined)
	}
}

func TestFetchRefusesAnUnknownFormat(t *testing.T) {
	origin := serveHTML(t, tableArticleHTML)

	code, _ := fetchJSON(t, `{"url":"`+origin.URL+`","format":"html"}`)
	if code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", code)
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
