package render

import (
	"context"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

const (
	// idleWindow is how long the network must stay quiet before a page counts
	// as settled.
	idleWindow = 500 * time.Millisecond
	// settleGrace is how long a page that has gone quiet is still watched for
	// a late request. A script that fires its fetch on a timer leaves the
	// network idle in the meantime, and calling that "settled" returns the
	// same empty shell a plain HTTP fetch already sees.
	settleGrace = 2 * time.Second
)

// networkIdle waits for a page's network to go quiet, which chromedp has no
// equivalent of and which is the whole reason this service exists: a
// JS-rendered page's content arrives through a fetch/XHR fired after the load
// event, so waiting on load alone returns an empty shell.
type networkIdle struct {
	mu       sync.Mutex
	inFlight int
	lastEvt  time.Time
	started  bool
}

func watchNetworkIdle(ctx context.Context) *networkIdle {
	n := &networkIdle{}
	chromedp.ListenTarget(ctx, func(ev any) {
		switch ev.(type) {
		case *network.EventRequestWillBeSent:
			n.request()
		case *network.EventLoadingFinished, *network.EventLoadingFailed:
			n.done()
		}
	})
	return n
}

func (n *networkIdle) request() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.started = true
	n.inFlight++
	n.lastEvt = time.Now()
}

func (n *networkIdle) done() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.inFlight > 0 {
		n.inFlight--
	}
	n.lastEvt = time.Now()
}

// quietFor reports how long the network has had nothing in flight, and
// whether any request was ever seen at all.
func (n *networkIdle) quietFor() (time.Duration, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.started || n.inFlight > 0 {
		return 0, n.started
	}
	return time.Since(n.lastEvt), true
}

// wait blocks until the network has been quiet for idleWindow and has stayed
// that way past settleGrace, or until the context's deadline.
//
// A page that never settles — an open websocket, a poller — hits the deadline
// and yields whatever it had rendered by then, which beats failing a request
// that already holds usable content.
func (n *networkIdle) wait() chromedp.ActionFunc {
	return func(ctx context.Context) error {
		tick := time.NewTicker(50 * time.Millisecond)
		defer tick.Stop()

		var quietSince time.Time
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-tick.C:
			}

			quiet, seen := n.quietFor()
			if !seen || quiet < idleWindow {
				quietSince = time.Time{}
				continue
			}
			if quietSince.IsZero() {
				quietSince = time.Now()
			}
			// Keep watching past the first lull: a late fetch resets inFlight
			// and puts the page back in flight before the grace elapses.
			if time.Since(quietSince) >= settleGrace {
				return nil
			}
		}
	}
}
