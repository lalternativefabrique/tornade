import { createFileRoute } from '@tanstack/react-router'
import type { PlatformSession } from '@lalternative/auth'
import { auth } from '@/lib/auth'

/**
 * The web app's own profile endpoint, read straight from the better-auth
 * session (not the Go core), so it can expose the admin `role`. `getProfile`
 * and the admin gate consume this. `createPlatformAuth` widens its return to
 * the base Auth type, hiding `role`; PlatformSession is the package's
 * hand-maintained contract for what getSession really returns.
 */
export const Route = createFileRoute('/api/me')({
  server: {
    handlers: {
      GET: async ({ request }: { request: Request }) => {
        // eslint-disable-next-line @typescript-eslint/no-unnecessary-type-assertion
        const session = (await auth.api.getSession({
          headers: request.headers,
        })) as PlatformSession | null
        if (!session) {
          return new Response(JSON.stringify({ error: 'Unauthorized' }), {
            status: 401,
            headers: { 'Content-Type': 'application/json' },
          })
        }
        return new Response(
          JSON.stringify({
            user_id: session.user.id,
            email: session.user.email,
            name: session.user.name,
            avatar_url: session.user.image ?? '',
            roles: [session.user.role ?? 'user'],
          }),
          { headers: { 'Content-Type': 'application/json' } },
        )
      },
    },
  },
})
