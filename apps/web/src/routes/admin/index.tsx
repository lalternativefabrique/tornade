import { createFileRoute, Link } from '@tanstack/react-router'
import { AdminHome } from '@lalternative/admin'
import { adminUserApi } from '@/lib/admin-api'

export const Route = createFileRoute('/admin/')({
  component: () => (
    <AdminHome
      api={adminUserApi}
      usersLink={({ className, children }) => (
        <Link to="/admin/users" className={className}>
          {children}
        </Link>
      )}
    />
  ),
})
