import { createFileRoute } from '@tanstack/react-router'
import { bootstrapFirstAdmin } from '@lalternative/auth/server'
import { auth } from '@/lib/auth'
import { pool } from '@/lib/db'
import { checkAdminSetupGate } from '@/lib/admin-setup-gate'

function json(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

/**
 * First-admin bootstrap. GET reports whether any admin exists; POST mints the
 * first one behind the setup gate, and refuses once any admin exists.
 */
export const Route = createFileRoute('/api/admin/setup')({
  server: {
    handlers: {
      GET: async () => {
        const result = await pool.query(`SELECT COUNT(*)::int AS count FROM "user" WHERE role = 'admin'`)
        return json(200, { hasAdmin: result.rows[0].count > 0 })
      },

      POST: async ({ request }: { request: Request }) => {
        const body = (await request.json()) as {
          email?: string
          password?: string
          name?: string
          setupToken?: string
        }
        if (!body.email || !body.password || !body.name) {
          return json(400, { error: 'Email, password and name are required' })
        }
        if (body.password.length < 8) {
          return json(400, { error: 'Password must be at least 8 characters' })
        }
        const gateError = checkAdminSetupGate(body.email, body.setupToken)
        if (gateError) return json(403, { error: gateError })

        try {
          const result = await bootstrapFirstAdmin(auth, pool, {
            email: body.email,
            password: body.password,
            name: body.name,
          })
          if (!result.ok) return json(403, { error: 'Setup already completed' })
          return json(200, { success: true, email: body.email })
        } catch (err) {
          const duplicate =
            typeof err === 'object' && err !== null && 'code' in err && (err as { code?: string }).code === '23505'
          if (duplicate) return json(409, { error: 'This email is already registered' })
          throw err
        }
      },
    },
  },
})
