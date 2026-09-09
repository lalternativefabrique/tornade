import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { AdminLoginForm } from '@lalternative/admin'
import { authClient } from '@/lib/auth-client'
import { getProfile } from '@/lib/services/auth'

/**
 * Admin sign-in. Filename has a trailing underscore so it sits outside the
 * `/admin` layout guard — otherwise you'd need to already be an admin to reach
 * the page that logs you in. The form (in @lalternative/admin) refuses
 * non-admins; navigation to /admin on success is wired here.
 */
export const Route = createFileRoute('/admin/login_')({
  component: AdminLoginPage,
})

function AdminLoginPage() {
  const navigate = useNavigate()
  return (
    <AdminLoginForm
      authClient={authClient}
      getProfile={getProfile}
      onSuccess={() => navigate({ to: '/admin' })}
    />
  )
}
