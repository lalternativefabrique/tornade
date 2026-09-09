import { createReadStream, statSync } from 'node:fs'
import { createServer } from 'node:http'
import { extname, join, normalize } from 'node:path'

import handler from './dist/server/server.js'

// `vite build` emits dist/server/server.js as a fetch handler, not a listening
// server. This wraps it: static files from dist/client, everything else to
// the handler.
const CLIENT_DIR = join(import.meta.dirname, 'dist/client')
const PORT = Number(process.env.PORT ?? 5273)

const MIME = {
  '.js': 'text/javascript',
  '.mjs': 'text/javascript',
  '.css': 'text/css',
  '.html': 'text/html; charset=utf-8',
  '.json': 'application/json',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.ico': 'image/x-icon',
  '.woff': 'font/woff',
  '.woff2': 'font/woff2',
  '.txt': 'text/plain; charset=utf-8',
  '.map': 'application/json',
}

function serveStatic(pathname, res) {
  const filePath = normalize(join(CLIENT_DIR, pathname))
  if (!filePath.startsWith(CLIENT_DIR)) return false
  let stat
  try {
    stat = statSync(filePath)
  } catch {
    return false
  }
  if (!stat.isFile()) return false
  res.writeHead(200, {
    'content-type': MIME[extname(filePath)] ?? 'application/octet-stream',
    'content-length': stat.size,
    'cache-control': pathname.startsWith('/assets/')
      ? 'public, max-age=31536000, immutable'
      : 'public, max-age=0, must-revalidate',
  })
  createReadStream(filePath).pipe(res)
  return true
}

function toRequest(req) {
  const url = new URL(req.url, `http://${req.headers.host ?? 'localhost'}`)
  const method = req.method ?? 'GET'
  return new Request(url, {
    method,
    headers: req.headers,
    body: method === 'GET' || method === 'HEAD' ? undefined : req,
    duplex: 'half',
  })
}

const server = createServer(async (req, res) => {
  try {
    const { pathname } = new URL(
      req.url,
      `http://${req.headers.host ?? 'localhost'}`,
    )
    if (
      (req.method === 'GET' || req.method === 'HEAD') &&
      serveStatic(pathname, res)
    )
      return

    const response = await handler.fetch(toRequest(req))
    // set-cookie legitimately repeats, and entries() folds repeats into one
    // comma-joined value a browser reads as a single malformed cookie.
    const headers = Object.fromEntries(response.headers.entries())
    delete headers['set-cookie']
    const setCookie = response.headers.getSetCookie()
    if (setCookie.length > 0) headers['set-cookie'] = setCookie
    res.writeHead(response.status, headers)
    if (response.body) {
      const reader = response.body.getReader()
      for (;;) {
        const { done, value } = await reader.read()
        if (done) break
        res.write(value)
      }
    }
    res.end()
  } catch (error) {
    console.error('request failed:', error)
    if (!res.headersSent) res.writeHead(500)
    res.end('Internal Server Error')
  }
})

server.listen(PORT, '0.0.0.0', () => {
  console.log(`tornade web listening on :${PORT}`)
})

for (const signal of ['SIGTERM', 'SIGINT']) {
  process.on(signal, () => server.close(() => process.exit(0)))
}
