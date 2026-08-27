import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createApp, DEFAULT_TIMEOUT_MS, MAX_TIMEOUT_MS } from '../app.js';

async function withServer(render, run) {
  const server = createApp(render);
  await new Promise((resolve) => server.listen(0, resolve));
  const { port } = server.address();
  try {
    await run(`http://127.0.0.1:${port}`);
  } finally {
    await new Promise((resolve) => server.close(resolve));
  }
}

test('GET /healthz returns ok', async () => {
  await withServer(async () => ({ html: '', finalUrl: '' }), async (base) => {
    const res = await fetch(`${base}/healthz`);
    assert.equal(res.status, 200);
    assert.deepEqual(await res.json(), { ok: true });
  });
});

test('POST /render rejects a missing url', async () => {
  await withServer(async () => ({ html: '', finalUrl: '' }), async (base) => {
    const res = await fetch(`${base}/render`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({}),
    });
    assert.equal(res.status, 400);
  });
});

test('POST /render rejects a non-http(s) scheme', async () => {
  await withServer(async () => ({ html: '', finalUrl: '' }), async (base) => {
    const res = await fetch(`${base}/render`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url: 'ftp://example.com' }),
    });
    assert.equal(res.status, 400);
  });
});

test('POST /render returns the rendered html and final url', async () => {
  let receivedTimeout;
  const render = async (url, timeoutMs) => {
    receivedTimeout = timeoutMs;
    return { html: '<html>hi</html>', finalUrl: url };
  };

  await withServer(render, async (base) => {
    const res = await fetch(`${base}/render`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url: 'https://example.com' }),
    });
    assert.equal(res.status, 200);
    const body = await res.json();
    assert.equal(body.html, '<html>hi</html>');
    assert.equal(body.final_url, 'https://example.com');
    assert.equal(receivedTimeout, DEFAULT_TIMEOUT_MS);
  });
});

test('POST /render clamps timeout_ms to MAX_TIMEOUT_MS', async () => {
  let receivedTimeout;
  const render = async (_url, timeoutMs) => {
    receivedTimeout = timeoutMs;
    return { html: '', finalUrl: '' };
  };

  await withServer(render, async (base) => {
    await fetch(`${base}/render`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url: 'https://example.com', timeout_ms: 999_999 }),
    });
    assert.equal(receivedTimeout, MAX_TIMEOUT_MS);
  });
});

test('POST /render surfaces a render failure as 502', async () => {
  const render = async () => {
    throw new Error('navigation timeout');
  };

  await withServer(render, async (base) => {
    const res = await fetch(`${base}/render`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url: 'https://example.com' }),
    });
    assert.equal(res.status, 502);
    const body = await res.json();
    assert.match(body.error, /navigation timeout/);
  });
});
