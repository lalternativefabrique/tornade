import { createFileRoute } from '@tanstack/react-router'
import { ssoEnabled } from '@/lib/auth'
import { SSO_PROVIDER_ID } from '@/lib/sso-config'

export const Route = createFileRoute('/api/auth/sso')({
  server: {
    handlers: {
      GET: async () =>
        new Response(
          JSON.stringify({ enabled: ssoEnabled, providerId: SSO_PROVIDER_ID }),
          {
            headers: { 'Content-Type': 'application/json' },
          },
        ),
    },
  },
})
