import type { AdminUserApi } from '@lalternative/admin'
import { authClient } from './auth-client'

/**
 * Adapter from the Better Auth admin() client to `@lalternative/admin`'s
 * `AdminUserApi`. This is the one place that touches the auth client's (widened,
 * role-hiding) type, so the shared admin components stay decoupled from it.
 *
 * Delete is intentionally NOT wired here — the users table routes deletion to
 * the server endpoint at /api/admin/users/$userId (better-auth remove-user +
 * cascade). Add domain-data cleanup there per bounded context.
 */
export const adminUserApi: AdminUserApi = {
  listUsers: async (query) => {
    const res = await authClient.admin.listUsers({ query: query ?? {} })
    if (res.error) throw new Error(res.error.message ?? 'Failed to load users')
    return {
      users: res.data.users,
      total: res.data.total,
    }
  },
  banUser: async (userId, reason) => {
    const res = await authClient.admin.banUser({ userId, banReason: reason })
    if (res.error) throw new Error(res.error.message ?? 'Failed to ban user')
    return res.data
  },
  unbanUser: async (userId) => {
    const res = await authClient.admin.unbanUser({ userId })
    if (res.error) throw new Error(res.error.message ?? 'Failed to unban user')
    return res.data
  },
  setRole: async (userId, role) => {
    const res = await authClient.admin.setRole({
      userId,
      role: role as 'admin' | 'user',
    })
    if (res.error) throw new Error(res.error.message ?? 'Failed to set role')
    return res.data
  },
}
