/**
 * The profile shape returned by /api/me. `roles` drives the admin gate
 * (see @lalternative/admin `hasAdminFeatures`).
 */
export interface UserProfile {
  user_id: string
  email: string
  name: string
  avatar_url?: string
  roles: string[]
}
