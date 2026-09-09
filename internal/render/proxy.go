package render

import (
	"context"
	"net/url"
	"strings"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/chromedp"
)

// proxyAuth is the credential pair Chromium is asked for mid-connection.
//
// Chromium refuses userinfo in --proxy-server: a password on a command line
// is readable by every process on the host. It asks over CDP instead, once
// per connection, which is what answerProxyAuth replies to.
type proxyAuth struct {
	endpoint string
	user     string
	password string
}

// parseProxy splits a "scheme://user:pass@host:port" endpoint into what the
// flag takes and what the auth handler answers with. An empty raw yields a
// zero value, which means "no proxy" everywhere it is read.
func parseProxy(raw string) (proxyAuth, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return proxyAuth{}, nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return proxyAuth{}, errInvalidProxy
	}

	auth := proxyAuth{endpoint: u.Scheme + "://" + u.Host}
	if u.User != nil {
		auth.user = u.User.Username()
		auth.password, _ = u.User.Password()
	}
	return auth, nil
}

type invalidProxyError struct{}

// Error carries no detail: the value it came from holds credentials.
func (invalidProxyError) Error() string { return "proxy URL is not parseable" }

var errInvalidProxy = invalidProxyError{}

func (p proxyAuth) configured() bool { return p.endpoint != "" }

// answerProxyAuth replies to the proxy's authentication challenge and lets
// every other paused request through untouched.
//
// Fetch.enable with a handleAuthRequests pattern pauses requests as well as
// auth challenges, so a request that is not answered here stalls the page
// until the deadline — hence the continue on the plain request event.
func (p proxyAuth) answerProxyAuth(ctx context.Context) {
	if !p.configured() {
		return
	}
	chromedp.ListenTarget(ctx, func(ev any) {
		switch e := ev.(type) {
		case *fetch.EventAuthRequired:
			resp := &fetch.AuthChallengeResponse{Response: fetch.AuthChallengeResponseResponseProvideCredentials}
			if e.AuthChallenge.Source != fetch.AuthChallengeSourceProxy {
				// A site's own 401 is not ours to answer: replying with the
				// proxy's credentials would send them to the origin.
				resp = &fetch.AuthChallengeResponse{Response: fetch.AuthChallengeResponseResponseCancelAuth}
			} else {
				resp.Username = p.user
				resp.Password = p.password
			}
			// Off the event goroutine: a CDP call from inside the listener
			// deadlocks on the connection the event arrived on.
			go func() {
				_ = chromedp.Run(ctx, fetch.ContinueWithAuth(e.RequestID, resp))
			}()
		case *fetch.EventRequestPaused:
			go func() {
				_ = chromedp.Run(ctx, fetch.ContinueRequest(e.RequestID))
			}()
		}
	})
}
