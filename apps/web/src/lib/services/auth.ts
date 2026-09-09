import type { UserProfile } from '../types/auth'

/**
 * Fetch the current user's profile from the web app's /api/me. Throws when not
 * authenticated so callers (route guards, the admin gate) can redirect.
 */
export async function getProfile(): Promise<UserProfile> {
  const response = await fetch('/api/me', { credentials: 'include' })
  if (!response.ok) {
    throw new Error('Not authenticated')
  }
  return response.json() as Promise<UserProfile>
}
