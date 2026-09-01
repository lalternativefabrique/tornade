// Package render drives a headless Chromium to produce a page's rendered
// HTML, for pages a plain HTTP fetch sees as an empty shell.
package render

import (
	"context"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// Browser holds one Chromium process shared across every request. A fresh
// browser per request would pay Chromium's 1-2s startup every time; a context
// is the unit of isolation that is actually cheap.
type Browser struct {
	execPath string
	once     sync.Once
	alloc    context.Context
	cancel   context.CancelFunc
}

const userAgent = "Mozilla/5.0 (compatible; Tornade/1.0; +https://github.com/lalternativefabrique/tornade)"

// New builds a Browser. execPath, when set, points at the Chromium binary —
// the Playwright base image installs it under /ms-playwright rather than on
// PATH, where chromedp looks by default.
func New(execPath string) *Browser { return &Browser{execPath: execPath} }

func (b *Browser) start() {
	b.once.Do(func() {
		opts := append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.UserAgent(userAgent),
			chromedp.Flag("disable-dev-shm-usage", true),
			// Chromium's own sandbox needs unprivileged user namespaces, which
			// a container's default seccomp profile blocks. Playwright passed
			// this same flag by default, so this is the isolation this service
			// has always run with: the container is the boundary around the
			// third-party JavaScript, not Chromium's inner sandbox.
			chromedp.NoSandbox,
		)
		if b.execPath != "" {
			opts = append(opts, chromedp.ExecPath(b.execPath))
		}
		b.alloc, b.cancel = chromedp.NewExecAllocator(context.Background(), opts...)
	})
}

// Render implements fetch.Renderer.
func (b *Browser) Render(ctx context.Context, url string, timeout time.Duration) (string, error) {
	html, _, err := b.RenderPage(ctx, url, timeout)
	return html, err
}

// RenderPage navigates to url, waits for the network to go quiet and returns
// the rendered HTML plus the URL actually loaded, which a redirect can make
// differ from the one asked for.
func (b *Browser) RenderPage(ctx context.Context, url string, timeout time.Duration) (string, string, error) {
	b.start()

	// The tab hangs off the shared allocator but inherits the caller's
	// cancellation and deadline, so an abandoned or overrunning request tears
	// its own tab down without touching the browser others are using.
	tabCtx, cancelTab := chromedp.NewContext(b.alloc)
	defer cancelTab()
	tabCtx, cancelTimeout := context.WithTimeout(tabCtx, timeout)
	defer cancelTimeout()
	stop := context.AfterFunc(ctx, cancelTimeout)
	defer stop()

	var html, finalURL string
	// The listener must be attached before any navigation, or the requests
	// that carry the page's content are missed entirely.
	idle := watchNetworkIdle(tabCtx)
	err := chromedp.Run(tabCtx,
		network.Enable(),
		chromedp.Navigate(url),
		idle.wait(),
		chromedp.Location(&finalURL),
		chromedp.OuterHTML("html", &html),
	)
	if err != nil {
		return "", "", err
	}
	return html, finalURL, nil
}

func (b *Browser) Close() {
	if b.cancel != nil {
		b.cancel()
	}
}
