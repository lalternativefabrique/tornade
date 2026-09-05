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

	"github.com/lalternative/packages/go/audioreader"
	"github.com/lalternative/packages/go/search"
	"github.com/lalternative/packages/go/search/fetch"

	"github.com/lalternativefabrique/tornade/signed"
)

// Deps are the backends the handlers speak to. A nil Reader or Renderer means
// that feature is not configured, which the handlers report rather than
// working around.
//
// A nil Primer with a non-nil Reader is the ordinary shape of a tornade with
// no bucket: readings still stream and are still served, they are just never
// kept, and nothing can be read ahead of time.
type Deps struct {
	Providers map[search.Category]search.Provider
	Renderer  fetch.Renderer
	Cache     fetch.Cache
	Reader    *audioreader.Reader
	Primer    *audioreader.Primer

	SearchDeadline   time.Duration
	RenderMaxTimeout time.Duration

	// Verifier authenticates a /speak request that came straight from a
	// browser. Nil accepts none, which is what a deployment reachable only
	// from the cluster wants.
	Verifier *signed.Verifier
	// AppKeys are the keys services authenticate with on a call of their own,
	// mapping each secret to the name of the service holding it.
	AppKeys map[string]string
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
	mux.Handle("POST /speak/prime", handlePrime(d))
	mux.Handle("POST /speak/pregenerate", handlePregenerate(d))
	mux.Handle("POST /speak/exists", handleExists(d))
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
