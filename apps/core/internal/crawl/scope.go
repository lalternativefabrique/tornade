// Package crawl walks a site from one URL: it reads pages through the same
// path /fetch uses, follows their links within bounds, and reports what it
// read. Nothing here knows about HTTP handlers or where a job is stored.
package crawl

import (
	"errors"
	"net/url"
	"path"
	"strings"
)

const (
	DefaultMaxDepth = 2
	MaxMaxDepth     = 5
	DefaultMaxPages = 50
	MaxMaxPages     = 500
)

// SeedSitemap starts a walk from every URL the site's sitemaps declare,
// not just Start, so a whole site is read flat rather than by depth.
const SeedSitemap = "sitemap"

// Scope bounds a walk. Paths are prefixes on the URL path: a URL is kept
// when it matches one of IncludePaths (or there are none) and none of
// ExcludePaths.
type Scope struct {
	Start        string   `json:"url"`
	MaxDepth     int      `json:"max_depth"`
	MaxPages     int      `json:"max_pages"`
	IncludePaths []string `json:"include_paths,omitempty"`
	ExcludePaths []string `json:"exclude_paths,omitempty"`
	Seed         string   `json:"seed,omitempty"`

	start *url.URL
}

var (
	ErrBadStart = errors.New("start url must be http or https")
	ErrBadSeed  = errors.New("seed must be empty or sitemap")
)

// Normalize fills the defaults, clamps the bounds and parses Start. It is
// what a handler calls before handing the scope to a walk.
func (s *Scope) Normalize() error {
	u, err := url.Parse(strings.TrimSpace(s.Start))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ErrBadStart
	}
	u.Fragment = ""
	s.start = u
	s.Start = u.String()
	if s.Seed != "" && s.Seed != SeedSitemap {
		return ErrBadSeed
	}

	if s.MaxDepth <= 0 {
		s.MaxDepth = DefaultMaxDepth
	}
	if s.MaxDepth > MaxMaxDepth {
		s.MaxDepth = MaxMaxDepth
	}
	if s.MaxPages <= 0 {
		s.MaxPages = DefaultMaxPages
	}
	if s.MaxPages > MaxMaxPages {
		s.MaxPages = MaxMaxPages
	}
	return nil
}

// Admits reports whether a discovered link stays inside the walk: same
// site, a page rather than a binary, and within the path rules.
func (s *Scope) Admits(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	if siteOf(u.Host) != siteOf(s.start.Host) {
		return false
	}
	if isBinary(u.Path) {
		return false
	}
	p := u.Path
	if p == "" {
		p = "/"
	}
	for _, ex := range s.ExcludePaths {
		if strings.HasPrefix(p, ex) {
			return false
		}
	}
	if len(s.IncludePaths) == 0 {
		return true
	}
	for _, in := range s.IncludePaths {
		if strings.HasPrefix(p, in) {
			return true
		}
	}
	return false
}

// canonical is the key two spellings of one page share: the fragment gone,
// and a trailing slash ignored, since /doc and /doc/ are one page on
// nearly every site and listing both wastes a read.
func canonical(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.Fragment = ""
	if len(u.Path) > 1 {
		u.Path = strings.TrimSuffix(u.Path, "/")
	}
	return u.String()
}

func siteOf(host string) string {
	return strings.TrimPrefix(strings.ToLower(host), "www.")
}

var binaryExtensions = map[string]bool{
	".pdf": true, ".zip": true, ".gz": true, ".tar": true, ".rar": true, ".7z": true,
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".svg": true, ".ico": true,
	".mp3": true, ".mp4": true, ".webm": true, ".avi": true, ".mov": true, ".wav": true, ".ogg": true,
	".doc": true, ".docx": true, ".xls": true, ".xlsx": true, ".ppt": true, ".pptx": true,
	".css": true, ".js": true, ".woff": true, ".woff2": true, ".ttf": true, ".exe": true, ".dmg": true,
}

func isBinary(p string) bool {
	return binaryExtensions[strings.ToLower(path.Ext(p))]
}
