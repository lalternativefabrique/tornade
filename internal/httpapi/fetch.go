package httpapi

import (
	"net/http"
	"net/url"

	"github.com/lalternative/packages/go/search/fetch"
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
		var req fetchRequest
		if err := decode(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if msg := validateURL(req.URL); msg != "" {
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

func validateURL(raw string) string {
	if raw == "" {
		return "url is required"
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "url is not a valid URL"
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "url must be http or https"
	}
	return ""
}
