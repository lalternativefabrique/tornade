import { createFileRoute } from '@tanstack/react-router'
import type { PlatformSession } from '@lalternative/auth'
import { auth } from '@/lib/auth'

/**
 * Delete a user account (better-auth remove-user + its cascade: account,
 * session, …). This is the server-side gate for the admin users table.
 *
 * Scope: it removes the account only. Domain data keyed on user_id in your
 * bounded contexts is intentionally NOT touched here — purge it per context,
 * e.g. by publishing a `user.deleted` integration event from the core and
 * consuming it where the data lives.
 */
export const Route = createFileRoute('/api/admin/users/$userId')({
  server: {
    handlers: {
      DELETE: async ({
        request,
        params,
      }: {
        request: Request
        params: { userId: string }
      }) => {
        // PlatformSession is @lalternative/auth's hand-maintained contract for
        // what getSession really returns (the widened Auth type hides `role`).
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
        // The real gate — route beforeLoad guards are skipped during SSR.
        if (session.user.role !== 'admin') {
          return new Response(JSON.stringify({ error: 'Forbidden' }), {
            status: 403,
            headers: { 'Content-Type': 'application/json' },
          })
        }
        if (session.user.id === params.userId) {
          return new Response(
            JSON.stringify({
              error: 'You cannot delete your own account here.',
            }),
            { status: 400, headers: { 'Content-Type': 'application/json' } },
          )
        }

        // admin() endpoints are mounted under /api/auth but absent from the
        // widened type, so reach them over HTTP. Forward the caller's cookies:
        // Better Auth re-checks the admin role itself.
        const authOrigin = new URL(request.url).origin
        const cookie = request.headers.get('cookie') ?? ''
        const removed = await fetch(
          `${authOrigin}/api/auth/admin/remove-user`,
          {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', cookie },
            body: JSON.stringify({ userId: params.userId }),
          },
        )
        if (!removed.ok) {
          return new Response(
            JSON.stringify({ error: 'Failed to delete the account.' }),
            { status: 502, headers: { 'Content-Type': 'application/json' } },
          )
        }

        return new Response(JSON.stringify({ deleted: true }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      },
    },
  },
})
