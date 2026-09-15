package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/lalternative/packages/go/search/fetch"

	"github.com/lalternativefabrique/vvaves/core/internal/challenge"
	"github.com/lalternativefabrique/vvaves/core/internal/crawl"
)

const (
	defaultMapLimit = 100
	maxMapLimit     = 1000
	defaultMapDepth = 3
	maxMapDeadline  = 50 * time.Second
	crawlPageLimit  = 20
	maxCrawlPages   = 100
)

// pageFetcher reads a page for a crawl exactly as /fetch would: the same
// address check, cache, renderer and challenge check, so a crawl can reach
// nothing a single fetch could not.
type pageFetcher struct {
	d Deps
}

func (f pageFetcher) Fetch(ctx context.Context, url string) (*fetch.Page, error) {
	if msg := validateURL(url, f.d.AllowPrivateFetch); msg != "" {
		return nil, errors.New(msg)
	}
	page, err := fetch.FetchWithFallback(ctx, url, f.d.Renderer, f.d.CrawlMaxRunes, f.d.Cache)
	if err != nil {
		return nil, err
	}
	if err := challenge.Check(page); err != nil {
		return nil, err
	}
	return page, nil
}

// Crawler is the crawl service over d's store and queue. main runs its Run
// loop; the handlers build the same value per request, it holds no state.
func Crawler(d Deps) *crawl.Service {
	return &crawl.Service{Store: d.CrawlStore, Queue: d.CrawlQueue, Fetcher: pageFetcher{d}, MaxRunes: d.CrawlMaxRunes}
}

type scopeRequest struct {
	URL          string   `json:"url"`
	MaxDepth     int      `json:"max_depth"`
	MaxPages     int      `json:"max_pages"`
	IncludePaths []string `json:"include_paths"`
	ExcludePaths []string `json:"exclude_paths"`
}

func (r scopeRequest) scope(d Deps) (crawl.Scope, string) {
	if msg := validateURL(r.URL, d.AllowPrivateFetch); msg != "" {
		return crawl.Scope{}, msg
	}
	s := crawl.Scope{Start: r.URL, MaxDepth: r.MaxDepth, MaxPages: r.MaxPages, IncludePaths: r.IncludePaths, ExcludePaths: r.ExcludePaths}
	if err := s.Normalize(); err != nil {
		return crawl.Scope{}, err.Error()
	}
	return s, ""
}

type mapRequest struct {
	scopeRequest
	Limit      int   `json:"limit"`
	DeadlineMS int64 `json:"deadline_ms"`
}

type mapResponse struct {
	URL   string   `json:"url"`
	Links []string `json:"links"`
}

func handleMap(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := d.guardService(r); err != nil {
			writeAuthError(w, err)
			return
		}
		var req mapRequest
		if err := decode(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if req.MaxDepth <= 0 {
			req.MaxDepth = defaultMapDepth
		}
		scope, msg := req.scope(d)
		if msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
		limit := req.Limit
		if limit <= 0 {
			limit = defaultMapLimit
		}
		if limit > maxMapLimit {
			limit = maxMapLimit
		}
		deadline := maxMapDeadline
		if asked := time.Duration(req.DeadlineMS) * time.Millisecond; asked > 0 && asked < deadline {
			deadline = asked
		}
		ctx, cancel := context.WithTimeout(r.Context(), deadline)
		defer cancel()

		links := crawl.Map(ctx, scope, pageFetcher{d}, limit)
		writeJSON(w, http.StatusOK, mapResponse{URL: scope.Start, Links: links})
	}
}

type crawlRequest struct {
	scopeRequest
}

func handleCrawl(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.CrawlStore == nil || d.CrawlQueue == nil {
			writeError(w, http.StatusServiceUnavailable, "crawling is not configured")
			return
		}
		if err := d.guardService(r); err != nil {
			writeAuthError(w, err)
			return
		}
		var req crawlRequest
		if err := decode(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		scope, msg := req.scope(d)
		if msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
		job, err := Crawler(d).Submit(r.Context(), scope)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not queue the crawl")
			return
		}
		writeJSON(w, http.StatusAccepted, job)
	}
}

type crawlStatusResponse struct {
	crawl.Job
	Results []crawl.StoredPage `json:"results"`
	Next    *int               `json:"next,omitempty"`
}

func handleCrawlStatus(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.CrawlStore == nil {
			writeError(w, http.StatusServiceUnavailable, "crawling is not configured")
			return
		}
		if err := d.guardService(r); err != nil {
			writeAuthError(w, err)
			return
		}
		job, err := d.CrawlStore.Get(r.Context(), r.PathValue("id"))
		if errors.Is(err, crawl.ErrNotFound) {
			writeError(w, http.StatusNotFound, "unknown crawl")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not read the crawl")
			return
		}
		q := r.URL.Query()
		offset, _ := strconv.Atoi(q.Get("offset"))
		limit, _ := strconv.Atoi(q.Get("limit"))
		if offset < 0 {
			offset = 0
		}
		if limit <= 0 {
			limit = crawlPageLimit
		}
		if limit > maxCrawlPages {
			limit = maxCrawlPages
		}
		format := q.Get("format")
		if format != "" && format != formatText && format != formatMarkdown {
			writeError(w, http.StatusBadRequest, "format must be text or markdown")
			return
		}
		pages, err := d.CrawlStore.Pages(r.Context(), job.ID, offset, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not read the crawl")
			return
		}
		for i := range pages {
			switch format {
			case formatText:
				pages[i].Markdown = ""
			case formatMarkdown:
				pages[i].Text = ""
			}
		}
		out := crawlStatusResponse{Job: job, Results: pages}
		if next := offset + len(pages); len(pages) == limit && next < job.Pages+job.Failed {
			out.Next = &next
		}
		writeJSON(w, http.StatusOK, out)
	}
}
