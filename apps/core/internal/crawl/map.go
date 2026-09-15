package crawl

import "context"

// Map lists the site's URLs from scope.Start: every page read plus every
// admitted link found on them, in discovery order, up to limit. Pages are
// read only to find links, so MaxPages bounds the reads and limit the list.
func Map(ctx context.Context, scope Scope, f Fetcher, limit int) []string {
	seen := map[string]bool{scope.Start: true}
	urls := []string{scope.Start}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	Walk(ctx, scope, f, func(r Result) {
		if r.Page == nil {
			return
		}
		for _, link := range r.Page.Links {
			if len(urls) >= limit {
				cancel()
				return
			}
			if !seen[link] && scope.Admits(link) {
				seen[link] = true
				urls = append(urls, link)
			}
		}
	})
	if len(urls) > limit {
		urls = urls[:limit]
	}
	return urls
}
