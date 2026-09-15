package crawl

import (
	"context"
	"sync"

	"github.com/lalternative/packages/go/search/fetch"
)

// Fetcher reads one page the way /fetch does: through the cache, the
// renderer and the challenge check. The walk never talks HTTP itself.
type Fetcher interface {
	Fetch(ctx context.Context, url string) (*fetch.Page, error)
}

// Result is one page of a walk, or the reason it could not be read.
type Result struct {
	URL   string
	Depth int
	Page  *fetch.Page
	Err   error
}

// concurrency bounds the pages read at once from one site: a crawl is a
// guest there, and three in flight already keeps the walk moving.
const concurrency = 3

// Walk reads the site from scope.Start breadth-first and hands each page to
// emit as it lands. It stops at MaxDepth, at MaxPages read, when robots.txt
// forbids everything left, or when ctx ends. The returned count is the
// number of pages emitted.
func Walk(ctx context.Context, scope Scope, f Fetcher, emit func(Result)) int {
	robots := loadRobots(ctx, scope.start)

	seen := map[string]bool{canonical(scope.Start): true}
	frontier := []string{scope.Start}
	if scope.Seed == SeedSitemap {
		for _, u := range sitemapURLs(ctx, scope, robots.sitemaps, scope.MaxPages) {
			if !seen[canonical(u)] {
				seen[canonical(u)] = true
				frontier = append(frontier, u)
			}
		}
	}
	read := 0

	for depth := 0; depth <= scope.MaxDepth && len(frontier) > 0 && read < scope.MaxPages; depth++ {
		var next []string
		var mu sync.Mutex
		sem := make(chan struct{}, concurrency)
		var wg sync.WaitGroup

		for _, u := range frontier {
			if read >= scope.MaxPages || ctx.Err() != nil {
				break
			}
			if !robots.allows(u) {
				continue
			}
			read++

			wg.Add(1)
			sem <- struct{}{}
			go func(u string) {
				defer wg.Done()
				defer func() { <-sem }()

				page, err := f.Fetch(ctx, u)
				mu.Lock()
				defer mu.Unlock()
				emit(Result{URL: u, Depth: depth, Page: page, Err: err})
				if page == nil || depth == scope.MaxDepth {
					return
				}
				for _, link := range page.Links {
					if key := canonical(link); !seen[key] && scope.Admits(link) {
						seen[key] = true
						next = append(next, link)
					}
				}
			}(u)
		}
		wg.Wait()
		frontier = next
	}
	return read
}
