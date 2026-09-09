import { Pool } from 'pg'

/**
 * Postgres pool for the web tier. Better Auth owns the `user`/`account`/
 * `session` tables (see apps/core/migrations/postgres/0002_better_auth.sql);
 * this pool is what better-auth and the admin setup route write through.
 */
export const pool = new Pool({
  connectionString: process.env.DATABASE_URL,
})
