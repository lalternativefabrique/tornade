package crawl

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/lalternative/packages/go/search/fetch"
)

// site serves a small tree: / links to /a, /b and /docs/x; /a links to /a/1
// and back to /; /docs/x links to /docs/y; /private is forbidden by robots.
func site(t *testing.T) *httptest.Server {
	t.Helper()
	pages := map[string]string{
		"/":         `<a href="/a">a</a> <a href="/b">b</a> <a href="/docs/x">x</a> <a href="/private">p</a> <a href="/file.pdf">pdf</a> <a href="https://elsewhere.test/">out</a>`,
		"/a":        `<a href="/a/1">a1</a> <a href="/">home</a>`,
		"/a/1":      `<a href="/a/1/deep">deeper</a>`,
		"/a/1/deep": `leaf`,
		"/b":        `leaf b`,
		"/docs/x":   `<a href="/docs/y">y</a>`,
		"/docs/y":   `leaf y`,
		"/private":  `secret`,
	}
	var mu sync.Mutex
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			fmt.Fprint(w, "User-agent: *\nDisallow: /private\n")
			return
		}
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
		body, ok := pages[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `<html><head><title>%s</title></head><body><article><p>%s</p></article></body></html>`, r.URL.Path, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

type staticFetcher struct{}

func (staticFetcher) Fetch(ctx context.Context, u string) (*fetch.Page, error) {
	return fetch.FetchStatic(ctx, u, 6000, nil)
}

func walk(t *testing.T, scope Scope) []Result {
	t.Helper()
	if err := scope.Normalize(); err != nil {
		t.Fatal(err)
	}
	var out []Result
	Walk(context.Background(), scope, staticFetcher{}, func(r Result) { out = append(out, r) })
	return out
}

func paths(rs []Result, base string) []string {
	var out []string
	for _, r := range rs {
		out = append(out, strings.TrimPrefix(r.URL, base))
	}
	sort.Strings(out)
	return out
}

func TestWalkFollowsInternalLinksToMaxDepth(t *testing.T) {
	srv := site(t)
	got := paths(walk(t, Scope{Start: srv.URL + "/", MaxDepth: 2}), srv.URL)
	want := []string{"/", "/a", "/a/1", "/b", "/docs/x", "/docs/y"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("walked %v, want %v", got, want)
	}
}

func TestWalkStopsAtMaxPages(t *testing.T) {
	srv := site(t)
	got := walk(t, Scope{Start: srv.URL + "/", MaxDepth: 5, MaxPages: 3})
	if len(got) != 3 {
		t.Errorf("read %d pages, want 3", len(got))
	}
}

func TestWalkHonoursIncludeAndExcludePaths(t *testing.T) {
	srv := site(t)
	got := paths(walk(t, Scope{Start: srv.URL + "/", MaxDepth: 3, IncludePaths: []string{"/a", "/docs"}, ExcludePaths: []string{"/a/1"}}), srv.URL)
	want := []string{"/", "/a", "/docs/x", "/docs/y"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("walked %v, want %v", got, want)
	}
}

func TestWalkSkipsWhatRobotsForbid(t *testing.T) {
	srv := site(t)
	for _, r := range walk(t, Scope{Start: srv.URL + "/", MaxDepth: 1}) {
		if strings.HasSuffix(r.URL, "/private") {
			t.Fatal("read /private despite robots.txt")
		}
	}
}

func TestWalkReportsAPageItCouldNotRead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			fmt.Fprint(w, `<html><body><article><p><a href="/missing">m</a> some text for the page</p></article></body></html>`)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	var failed []Result
	for _, r := range walk(t, Scope{Start: srv.URL + "/", MaxDepth: 1}) {
		if r.Err != nil {
			failed = append(failed, r)
		}
	}
	if len(failed) != 1 || !strings.HasSuffix(failed[0].URL, "/missing") {
		t.Errorf("want one failed result for /missing, got %+v", failed)
	}
}

func TestScopeNormalizeClampsAndRejects(t *testing.T) {
	s := Scope{Start: "https://example.com/x#frag", MaxDepth: 99, MaxPages: 99999}
	if err := s.Normalize(); err != nil {
		t.Fatal(err)
	}
	if s.MaxDepth != MaxMaxDepth || s.MaxPages != MaxMaxPages || s.Start != "https://example.com/x" {
		t.Errorf("normalized to %+v", s)
	}
	bad := Scope{Start: "ftp://example.com"}
	if err := bad.Normalize(); err == nil {
		t.Error("ftp start should be refused")
	}
}
