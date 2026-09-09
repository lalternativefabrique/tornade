import { createPlatformAuth } from '@lalternative/auth/server'
import { tanstackStartCookies } from 'better-auth/tanstack-start'
import { pool } from './db'

/**
 * Better Auth for the web app. The Go core does not sign tokens — it only
 * verifies the JWT minted from this session (see apps/core/middleware/jwt.go),
 * so BETTER_AUTH_SECRET here and JWT_SECRET in the core must be kept in sync
 * per your minting setup.
 *
 * Registration is open by default (`betaMode` off). To gate sign-up behind an
 * invitation (e.g. a waitlist), set `betaMode: true` and pass `isInvited`.
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
  // Nobody signs up here: the first admin is minted by /admin/setup, the
  // next ones by an admin. An open sign-up endpoint would only mint accounts
  // that can see nothing, and a closed one is one fewer door.
  betaMode: true,
  isInvited: async () => false,
  google: process.env.GOOGLE_CLIENT_ID
    ? {
        clientId: process.env.GOOGLE_CLIENT_ID,
        clientSecret: process.env.GOOGLE_CLIENT_SECRET!,
      }
    : undefined,
  plugins: [tanstackStartCookies()],
})

export type Auth = typeof auth
