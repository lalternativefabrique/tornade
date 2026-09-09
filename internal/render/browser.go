// Package render drives a headless Chromium to produce a page's rendered
// HTML, for pages a plain HTTP fetch sees as an empty shell.
package render

import (
	"context"
	"sync"
	"time"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// Browser holds one Chromium process shared across every request. A fresh
// browser per request would pay Chromium's 1-2s startup every time; a context
// is the unit of isolation that is actually cheap.
type Browser struct {
	execPath string
	proxy    proxyAuth

	once   sync.Once
	alloc  context.Context
	cancel context.CancelFunc

	// The proxied browser is a second Chromium process, started only if a
	// page is ever asked for through the proxy: --proxy-server is a
	// process-wide flag, so one browser cannot serve both routes.
	proxyOnce   sync.Once
	proxyAlloc  context.Context
	proxyCancel context.CancelFunc
}

// userAgent is the one the pages see. It names a real browser rather than
// this service because a page's markup is chosen from it: a site that
// recognises an automated reader serves a lighter shell, or nothing, which
// is the opposite of what a renderer is for.
const userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// New builds a Browser. execPath, when set, points at the Chromium binary —
// the Playwright base image installs it under /ms-playwright rather than on
// PATH, where chromedp looks by default.
//
// proxyURL, when set, is the residential endpoint RenderVia reads through.
// An unparseable value is an error here rather than a silent direct render.
func New(execPath, proxyURL string) (*Browser, error) {
	proxy, err := parseProxy(proxyURL)
	if err != nil {
		return nil, err
	}
	return &Browser{execPath: execPath, proxy: proxy}, nil
}

// HasProxy reports whether a proxied render is available at all.
func (b *Browser) HasProxy() bool { return b.proxy.configured() }

func (b *Browser) baseOpts() []chromedp.ExecAllocatorOption {
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
	return opts
}

func (b *Browser) start() {
	b.once.Do(func() {
		b.alloc, b.cancel = chromedp.NewExecAllocator(context.Background(), b.baseOpts()...)
	})
}

func (b *Browser) startProxied() {
	b.proxyOnce.Do(func() {
		opts := append(b.baseOpts(), chromedp.ProxyServer(b.proxy.endpoint))
		b.proxyAlloc, b.proxyCancel = chromedp.NewExecAllocator(context.Background(), opts...)
	})
}

// Render implements fetch.Renderer, the fallback FetchWithFallback reaches
// for once a static fetch has failed or come back near-empty.
//
// It goes through the proxy, when one is configured, because that is exactly
// the population of pages this is called on: a page refused on our egress, or
// one whose shell hides its content. Rendering it direct would repeat the
// fetch that already lost. Nothing else calls this — /render has its own
// handler on RenderPage — so the metered residential bandwidth is spent on
// retries alone.
func (b *Browser) Render(ctx context.Context, url string, timeout time.Duration) (string, error) {
	html, _, err := b.RenderViaProxy(ctx, url, timeout)
	return html, err
}

// RenderPage navigates to url, waits for the network to go quiet and returns
// the rendered HTML plus the URL actually loaded, which a redirect can make
// differ from the one asked for.
func (b *Browser) RenderPage(ctx context.Context, url string, timeout time.Duration) (string, string, error) {
	b.start()
	return b.renderOn(ctx, b.alloc, false, url, timeout)
}

// RenderViaProxy renders through the residential proxy. It is the answer to a
// page refused on our own egress — a publisher behind bot management turns
// away a datacenter address whatever it sends — and costs a browser plus
// metered residential bandwidth, so it belongs on the retry rather than on
// every render.
//
// Without a proxy configured it renders direct, so a caller need not ask.
func (b *Browser) RenderViaProxy(ctx context.Context, url string, timeout time.Duration) (string, string, error) {
	if !b.proxy.configured() {
		return b.RenderPage(ctx, url, timeout)
	}
	b.startProxied()
	return b.renderOn(ctx, b.proxyAlloc, true, url, timeout)
}

func (b *Browser) renderOn(ctx context.Context, alloc context.Context, viaProxy bool, url string, timeout time.Duration) (string, string, error) {
	// The tab hangs off the shared allocator but inherits the caller's
	// cancellation and deadline, so an abandoned or overrunning request tears
	// its own tab down without touching the browser others are using.
	tabCtx, cancelTab := chromedp.NewContext(alloc)
	defer cancelTab()
	tabCtx, cancelTimeout := context.WithTimeout(tabCtx, timeout)
	defer cancelTimeout()
	stop := context.AfterFunc(ctx, cancelTimeout)
	defer stop()

	var html, finalURL string
	// The listener must be attached before any navigation, or the requests
	// that carry the page's content are missed entirely.
	idle := watchNetworkIdle(tabCtx)

	actions := []chromedp.Action{network.Enable()}
	if viaProxy {
		// Fetch.enable must come before the navigation too: the proxy
		// challenges the very first request, and an unanswered challenge
		// stalls the page until the deadline.
		b.proxy.answerProxyAuth(tabCtx)
		actions = append(actions, fetch.Enable().WithHandleAuthRequests(true))
	}
	actions = append(actions,
		chromedp.Navigate(url),
		idle.wait(),
		chromedp.Location(&finalURL),
		chromedp.OuterHTML("html", &html),
	)

	if err := chromedp.Run(tabCtx, actions...); err != nil {
		return "", "", err
	}
	return html, finalURL, nil
}

func (b *Browser) Close() {
	if b.cancel != nil {
		b.cancel()
	}
	if b.proxyCancel != nil {
		b.proxyCancel()
	}
}
