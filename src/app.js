import { createServer } from 'node:http';

// Hard ceiling independent of what a caller asks for: an unbounded render
// call is a request that never finishes, which on a shared browser process
// starves every other in-flight render behind it.
export const MAX_TIMEOUT_MS = 20_000;
export const DEFAULT_TIMEOUT_MS = 8_000;

async function readJSON(req) {
  const chunks = [];
  for await (const chunk of req) chunks.push(chunk);
  return JSON.parse(Buffer.concat(chunks).toString('utf8') || '{}');
}

function sendJSON(res, status, body) {
  const payload = JSON.stringify(body);
  res.writeHead(status, { 'Content-Type': 'application/json' });
  res.end(payload);
}

async function handleRender(req, res, render) {
  let body;
  try {
    body = await readJSON(req);
  } catch {
    return sendJSON(res, 400, { error: 'invalid JSON body' });
  }

  const url = typeof body.url === 'string' ? body.url : '';
  if (!url) {
    return sendJSON(res, 400, { error: 'url is required' });
  }
  let parsed;
  try {
    parsed = new URL(url);
  } catch {
    return sendJSON(res, 400, { error: 'url is not a valid URL' });
  }
  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
    return sendJSON(res, 400, { error: 'url must be http or https' });
  }

  const requested = Number.isFinite(body.timeout_ms) ? body.timeout_ms : DEFAULT_TIMEOUT_MS;
  const timeoutMs = Math.min(Math.max(requested, 1_000), MAX_TIMEOUT_MS);

  try {
    const { html, finalUrl } = await render(url, timeoutMs);
    return sendJSON(res, 200, { html, final_url: finalUrl });
  } catch (err) {
    // A page that never goes network-idle, refuses the connection, or 4xx/5xxs
    // is routine on the open web — the caller (fetch.Renderer) already treats
    // a render error as "fall back to the static result", not a fatal error.
    return sendJSON(res, 502, { error: String(err.message || err) });
  }
}

// createApp wires the HTTP contract to a render function, kept separate from
// browser.js so the contract (status codes, validation, timeout clamping)
// tests without launching a real browser.
export function createApp(render) {
  return createServer((req, res) => {
    if (req.method === 'GET' && req.url === '/healthz') {
      return sendJSON(res, 200, { ok: true });
    }
    if (req.method === 'POST' && req.url === '/render') {
      return handleRender(req, res, render);
    }
    return sendJSON(res, 404, { error: 'not found' });
  });
}
