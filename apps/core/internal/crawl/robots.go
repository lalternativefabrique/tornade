package crawl

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/temoto/robotstxt"
)

// userAgent is the token robots.txt rules are matched against. It names the
// service, as a polite crawler should, and is not the one pages see.
const userAgent = "vvaves"

type robots struct {
	group *robotstxt.Group
}

// loadRobots reads the site's robots.txt. A site without one, or one that
// cannot be read, allows everything: that is the convention, and a crawl
// refused for a transient error on that file would be the surprising case.
func loadRobots(ctx context.Context, site *url.URL) robots {
	target := &url.URL{Scheme: site.Scheme, Host: site.Host, Path: "/robots.txt"}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return robots{}
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return robots{}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return robots{}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
	if err != nil {
		return robots{}
	}
	data, err := robotstxt.FromBytes(body)
	if err != nil {
		return robots{}
	}
	return robots{group: data.FindGroup(userAgent)}
}

func (r robots) allows(raw string) bool {
	if r.group == nil {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	p := u.EscapedPath()
	if p == "" {
		p = "/"
	}
	return r.group.Test(p)
}
