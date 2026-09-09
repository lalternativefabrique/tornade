import { createFileRoute } from '@tanstack/react-router'
import type { PlatformSession } from '@lalternative/auth'
import { auth } from '@/lib/auth'
import { mintCoreToken } from '@/lib/core-token'

/**
 * Same-origin proxy to tornade's admin API. The browser calls /api/v1/<path>;
 * the session is exchanged here for a short-lived JWT tornade verifies, so no
 * token is ever handed to the page. Only an admin session gets through: the
 * registry is the whole point of this app, and there is no other user.
 */
const CORE_URL = process.env.CORE_API_URL ?? 'http://localhost:8080'

const HOP_BY_HOP = new Set([
  'connection',
  'keep-alive',
  'proxy-authenticate',
  'proxy-authorization',
  'te',
  'trailer',
  'transfer-encoding',
  'upgrade',
  'host',
  'content-length',
  'cookie',
])

function forwardHeaders(headers: Headers): Headers {
  const out = new Headers()
  headers.forEach((value, key) => {
    if (!HOP_BY_HOP.has(key.toLowerCase())) out.set(key, value)
  })
  return out
}

function json(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

async function proxy(request: Request): Promise<Response> {
  // eslint-disable-next-line @typescript-eslint/no-unnecessary-type-assertion
  const session = (await auth.api.getSession({
    headers: request.headers,
  })) as PlatformSession | null
  if (!session) return json(401, { error: 'Unauthorized' })
  if (session.user.role !== 'admin') return json(403, { error: 'Forbidden' })

  const url = new URL(request.url)
  const headers = forwardHeaders(request.headers)
  headers.set('Authorization', `Bearer ${mintCoreToken(session)}`)

  const upstream = await fetch(`${CORE_URL}${url.pathname}${url.search}`, {
    method: request.method,
    headers,
    body:
      request.method === 'GET' || request.method === 'HEAD'
        ? undefined
        : request.body,
    duplex: 'half',
    redirect: 'manual',
  } as RequestInit)

  return new Response(upstream.body, {
    status: upstream.status,
    headers: forwardHeaders(upstream.headers),
  })
}

export const Route = createFileRoute('/api/v1/$')({
  server: {
    handlers: {
      GET: ({ request }: { request: Request }) => proxy(request),
      POST: ({ request }: { request: Request }) => proxy(request),
      DELETE: ({ request }: { request: Request }) => proxy(request),
    },
  },
})
