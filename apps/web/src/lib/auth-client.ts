import { createAuthClient } from 'better-auth/react'
import { emailOTPClient, adminClient } from 'better-auth/client/plugins'

/**
 * Better Auth React client. Built inline with `createAuthClient` rather than
 * @lalternative/auth's `createPlatformAuthClient` on purpose: the wrapper's
 * return type hard-codes `plugins: any[]`, which hides `authClient.admin.*`
 * from the static types — and the admin surface needs it. Keep the plugin list
 * (emailOTP + admin) in sync with the wrapper.
 *
 * On the server `window` is undefined, so pass an explicit baseURL for SSR
 * loaders / `beforeLoad` hooks; the browser uses window.location.origin.
 */
const baseURL =
  typeof window === 'undefined'
    ? (process.env.BETTER_AUTH_URL ?? 'http://localhost:5273')
    : window.location.origin

export const authClient = createAuthClient({
  baseURL,
  plugins: [emailOTPClient(), adminClient()],
})
