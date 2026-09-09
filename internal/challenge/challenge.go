// Package challenge tells a bot-management interstitial from the article it
// stands in for.
//
// A Cloudflare "Just a moment…" or a DataDome captcha answers 200 with a
// plausible title and a few sentences, which readability extracts like any
// page. Nothing upstream tells it apart, so tornade checks what fetch hands
// back before storing, caching or serving it.
package challenge

import (
	"strings"

	"github.com/lalternative/packages/go/search/fetch"
)

// Error reports a challenge page served in place of the one asked for.
type Error struct {
	Provider string
}

func (e *Error) Error() string {
	return "fetch page: " + e.Provider + " challenge served instead of the page"
}

// maxRunes bounds the extracted text a page can carry and still be taken for
// an interstitial. A challenge page is a couple of sentences; an article
// about captcha walls, or one whose title reads "Access Denied", is
// paragraphs. Only the extracted text is visible here, so this bound is what
// keeps the phrase matches below from flagging real content.
const maxRunes = 600

var textMarkers = []struct {
	provider string
	marker   string
}{
	{"cloudflare", "performance & security by cloudflare"},
	{"cloudflare", "needs to review the security of your connection"},
	{"cloudflare", "enable javascript and cookies to continue"},
	{"datadome", "please enable js and disable any ad blocker"},
	{"imperva", "incapsula incident id"},
	{"perimeterx", "press & hold"},
	{"akamai", "you don't have permission to access"},
}

var titleMarkers = []string{
	"just a moment",
	"attention required",
	"access denied",
	"verify you are human",
	"verify you are a human",
	"are you a human",
	"bot verification",
	"security check",
	"checking your browser",
	"please wait",
	"one more step",
	"enable javascript and cookies",
}

// Detect reports whether a page's extracted title and text are those of a
// challenge, and which vendor served it when the wording gives it away.
func Detect(title, text string) (provider string, ok bool) {
	if len([]rune(text)) > maxRunes {
		return "", false
	}
	lowerText := strings.ToLower(text)
	for _, m := range textMarkers {
		if strings.Contains(lowerText, m.marker) {
			return m.provider, true
		}
	}
	lowerTitle := strings.ToLower(title)
	for _, t := range titleMarkers {
		if strings.Contains(lowerTitle, t) {
			return "unknown", true
		}
	}
	return "", false
}

// Check returns an *Error when the page is a challenge, nil otherwise.
func Check(p *fetch.Page) error {
	if p == nil {
		return nil
	}
	if provider, ok := Detect(p.Title, p.Text); ok {
		return &Error{Provider: provider}
	}
	return nil
}

// GuardCache wraps c so a challenge page is never stored: the next fetch of
// that URL must see whether the challenge has lifted, not the interstitial
// kept for the cache's whole TTL.
func GuardCache(c fetch.Cache) fetch.Cache {
	return guardedCache{inner: c}
}

type guardedCache struct {
	inner fetch.Cache
}

func (g guardedCache) Get(url string) (*fetch.Page, bool) {
	return g.inner.Get(url)
}

func (g guardedCache) Set(url string, p *fetch.Page) {
	if Check(p) != nil {
		return
	}
	g.inner.Set(url, p)
}
