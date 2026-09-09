import { createFileRoute } from '@tanstack/react-router'
import { randomUUID } from 'node:crypto'
import { hashPassword } from 'better-auth/crypto'
import { pool } from '@/lib/db'

/**
 * First-admin bootstrap. GET reports whether any admin exists; POST creates the
 * very first one directly in the DB (before any admin exists, the normal
 * sign-up flow can't mint an admin). Closes itself once an admin is present.
 */
export const Route = createFileRoute('/api/admin/setup')({
  server: {
    handlers: {
      GET: async () => {
        const result = await pool.query(
          `SELECT COUNT(*) as count FROM "user" WHERE role = 'admin'`,
        )
        const hasAdmin = parseInt(result.rows[0].count, 10) > 0
        return new Response(JSON.stringify({ hasAdmin }), {
          headers: { 'Content-Type': 'application/json' },
        })
      },

      POST: async ({ request }: { request: Request }) => {
        const check = await pool.query(
          `SELECT COUNT(*) as count FROM "user" WHERE role = 'admin'`,
        )
        if (parseInt(check.rows[0].count, 10) > 0) {
          return new Response(JSON.stringify({ error: 'Setup already completed' }), {
            status: 403,
            headers: { 'Content-Type': 'application/json' },
          })
        }

        const body = (await request.json()) as {
          email: string
          password: string
          name: string
        }
        if (!body.email || !body.password || !body.name) {
          return new Response(
            JSON.stringify({ error: 'Email, password and name are required' }),
            { status: 400, headers: { 'Content-Type': 'application/json' } },
          )
        }

        const hashedPassword = await hashPassword(body.password)
        const userId = randomUUID()

        await pool.query(
          `INSERT INTO "user" (id, name, email, "emailVerified", role, "createdAt", "updatedAt")
           VALUES ($1, $2, $3, true, 'admin', NOW(), NOW())`,
          [userId, body.name, body.email],
        )
        await pool.query(
          `INSERT INTO account (id, "accountId", "providerId", "userId", password, "createdAt", "updatedAt")
           VALUES ($1, $2, 'credential', $3, $4, NOW(), NOW())`,
          [randomUUID(), userId, userId, hashedPassword],
        )

        return new Response(
          JSON.stringify({ success: true, email: body.email }),
          { headers: { 'Content-Type': 'application/json' } },
        )
      },
    },
  },
})
