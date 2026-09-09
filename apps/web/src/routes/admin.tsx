import { createFileRoute, Link, Outlet, redirect } from '@tanstack/react-router'
import { AdminLayout } from '@lalternative/admin'
import { getProfile } from '@/lib/services/auth'
import { hasAdminFeatures } from '@/lib/hooks/useAdminFeaturesEnabled'

/**
 * Back-office shell. It deliberately sits outside `_protected` (no app chrome),
 * so it re-does the session guard and adds the admin-role check on top.
 *
 * `/admin/login` and `/admin/setup` are excluded from this layout (their files
 * carry a trailing underscore): login must be reachable without an admin
 * session, and setup bootstraps the very first admin.
 *
 * The guard here is a navigation hint only — it is skipped during SSR and every
 * admin endpoint re-checks the role server-side.
 */
export const Route = createFileRoute('/admin')({
  beforeLoad: async ({ context }) => {
    if (typeof window === 'undefined') return
    let user
    try {
      user = await context.queryClient.ensureQueryData({
        queryKey: ['me'],
        queryFn: getProfile,
        staleTime: Infinity,
      })
    } catch {
      throw redirect({ to: '/admin/login' })
    }
    if (!hasAdminFeatures(user)) {
      throw redirect({ to: '/admin/login' })
    }
  },
  component: AdminShell,
})

function AdminShell() {
  const linkClass = 'text-muted-foreground hover:text-foreground'
  const activeClass = 'text-foreground'
  return (
    <AdminLayout
      nav={
        <>
          <Link to="/admin" activeOptions={{ exact: true }} className={linkClass} activeProps={{ className: activeClass }}>
            Tableau de bord
          </Link>
          <Link to="/admin/apps" className={linkClass} activeProps={{ className: activeClass }}>
            Applications
          </Link>
          <Link to="/admin/users" className={linkClass} activeProps={{ className: activeClass }}>
            Utilisateurs
          </Link>
        </>
      }
      app={{ name: 'Tornade', tone: 'blue' }}
    >
      <Outlet />
    </AdminLayout>
  )
}
