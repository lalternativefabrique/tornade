import { createFileRoute, redirect, useNavigate } from '@tanstack/react-router'
import { AdminSetupForm } from '@lalternative/admin'

/**
 * First-admin bootstrap. The `admin_` segment keeps it outside the `/admin`
 * layout guard: it must be reachable with no session. The setup token rides
 * in the URL the operator was handed, `?token=`, and the server re-checks it
 * along with everything else.
 */
export const Route = createFileRoute('/admin_/setup')({
  validateSearch: (search: Record<string, unknown>) => ({
    token: typeof search.token === 'string' ? search.token : undefined,
  }),
  beforeLoad: async () => {
    if (typeof window === 'undefined') return
    let hasAdmin = false
    try {
      const res = await fetch('/api/admin/setup')
      hasAdmin = ((await res.json()) as { hasAdmin: boolean }).hasAdmin
    } catch {
      return
    }
    if (hasAdmin) throw redirect({ to: '/admin/login' })
  },
  component: AdminSetupPage,
})

function AdminSetupPage() {
  const navigate = useNavigate()
  const { token } = Route.useSearch()

  return (
    <div className="min-h-screen flex items-center justify-center px-4">
      <AdminSetupForm
        title="Tornade"
        subtitle="Create the first admin account"
        onSubmit={async ({ name, email, password }) => {
          const res = await fetch('/api/admin/setup', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name, email, password, setupToken: token }),
          })
          const data = (await res.json().catch(() => ({}))) as { error?: string }
          if (!res.ok) throw new Error(data.error ?? 'Setup failed')
        }}
        onSuccess={() => navigate({ to: '/admin/login' })}
      />
    </div>
  )
}
