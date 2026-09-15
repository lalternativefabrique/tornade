package httpapi

import (
	"context"
	"errors"
	"net/url"

	"github.com/lalternative/packages/go/search/fetch"

	"github.com/lalternativefabrique/vvaves/core/internal/challenge"
)

// readPage is the one way a page is read here, for /fetch, a crawl and a
// search's content alike. A bot-management interstitial served as the page
// is a refusal of this deployment's egress that the library cannot see,
// since it answers 200 with a title; the host is marked for the proxy and
// the page read once more through it.
func readPage(ctx context.Context, d Deps, rawURL string, renderer fetch.Renderer, maxRunes int) (*fetch.Page, error) {
	page, err := fetch.FetchWithFallback(ctx, rawURL, renderer, maxRunes, d.Cache)
	if err != nil {
		return nil, err
	}
	var served *challenge.Error
	if err := challenge.Check(page); !errors.As(err, &served) {
		return page, err
	}
	host := hostOf(rawURL)
	if fetch.ProxyPreferred(host) {
		return nil, served
	}
	fetch.PreferProxy(host)
	if !fetch.ProxyPreferred(host) {
		return nil, served
	}
	page, err = fetch.FetchWithFallback(ctx, rawURL, renderer, maxRunes, d.Cache)
	if err != nil {
		return nil, err
	}
	if err := challenge.Check(page); err != nil {
		return nil, err
	}
	return page, nil
}

func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Host
}
