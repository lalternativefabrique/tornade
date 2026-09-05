package httpapi

import (
	"errors"
	"net/http"

	"github.com/lalternativefabrique/tornade/signed"
)

// HeaderAppKey carries an application's own key on a server-to-server call.
//
// A browser never sends this — it cannot hold a secret — which is why the
// signed URL exists alongside it. The two answer different callers: a service
// on the cluster's own network authenticates as itself, a listener's browser
// carries an authorisation for one reading.
const HeaderAppKey = "X-Tornade-Key"

// guardSpeak refuses a /speak request that authenticates as neither.
//
// Both checks are skipped entirely when nothing is configured: a deployment
// reachable only from inside the cluster has the network as its boundary, and
// making it invent a key to talk to itself would be a credential that exists
// only to be checked.
func (d Deps) guardSpeak(r *http.Request, scope, id, text string) error {
	if d.Verifier == nil && len(d.AppKeys) == 0 {
		return nil
	}
	if key := r.Header.Get(HeaderAppKey); key != "" && d.AppKeys[key] {
		return nil
	}
	return d.Verifier.Verify(r.URL.Query(), scope, id, text)
}

// writeAuthError answers a failed guard.
//
// An expired link is told apart from a rejected one: asking the application
// again fixes the first and nothing fixes the second, and a player that knows
// the difference can fetch a fresh URL rather than showing an error to
// someone whose only mistake was leaving the page open.
func writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, signed.ErrExpired):
		writeError(w, http.StatusUnauthorized, "signature expired")
	default:
		writeError(w, http.StatusForbidden, "not authorised to read this text")
	}
}
