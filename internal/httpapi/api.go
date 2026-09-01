// Package httpapi is tornade's HTTP contract over the search, fetch and tts
// libraries.
//
// Handlers take interfaces rather than concrete backends, so the contract —
// status codes, validation, bounds — is tested without a SearXNG instance, a
// render service or a voice behind it.
package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/lalternative/packages/go/search"
	"github.com/lalternative/packages/go/search/fetch"
	"github.com/lalternative/packages/go/tts"
)

// Deps are the backends the handlers speak to. A nil Voice or Renderer means
// that feature is not configured, which the handlers report rather than
// working around.
type Deps struct {
	Providers   map[search.Category]search.Provider
	Renderer    fetch.Renderer
	Cache       fetch.Cache
	Voice       tts.Voice
	VoiceFormat string

	SearchDeadline   time.Duration
	RenderMaxTimeout time.Duration
}

func New(d Deps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.Handle("POST /search", handleSearch(d))
	mux.Handle("POST /fetch", handleFetch(d))
	mux.Handle("POST /render", handleRender(d))
	mux.Handle("POST /speak", handleSpeak(d))
	return mux
}

// maxBody bounds a request body. Every endpoint here takes a small JSON
// object except /speak, whose text is the one field that can legitimately be
// large — a whole article.
const maxBody = 4 << 20

func decode(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, maxBody)).Decode(v)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
