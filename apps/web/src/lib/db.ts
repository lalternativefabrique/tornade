import { Pool } from 'pg'

/**
 * Postgres pool for the web tier. Better Auth owns the `user`/`account`/
 * `session` tables (see apps/migrations/000002_better_auth.up.sql);
 * this pool is what better-auth and the admin setup route write through.
 */
export const pool = new Pool({
  connectionString: process.env.DATABASE_URL,
})
