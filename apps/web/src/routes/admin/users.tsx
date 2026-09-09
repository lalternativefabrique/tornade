import { createFileRoute } from '@tanstack/react-router'
import { UsersTable } from '@lalternative/admin'
import { adminUserApi } from '@/lib/admin-api'

export const Route = createFileRoute('/admin/users')({
  component: () => (
    <UsersTable
      api={adminUserApi}
      onDeleteUser={async (userId) => {
        const res = await fetch(`/api/admin/users/${userId}`, {
          method: 'DELETE',
          credentials: 'include',
        })
        if (!res.ok) {
          const body = (await res.json().catch(() => ({}))) as { error?: string }
          throw new Error(body.error ?? 'Failed to delete user')
        }
      }}
    />
  ),
})
