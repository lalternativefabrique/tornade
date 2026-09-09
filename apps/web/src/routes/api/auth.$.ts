import { createFileRoute } from '@tanstack/react-router'
import { auth } from '@/lib/auth'

/**
 * Better Auth catch-all. Mounts every better-auth endpoint under /api/auth/*,
 * including the admin() plugin routes (list-users, remove-user, ban-user,
 * set-role) the admin surface calls. `authClient` and `auth.api` both go
 * through this handler.
 */
export const Route = createFileRoute('/api/auth/$')({
  server: {
    handlers: {
      GET: async ({ request }: { request: Request }) => auth.handler(request),
      POST: async ({ request }: { request: Request }) => auth.handler(request),
    },
  },
})
