import { createFileRoute } from '@tanstack/react-router'
import { ssoEnabled } from '@/lib/auth'

export const Route = createFileRoute('/api/auth/sso')({
  server: {
    handlers: {
      GET: async () =>
        new Response(JSON.stringify({ enabled: ssoEnabled, providerId: 'urbangate' }), {
          headers: { 'Content-Type': 'application/json' },
        }),
    },
  },
})
