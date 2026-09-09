import { timingSafeEqual } from 'node:crypto'

const REQUIRED_TOKEN = process.env.ADMIN_SETUP_TOKEN
const ALLOWED_EMAILS = process.env.ADMIN_ALLOWED_EMAILS?.split(',')
  .map((email) => email.trim().toLowerCase())
  .filter(Boolean)

function tokenMatches(given: string | undefined, expected: string): boolean {
  if (given === undefined) return false
  const a = Buffer.from(given)
  const b = Buffer.from(expected)
  return a.length === b.length && timingSafeEqual(a, b)
}

/**
 * /api/admin/setup is reachable with no session by design: the first admin
 * cannot authenticate before they exist. Without this gate, whoever reaches
 * the route first becomes admin, and an admin of tornade hands out the keys
 * of every application that speaks. In production an unconfigured gate is a
 * refusal, not a pass; outside production both checks stay opt-in for a
 * laptop.
 */
export function checkAdminSetupGate(email: string, setupToken: string | undefined): string | null {
  if (!REQUIRED_TOKEN && !ALLOWED_EMAILS) {
    if (process.env.NODE_ENV === 'production') {
      return 'Admin setup is not configured'
    }
    return null
  }
  if (REQUIRED_TOKEN && !tokenMatches(setupToken, REQUIRED_TOKEN)) {
    return 'Invalid or missing setup token'
  }
  if (ALLOWED_EMAILS && !ALLOWED_EMAILS.includes(email.trim().toLowerCase())) {
    return 'This email is not authorized to create the admin account'
  }
  return null
}
