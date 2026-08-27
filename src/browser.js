import { chromium } from 'playwright';

// One Chromium process is shared across every request; a browser context
// (Playwright's lightweight incognito-style session) is opened and closed
// per render instead. Launching a fresh browser per request would pay
// Chromium's ~1-2s startup cost on every call — the context is the unit of
// isolation that's actually cheap.
let browserPromise = null;

function getBrowser() {
  if (!browserPromise) {
    browserPromise = chromium.launch({
      headless: true,
      args: ['--disable-dev-shm-usage'],
    });
  }
  return browserPromise;
}

// render navigates to url, waits for the network to go quiet, and returns
// the fully rendered HTML plus the URL actually loaded (a redirect target
// can differ from what was asked for).
//
// 'networkidle' rather than 'load': a JS-rendered page's content typically
// arrives via a fetch/XHR that fires after the load event, so waiting for
// load alone would return the same empty shell a plain HTTP fetch already
// sees — the entire reason this service exists.
export async function render(url, timeoutMs) {
  const browser = await getBrowser();
  const context = await browser.newContext({
    userAgent:
      'Mozilla/5.0 (compatible; Tornade/1.0; +https://github.com/lalternativefabrique/tornade)',
  });
  try {
    const page = await context.newPage();
    const response = await page.goto(url, {
      waitUntil: 'networkidle',
      timeout: timeoutMs,
    });
    const html = await page.content();
    return { html, finalUrl: response ? response.url() : url };
  } finally {
    await context.close();
  }
}

export async function closeBrowser() {
  if (!browserPromise) return;
  const browser = await browserPromise;
  await browser.close();
}
