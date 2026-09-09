package challenge

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lalternative/packages/go/search/fetch"
)

const cloudflareText = `www.example.com

Verifying you are human. This may take a few seconds.

www.example.com needs to review the security of your connection before proceeding.

Ray ID: 8d1f2c3a4b5e6f70

Performance & security by Cloudflare`

func TestDetectRecognisesVendorWording(t *testing.T) {
	cases := []struct {
		name, title, text, provider string
	}{
		{"cloudflare", "Just a moment...", cloudflareText, "cloudflare"},
		{"datadome", "example.com", "Please enable JS and disable any ad blocker", "datadome"},
		{"imperva", "example.com", "Request unsuccessful. Incapsula incident ID: 123-456", "imperva"},
		{"akamai", "Access Denied", "You don't have permission to access \"/a\" on this server. Reference #18.1", "akamai"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			provider, ok := Detect(c.title, c.text)
			if !ok || provider != c.provider {
				t.Errorf("Detect = (%q, %v), want (%q, true)", provider, ok, c.provider)
			}
		})
	}
}

func TestDetectFlagsAnInterstitialTitleOverLittleText(t *testing.T) {
	provider, ok := Detect("Verify you are human | example.com", "Complete the check below to continue to the site you requested.")
	if !ok || provider != "unknown" {
		t.Errorf("Detect = (%q, %v), want (unknown, true)", provider, ok)
	}
}

func TestDetectKeepsAnArticleAboutChallenges(t *testing.T) {
	text := strings.Repeat("Every day more publishers put a \"please wait, checking your browser\" page between readers and their articles. ", 8) +
		"Performance & security by Cloudflare is the footer they all share."
	if provider, ok := Detect("Why \"verify you are human\" walls break the open web", text); ok {
		t.Errorf("flagged as %q, want an article that merely quotes a challenge to pass", provider)
	}
}

func TestDetectKeepsAThinPageWithAnOrdinaryTitle(t *testing.T) {
	if provider, ok := Detect("Weekly recipe: soup", "Boil water. Add vegetables."); ok {
		t.Errorf("flagged as %q, want a thin ordinary page to pass", provider)
	}
}

func TestCheckReturnsATypedError(t *testing.T) {
	err := Check(&fetch.Page{Title: "Just a moment...", Text: cloudflareText})
	var ce *Error
	if !errors.As(err, &ce) || ce.Provider != "cloudflare" {
		t.Fatalf("Check = %v, want a *Error for cloudflare", err)
	}
	if err := Check(&fetch.Page{Title: "A real article", Text: strings.Repeat("Body. ", 200)}); err != nil {
		t.Errorf("Check = %v, want nil for an article", err)
	}
	if err := Check(nil); err != nil {
		t.Errorf("Check(nil) = %v, want nil", err)
	}
}

func TestGuardCacheNeverStoresAChallenge(t *testing.T) {
	c := GuardCache(fetch.NewMemoryCache(time.Minute))

	c.Set("http://a", &fetch.Page{Title: "Just a moment...", Text: cloudflareText})
	if _, ok := c.Get("http://a"); ok {
		t.Error("a challenge page was cached")
	}

	article := &fetch.Page{Title: "A real article", Text: strings.Repeat("Body. ", 200)}
	c.Set("http://b", article)
	if got, ok := c.Get("http://b"); !ok || got != article {
		t.Error("an article was not cached")
	}
}
