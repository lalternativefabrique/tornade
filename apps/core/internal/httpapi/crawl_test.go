package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lalternativefabrique/vvaves/core/internal/crawl"
	"github.com/lalternativefabrique/vvaves/core/internal/httpapi"
)

func serveSite(t *testing.T) *httptest.Server {
	t.Helper()
	pages := map[string]string{
		"/":    `<a href="/a">a</a> <a href="/b">b</a>`,
		"/a":   `<a href="/a/1">a1</a>`,
		"/a/1": `leaf`,
		"/b":   `<table><tr><th>k</th></tr><tr><td>v</td></tr></table>`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := pages[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `<html><head><title>%s</title></head><body><article><p>%s Ce paragraphe est assez long pour que readability le garde comme contenu principal de la page.</p></article></body></html>`, r.URL.Path, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func crawlDeps(t *testing.T) (httpapi.Deps, context.CancelFunc) {
	t.Helper()
	d := baseDeps()
	d.Unguarded = true
	d.AllowPrivateFetch = true
	d.CrawlStore = crawl.NewMemoryStore()
	d.CrawlQueue = crawl.NewMemoryQueue()
	d.CrawlMaxRunes = 6000
	ctx, cancel := context.WithCancel(context.Background())
	go httpapi.Crawler(d).Run(ctx)
	return d, cancel
}

func TestMapListsTheSiteURLs(t *testing.T) {
	site := serveSite(t)
	d, cancel := crawlDeps(t)
	defer cancel()

	rec := post(t, httpapi.New(d), "/map", `{"url":"`+site.URL+`/","limit":10}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body %s", rec.Code, rec.Body)
	}
	var out struct {
		Links []string `json:"links"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Links) != 4 {
		t.Errorf("links = %v, want the four pages", out.Links)
	}
}

func TestMapReadsTheSitemapUnlessToldNotTo(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sitemap.xml":
			fmt.Fprintf(w, `<urlset><url><loc>%s/only-in-sitemap</loc></url></urlset>`, srv.URL)
		case "/robots.txt":
			http.NotFound(w, r)
		default:
			fmt.Fprint(w, `<html><head><title>x</title></head><body><article><p>Ce paragraphe est assez long pour que readability le garde comme contenu principal de la page.</p></article></body></html>`)
		}
	}))
	t.Cleanup(srv.Close)
	d, cancel := crawlDeps(t)
	defer cancel()
	h := httpapi.New(d)

	var out struct {
		Links []string `json:"links"`
	}
	rec := post(t, h, "/map", `{"url":"`+srv.URL+`/"}`)
	json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Links) != 2 || !strings.HasSuffix(out.Links[1], "/only-in-sitemap") {
		t.Errorf("default map should read the sitemap: %v", out.Links)
	}
	out.Links = nil
	rec = post(t, h, "/map", `{"url":"`+srv.URL+`/","sitemap":false}`)
	json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Links) != 1 {
		t.Errorf("sitemap:false should walk links only: %v", out.Links)
	}
}

func TestCrawlRefusesAnUnknownSeed(t *testing.T) {
	d, cancel := crawlDeps(t)
	defer cancel()
	rec := post(t, httpapi.New(d), "/crawl", `{"url":"https://example.com/","seed":"rss"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestCrawlQueuesThenServesPages(t *testing.T) {
	site := serveSite(t)
	d, cancel := crawlDeps(t)
	defer cancel()
	h := httpapi.New(d)

	rec := post(t, h, "/crawl", `{"url":"`+site.URL+`/","max_depth":2,"exclude_paths":["/a/1"]}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d; body %s", rec.Code, rec.Body)
	}
	var job crawl.Job
	json.Unmarshal(rec.Body.Bytes(), &job)
	if job.ID == "" || job.Status != crawl.StatusQueued {
		t.Fatalf("job = %+v", job)
	}

	var status struct {
		crawl.Job
		Results []crawl.StoredPage `json:"results"`
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && status.Status != crawl.StatusDone {
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/crawl/"+job.ID+"?format=markdown", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; body %s", rec.Code, rec.Body)
		}
		json.Unmarshal(rec.Body.Bytes(), &status)
		time.Sleep(20 * time.Millisecond)
	}
	if status.Status != crawl.StatusDone || status.Pages != 3 {
		t.Fatalf("job never finished with 3 pages: %+v", status.Job)
	}
	var sawTable bool
	for _, p := range status.Results {
		if p.Text != "" {
			t.Errorf("format=markdown should omit text: %+v", p)
		}
		if strings.Contains(p.Markdown, "| k |") {
			sawTable = true
		}
	}
	if !sawTable {
		t.Errorf("markdown of /b should carry its table: %+v", status.Results)
	}
}

func TestCrawlStatusPaginates(t *testing.T) {
	site := serveSite(t)
	d, cancel := crawlDeps(t)
	defer cancel()
	h := httpapi.New(d)

	rec := post(t, h, "/crawl", `{"url":"`+site.URL+`/","max_depth":2}`)
	var job crawl.Job
	json.Unmarshal(rec.Body.Bytes(), &job)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if j, _ := d.CrawlStore.Get(context.Background(), job.ID); j.Status == crawl.StatusDone {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/crawl/"+job.ID+"?limit=2", nil))
	var out struct {
		Results []crawl.StoredPage `json:"results"`
		Next    *int               `json:"next"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Results) != 2 || out.Next == nil || *out.Next != 2 {
		t.Errorf("first page = %d results, next %v", len(out.Results), out.Next)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/crawl/"+job.ID+"?limit=2&offset=2", nil))
	out.Results, out.Next = nil, nil
	json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Results) != 2 || out.Next != nil {
		t.Errorf("last page = %d results, next %v", len(out.Results), out.Next)
	}
}

func TestCrawlRefusesAnInternalAddressAndAnUnknownJob(t *testing.T) {
	d, cancel := crawlDeps(t)
	defer cancel()
	d.AllowPrivateFetch = false
	h := httpapi.New(d)

	rec := post(t, h, "/crawl", `{"url":"http://169.254.169.254/latest/meta-data/"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	rec = post(t, h, "/map", `{"url":"http://10.0.0.1/"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("map status = %d, want 400", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/crawl/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown job status = %d, want 404", rec.Code)
	}
}

func TestCrawlReportsItselfUnconfigured(t *testing.T) {
	d := baseDeps()
	d.Unguarded = true
	rec := post(t, httpapi.New(d), "/crawl", `{"url":"https://example.com/"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}
