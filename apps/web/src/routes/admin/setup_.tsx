import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { AdminSetupForm } from '@lalternative/admin'

/**
 * First-admin bootstrap. Trailing underscore keeps it outside the `/admin`
 * layout guard (it must be reachable with no session). Redirects away once an
 * admin already exists. The actual creation is server-side (SQL insert) in
 * /api/admin/setup — this only collects the fields.
 */
export const Route = createFileRoute('/admin/setup_')({
  component: AdminSetupPage,
})

function AdminSetupPage() {
  const navigate = useNavigate()
  const [ready, setReady] = useState(false)

  useEffect(() => {
    fetch('/api/admin/setup')
      .then((res) => res.json())
      .then((data: { hasAdmin: boolean }) => {
        if (data.hasAdmin) navigate({ to: '/admin/login' })
        else setReady(true)
      })
      .catch(() => setReady(true))
  }, [navigate])

  if (!ready) return null

  return (
    <AdminSetupForm
      onSubmit={async ({ name, email, password }) => {
        const res = await fetch('/api/admin/setup', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ name, email, password }),
        })
        const data = (await res.json().catch(() => ({}))) as { error?: string }
        if (!res.ok) throw new Error(data.error ?? 'Setup failed')
      }}
      onSuccess={() => navigate({ to: '/admin/login' })}
    />
  )
}
