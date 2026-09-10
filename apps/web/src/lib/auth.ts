import { createPlatformAuth } from '@lalternative/auth/server'
import { tanstackStartCookies } from 'better-auth/tanstack-start'
import { pool } from './db'

/**
 * Better Auth for the web app. The Go core does not sign tokens — it only
 * verifies the JWT minted from this session (see apps/core/middleware/jwt.go),
 * so BETTER_AUTH_SECRET here and JWT_SECRET in the core must be kept in sync
 * per your minting setup.
 *
 * Nobody signs up here with a password: the team signs in through the suite's
 * identity provider (urbangate), and the admin role comes from its roles claim.
 */
const authSecret = process.env.BETTER_AUTH_SECRET
if (!authSecret) {
  throw new Error('BETTER_AUTH_SECRET environment variable is required')
}

export const auth = createPlatformAuth({
  database: pool,
  baseURL: process.env.BETTER_AUTH_URL ?? 'http://localhost:5273',
  secret: authSecret,
  appName: 'tornade',
  betaMode: true,
  isInvited: async () => false,
  google: process.env.GOOGLE_CLIENT_ID
    ? {
        clientId: process.env.GOOGLE_CLIENT_ID,
        clientSecret: process.env.GOOGLE_CLIENT_SECRET!,
      }
    : undefined,
  sso: process.env.URBANGATE_CLIENT_SECRET
    ? {
        issuer: process.env.URBANGATE_ISSUER_URL ?? 'https://id.urbangate.dev',
        clientId: process.env.URBANGATE_CLIENT_ID ?? 'tornade-admin',
        clientSecret: process.env.URBANGATE_CLIENT_SECRET,
        adminRole: 'tornade:admin',
      }
    : undefined,
  plugins: [tanstackStartCookies()],
})

export const ssoEnabled = Boolean(process.env.URBANGATE_CLIENT_SECRET)

export type Auth = typeof auth
