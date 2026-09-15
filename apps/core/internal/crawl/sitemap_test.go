package crawl

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sitemapSite declares an index in robots.txt pointing at a plain sitemap
// and a gzipped one; /hidden is only reachable through the sitemap.
func sitemapSite(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	page := func(w http.ResponseWriter, path, body string) {
		fmt.Fprintf(w, `<html><head><title>%s</title></head><body><article><p>%s Ce paragraphe est assez long pour que readability le garde comme contenu principal.</p></article></body></html>`, path, body)
	}
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := srv.URL
		switch r.URL.Path {
		case "/robots.txt":
			fmt.Fprintf(w, "User-agent: *\nDisallow: /private\nSitemap: %s/sitemap-index.xml\n", base)
		case "/sitemap-index.xml":
			fmt.Fprintf(w, `<?xml version="1.0"?><sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><sitemap><loc>%s/sitemap-a.xml</loc></sitemap><sitemap><loc>%s/sitemap-b.xml.gz</loc></sitemap><sitemap><loc>https://elsewhere.test/sitemap.xml</loc></sitemap></sitemapindex>`, base, base)
		case "/sitemap-a.xml":
			fmt.Fprintf(w, `<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><url><loc>%s/</loc><lastmod>2026-01-01</lastmod></url><url><loc>%s/a</loc></url><url><loc>%s/hidden</loc></url><url><loc>%s/private</loc></url></urlset>`, base, base, base, base)
		case "/sitemap-b.xml.gz":
			var buf bytes.Buffer
			gz := gzip.NewWriter(&buf)
			fmt.Fprintf(gz, `<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><url><loc>%s/b</loc></url><url><loc>%s/b/</loc></url><url><loc>https://elsewhere.test/x</loc></url></urlset>`, base, base)
			gz.Close()
			w.Write(buf.Bytes())
		case "/":
			page(w, "/", `<a href="/a">a</a>`)
		case "/a", "/b", "/hidden", "/private":
			page(w, r.URL.Path, "leaf")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestMapListsSitemapURLsFirstThenWalks(t *testing.T) {
	srv := sitemapSite(t)
	scope := Scope{Start: srv.URL + "/", MaxDepth: 1}
	if err := scope.Normalize(); err != nil {
		t.Fatal(err)
	}
	got := Map(context.Background(), scope, staticFetcher{}, 100, true)
	want := []string{"/", "/a", "/hidden", "/private", "/b"}
	var rel []string
	for _, u := range got {
		rel = append(rel, strings.TrimPrefix(u, srv.URL))
	}
	if strings.Join(rel, ",") != strings.Join(want, ",") {
		t.Errorf("mapped %v, want %v", rel, want)
	}
}

func TestMapSitemapStopsAtLimitWithoutReading(t *testing.T) {
	srv := sitemapSite(t)
	scope := Scope{Start: srv.URL + "/", MaxDepth: 1}
	if err := scope.Normalize(); err != nil {
		t.Fatal(err)
	}
	got := Map(context.Background(), scope, staticFetcher{}, 2, true)
	if len(got) != 2 {
		t.Errorf("limit not honoured: %v", got)
	}
}

func TestMapWithoutSitemapDoesNotSeeHiddenPages(t *testing.T) {
	srv := sitemapSite(t)
	scope := Scope{Start: srv.URL + "/", MaxDepth: 2}
	if err := scope.Normalize(); err != nil {
		t.Fatal(err)
	}
	for _, u := range Map(context.Background(), scope, staticFetcher{}, 100, false) {
		if strings.HasSuffix(u, "/hidden") {
			t.Fatal("/hidden is only in the sitemap")
		}
	}
}

func TestWalkSeededFromSitemapReadsHiddenPagesAndSkipsRobots(t *testing.T) {
	srv := sitemapSite(t)
	scope := Scope{Start: srv.URL + "/", MaxDepth: 0, Seed: SeedSitemap}
	if err := scope.Normalize(); err != nil {
		t.Fatal(err)
	}
	var urls []string
	Walk(context.Background(), scope, staticFetcher{}, func(r Result) {
		if r.Err == nil {
			urls = append(urls, strings.TrimPrefix(r.URL, srv.URL))
		}
	})
	joined := strings.Join(urls, ",")
	for _, want := range []string{"/hidden", "/b", "/a"} {
		if !strings.Contains(joined, want) {
			t.Errorf("seeded walk missed %s: %v", want, urls)
		}
	}
	if strings.Contains(joined, "/private") {
		t.Errorf("robots.txt ignored on a seeded page: %v", urls)
	}
	if strings.Count(joined, "/b") != 1 {
		t.Errorf("/b and /b/ should be one page: %v", urls)
	}
}

func TestScopeRejectsAnUnknownSeed(t *testing.T) {
	s := Scope{Start: "https://example.com/", Seed: "rss"}
	if err := s.Normalize(); err != ErrBadSeed {
		t.Errorf("err = %v, want ErrBadSeed", err)
	}
}
