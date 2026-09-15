package crawl

import "context"

// Map lists the site's URLs from scope.Start, up to limit: what its
// sitemaps declare when useSitemap is set, then every page read and every
// admitted link found on them, in discovery order. Pages are read only to
// find links, so MaxPages bounds the reads and limit the list; a sitemap
// that already fills the list saves the reads entirely.
func Map(ctx context.Context, scope Scope, f Fetcher, limit int, useSitemap bool) []string {
	seen := map[string]bool{canonical(scope.Start): true}
	urls := []string{scope.Start}
	add := func(u string) bool {
		if len(urls) >= limit {
			return false
		}
		if key := canonical(u); !seen[key] && scope.Admits(u) {
			seen[key] = true
			urls = append(urls, u)
		}
		return true
	}

	if useSitemap {
		robots := loadRobots(ctx, scope.start)
		for _, u := range sitemapURLs(ctx, scope, robots.sitemaps, limit) {
			if !add(u) {
				break
			}
		}
		if len(urls) >= limit {
			return urls
		}
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	Walk(ctx, scope, f, func(r Result) {
		if r.Page == nil {
			return
		}
		for _, link := range r.Page.Links {
			if !add(link) {
				cancel()
				return
			}
		}
	})
	return urls
}
