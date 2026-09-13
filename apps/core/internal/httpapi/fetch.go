package httpapi

import (
	"log"
	"net/http"
	"net/url"

	"github.com/lalternative/packages/go/fileguard"
	"github.com/lalternative/packages/go/search/fetch"

	"github.com/lalternativefabrique/vvaves/core/internal/challenge"
)

type fetchRequest struct {
	URL      string `json:"url"`
	MaxRunes int    `json:"max_runes"`
	Render   *bool  `json:"render"`
	Paginate int    `json:"paginate"`
}

type fetchResponse struct {
	Title string   `json:"title"`
	Text  string   `json:"text,omitempty"`
	Pages []string `json:"pages,omitempty"`
}

const defaultMaxRunes = 6000

func handleFetch(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := d.guardService(r); err != nil {
			writeAuthError(w, err)
			return
		}

		var req fetchRequest
		if err := decode(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if msg := validateURL(req.URL, d.AllowPrivateFetch); msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}

		maxRunes := req.MaxRunes
		if maxRunes <= 0 {
			maxRunes = defaultMaxRunes
		}

		renderer := d.Renderer
		if req.Render != nil && !*req.Render {
			renderer = nil
		}

		page, err := fetch.FetchWithFallback(r.Context(), req.URL, renderer, maxRunes, d.Cache)
		if err == nil {
			err = challenge.Check(page)
		}
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}

		out := fetchResponse{Title: page.Title}
		if req.Paginate > 0 {
			out.Pages = page.Paginate(req.Paginate)
		} else {
			out.Text = page.Text
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// validateURL answers why a URL may not be fetched, or "" when it may.
//
// fileguard refuses a host that is, or resolves to, an address this
// deployment reaches privately — loopback, link-local including the cloud
// metadata endpoint, multicast, RFC1918. Without that check these routes are
// an SSRF primitive: they take a URL from the caller and report what came
// back, which is a read of the internal network one request at a time.
//
// The caller is told its URL was refused and nothing more. Naming the reason
// would answer "is this address reachable from inside" for any address asked
// about, which is the scan the check exists to prevent; the detail goes to
// the log instead.
//
// This bounds what a caller may ask for, not what the fetch reaches. Behind
// FETCH_PROXY the connection is made to the proxy, so fileguard's dial-time
// guard would inspect the wrong endpoint and is deliberately not used here.
// A page is still fetched by a browser that resolves its own names and
// follows its own redirects — that is bounded by the network, not by this.
func validateURL(raw string, allowPrivate bool) string {
	if raw == "" {
		return "url is required"
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "url is not a valid URL"
	}
	// Checked whatever the deployment allows: a scheme outside http(s) is
	// refused for what it reads — file:// is the local disk — and allowing a
	// private address says nothing about allowing that.
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "url must be http or https"
	}
	if allowPrivate {
		return ""
	}
	if err := fileguard.ValidateFetchURL(raw); err != nil {
		log.Printf("vvaves: refused url %q: %v", raw, err)
		return "url is not allowed"
	}
	return ""
}
