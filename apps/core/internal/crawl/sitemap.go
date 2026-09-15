package crawl

import (
	"compress/gzip"
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// maxSitemapFiles bounds how many sitemap files one listing reads: an
	// index can name thousands, and a listing is a request, not a job.
	maxSitemapFiles = 50
	maxSitemapBytes = 16 << 20
	sitemapTimeout  = 10 * time.Second
)

type sitemapFile struct {
	XMLName  xml.Name `xml:""`
	Sitemaps []struct {
		Loc string `xml:"loc"`
	} `xml:"sitemap"`
	URLs []struct {
		Loc string `xml:"loc"`
	} `xml:"url"`
}

// sitemapURLs lists what the site declares in its sitemaps, in file order,
// up to limit. declared comes from robots.txt; without one, /sitemap.xml is
// tried. Indexes are followed. Only files on the site itself are read: a
// sitemap pointing elsewhere is not a reason to fetch elsewhere.
func sitemapURLs(ctx context.Context, scope Scope, declared []string, limit int) []string {
	queue := declared
	if len(queue) == 0 {
		queue = []string{(&url.URL{Scheme: scope.start.Scheme, Host: scope.start.Host, Path: "/sitemap.xml"}).String()}
	}
	seen := map[string]bool{}
	var out []string
	files := 0
	for len(queue) > 0 && files < maxSitemapFiles && len(out) < limit {
		loc := queue[0]
		queue = queue[1:]
		if seen[loc] || !sameSite(loc, scope) {
			continue
		}
		seen[loc] = true
		files++

		file, ok := readSitemap(ctx, loc)
		if !ok {
			continue
		}
		for _, s := range file.Sitemaps {
			queue = append(queue, strings.TrimSpace(s.Loc))
		}
		for _, u := range file.URLs {
			link := strings.TrimSpace(u.Loc)
			if link == "" || seen[link] || !scope.Admits(link) {
				continue
			}
			seen[link] = true
			out = append(out, link)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}

func sameSite(raw string, scope Scope) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && siteOf(u.Host) == siteOf(scope.start.Host)
}

func readSitemap(ctx context.Context, loc string) (sitemapFile, bool) {
	ctx, cancel := context.WithTimeout(ctx, sitemapTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, loc, nil)
	if err != nil {
		return sitemapFile{}, false
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return sitemapFile{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return sitemapFile{}, false
	}

	var body io.Reader = io.LimitReader(resp.Body, maxSitemapBytes)
	if strings.HasSuffix(strings.ToLower(loc), ".gz") {
		gz, err := gzip.NewReader(body)
		if err != nil {
			return sitemapFile{}, false
		}
		defer gz.Close()
		body = gz
	}
	var file sitemapFile
	if err := xml.NewDecoder(body).Decode(&file); err != nil {
		return sitemapFile{}, false
	}
	return file, true
}
